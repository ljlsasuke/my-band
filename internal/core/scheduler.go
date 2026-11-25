package core

import (
	"sort"
	"sync"
	"time"
)

// 可调参数 (PLAN.md M0-③ 调度器设计定稿)。
const (
	// DefaultLookAhead 前瞻窗口: 事件提前这么多发出, 补偿 winmm 消息投递
	// 的 5~20ms 延迟, 保证"听到的"落在时间轴上。
	DefaultLookAhead = 50 * time.Millisecond

	// DefaultTick 生产路径的调度轮询周期。
	DefaultTick = 5 * time.Millisecond

	// DefaultVoiceBudget 复音预算: 兜底音源 Microsoft GS Wavetable Synth
	// 为 32 复音, 超限会被系统挤音 (听感不可控), 所以在调度器层主动管理。
	// 挤音策略: 挤掉力度最低的活跃音 (同力度取最早发声的)。
	// 装了 VirtualMIDISynth 后复音上千, 可调大或置 0 关闭预算。
	DefaultVoiceBudget = 32
)

// EmitFunc 事件出口。回调应快速返回 (生产路径是往 winmm 发一条消息),
// 调度器在持锁状态下调用它。
type EmitFunc func(Event)

// voiceKey 标识一条正在发声的音。
type voiceKey struct{ ch, note int }

type voiceInfo struct {
	vel  int     // 力度, 挤音时优先挤最低
	born float64 // 曲内发声时间
	seq  uint64  // 单调序号, 同力度时挤最早发声的
}

// Scheduler 是总线的唯一时间权威 (主时钟)。
//
// 时钟模型 (PLAN.md M0-③, 已定稿):
//   - midiOutShortMsg 即发即忘, 合成器不回报播放位置, 不存在可读的
//     "音频时钟" → 主时钟 = 单调时钟 + 曲内时间轴;
//   - 前瞻调度: 曲内时间 <= now+LookAhead 的事件立即发出;
//   - time 广播、前端追帧均以 SongTime() 为准。
//
// 现场演奏/AI 即时事件不经过 Scheduler, 直接进总线 (T=0 立即生效)。
type Scheduler struct {
	LookAhead   time.Duration
	Tick        time.Duration
	VoiceBudget int // 0 = 不限复音

	emit EmitFunc
	now  func() time.Time // 可注入, 测试用

	events []Event // 按 T 升序
	idx    int     // 下一个待分发事件下标

	rate float64 // 播放速率倍率 (1.0 正常, M3 tempo op 用)

	mu         sync.Mutex
	stopCh     chan struct{}
	doneCh     chan struct{}
	running    bool
	paused     bool
	playStart  time.Time // 最近一次 resume 的墙钟时刻
	playedBase float64   // 暂停前累计的曲内时间
	active     map[voiceKey]voiceInfo
	seq        uint64
}

// NewScheduler 按曲内时间升序整理事件并创建调度器。
// emit 为事件出口 (后端发声、前端动画都从这里拿事件)。
func NewScheduler(events []Event, emit EmitFunc) *Scheduler {
	evs := make([]Event, len(events))
	copy(evs, events)
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].T < evs[j].T })
	return &Scheduler{
		LookAhead:   DefaultLookAhead,
		Tick:        DefaultTick,
		VoiceBudget: DefaultVoiceBudget,
		rate:        1.0,
		emit:        emit,
		now:         time.Now,
		events:      evs,
		active:      make(map[voiceKey]voiceInfo),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
}

// SetClock 注入时钟源 (仅测试用)。
func (s *Scheduler) SetClock(now func() time.Time) { s.now = now }

// Start 从 0 (或上次暂停处) 开始播放。
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.running = true
	s.playStart = s.now()
	go s.run()
}

func (s *Scheduler) run() {
	defer close(s.doneCh)
	ticker := time.NewTicker(s.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.Poll()
		}
	}
}

// Poll 推进一个调度周期: 发出曲内时间 <= now+LookAhead 的全部事件。
// 独立导出以便测试注入时钟后确定性驱动; 生产路径由 run() 周期调用。
func (s *Scheduler) Poll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.paused {
		return
	}
	deadline := s.songTimeLocked() + s.LookAhead.Seconds()
	for s.idx < len(s.events) && s.events[s.idx].T <= deadline {
		s.dispatchLocked(s.events[s.idx])
		s.idx++
	}
}

func (s *Scheduler) songTimeLocked() float64 {
	if s.paused {
		return s.playedBase
	}
	return s.playedBase + s.now().Sub(s.playStart).Seconds()*s.rate
}

// SetRate 设置播放速率倍率 (1.0 正常, 0.5 半速, 2.0 倍速)。
// 变速时重锚时钟: 当前曲内位置保持不变, 此后以新速率推进。
func (s *Scheduler) SetRate(rate float64) {
	if rate <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.songTimeLocked()
	s.playedBase, s.playStart, s.rate = st, s.now(), rate
}

// Seek 跳转到曲内位置 pos (秒)。
// 越过 seek 点的活跃音会被补发 NoteOff (跨点长音不保留, M3 简化语义,
// 见 docs/AP1.md); 队列指针直接跳到首个 T>=pos 的事件。
func (s *Scheduler) Seek(pos float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pos < 0 {
		pos = 0
	}
	// 活跃音补发 NoteOff, 避免悬音
	for k := range s.active {
		s.emit(Event{T: pos, Kind: KindNoteOff, Ch: k.ch, Note: k.note})
	}
	s.active = make(map[voiceKey]voiceInfo)
	s.idx = sort.Search(len(s.events), func(i int) bool { return s.events[i].T >= pos })
	s.playedBase = pos
	s.playStart = s.now()
}

// Pause 暂停 (记住曲内位置)。
func (s *Scheduler) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.paused {
		return
	}
	s.playedBase = s.songTimeLocked()
	s.paused = true
}

// Resume 从暂停处继续。
func (s *Scheduler) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || !s.paused {
		return
	}
	s.playStart = s.now()
	s.paused = false
}

// SongTime 当前曲内时间 (秒)。
func (s *Scheduler) SongTime() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.songTimeLocked()
}

// Paused 是否处于暂停态。
func (s *Scheduler) Paused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

// Remaining 尚未分发的事件数。
func (s *Scheduler) Remaining() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events) - s.idx
}

// Stop 停止调度, 并为所有活跃音补发 NoteOff (flush)。
// 未启动过时为空操作。阻塞直到调度循环退出。
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	for k := range s.active {
		s.emit(Event{T: s.songTimeLocked(), Kind: KindNoteOff, Ch: k.ch, Note: k.note})
	}
	s.active = make(map[voiceKey]voiceInfo)
	s.mu.Unlock()
	<-s.doneCh
}

// dispatchLocked 单事件分发: 维护活跃音表并驱动挤音/重触策略。
func (s *Scheduler) dispatchLocked(ev Event) {
	switch ev.Kind {
	case KindNoteOn:
		key := voiceKey{ev.Ch, ev.Note}
		// 同键重触: 先补 NoteOff
		if _, ok := s.active[key]; ok {
			delete(s.active, key)
			s.emit(Event{T: ev.T, Kind: KindNoteOff, Ch: ev.Ch, Note: ev.Note})
		}
		// 复音预算: 超限则挤音
		if s.VoiceBudget > 0 && len(s.active) >= s.VoiceBudget {
			s.stealLocked(ev.T)
		}
		s.seq++
		s.active[key] = voiceInfo{vel: ev.Vel, born: ev.T, seq: s.seq}
		s.emit(ev)
	case KindNoteOff:
		delete(s.active, voiceKey{ev.Ch, ev.Note})
		s.emit(ev)
	default:
		s.emit(ev)
	}
}

// stealLocked 挤掉力度最低 (同力度取最早) 的活跃音。
func (s *Scheduler) stealLocked(t float64) {
	var victim voiceKey
	found := false
	for k, v := range s.active {
		if !found {
			victim, found = k, true
			continue
		}
		ref := s.active[victim]
		if v.vel < ref.vel || (v.vel == ref.vel && v.seq < ref.seq) {
			victim = k
		}
	}
	if found {
		delete(s.active, victim)
		s.emit(Event{T: t, Kind: KindNoteOff, Ch: victim.ch, Note: victim.note})
	}
}
