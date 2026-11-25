package core

import (
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

type collector struct {
	mu  sync.Mutex
	got []Event
}

func (c *collector) emit(e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, e)
}

func (c *collector) snapshot() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Event, len(c.got))
	copy(out, c.got)
	return out
}

func TestPollEmitsWithinLookAhead(t *testing.T) {
	fc := &fakeClock{t: time.Now()}
	col := &collector{}
	evs := []Event{
		{T: 0.01, Kind: KindNoteOn, Ch: 0, Note: 60, Vel: 90},
		{T: 0.08, Kind: KindNoteOn, Ch: 0, Note: 64, Vel: 90},
		{T: 0.20, Kind: KindNoteOn, Ch: 0, Note: 67, Vel: 90},
		{T: 0.50, Kind: KindNoteOn, Ch: 0, Note: 72, Vel: 90},
	}
	s := NewScheduler(evs, col.emit)
	s.SetClock(fc.Now)
	s.LookAhead = 50 * time.Millisecond
	s.Start()
	defer s.Stop()

	// 曲内 0.01s, 截止 0.06s → 只有 T=0.01 的事件 (提前于它的播放时刻发出)
	fc.Advance(10 * time.Millisecond)
	s.Poll()
	if got := len(col.snapshot()); got != 1 {
		t.Fatalf("songTime=0.01s 应发出 1 个事件, 实得 %d", got)
	}

	// 曲内 0.11s, 截止 0.16s → T=0.08 发出, T=0.20 不发
	fc.Advance(100 * time.Millisecond)
	s.Poll()
	got := col.snapshot()
	if len(got) != 2 || got[1].Note != 64 {
		t.Fatalf("songTime=0.11s 应发出 2 个事件 (第二个 note=64), 实得 %v", got)
	}

	// T=0.20 未到, Remaining 应为 2
	if s.Remaining() != 2 {
		t.Fatalf("Remaining = %d, want 2", s.Remaining())
	}
}

func TestPauseResumeHoldsSongTime(t *testing.T) {
	fc := &fakeClock{t: time.Now()}
	col := &collector{}
	s := NewScheduler([]Event{{T: 10, Kind: KindMarker, Text: "far"}}, col.emit)
	s.SetClock(fc.Now)
	s.Start()

	fc.Advance(2 * time.Second)
	s.Pause()
	if st := s.SongTime(); st < 1.99 || st > 2.01 {
		t.Fatalf("暂停点 songTime = %.3f, want ~2.0", st)
	}

	// 暂停期间墙钟继续走, 曲内时间必须冻结
	fc.Advance(5 * time.Second)
	if st := s.SongTime(); st < 1.99 || st > 2.01 {
		t.Fatalf("暂停期间 songTime = %.3f, want 冻结在 ~2.0", st)
	}

	s.Resume()
	fc.Advance(1 * time.Second)
	if st := s.SongTime(); st < 2.99 || st > 3.01 {
		t.Fatalf("恢复后 songTime = %.3f, want ~3.0", st)
	}
}

func TestVoiceBudgetStealsQuietest(t *testing.T) {
	fc := &fakeClock{t: time.Now()}
	col := &collector{}
	// 预算 2: 第三个 NoteOn 应挤掉力度最低 (50) 的那个音
	evs := []Event{
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 60, Vel: 100},
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 64, Vel: 50},
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 67, Vel: 80},
	}
	s := NewScheduler(evs, col.emit)
	s.SetClock(fc.Now)
	s.VoiceBudget = 2
	s.Start()
	s.Poll()
	defer s.Stop()

	got := col.snapshot()
	want := []struct {
		kind Kind
		note int
	}{
		{KindNoteOn, 60}, {KindNoteOn, 64},
		{KindNoteOff, 64}, // 被挤: 力度最低
		{KindNoteOn, 67},
	}
	if len(got) != len(want) {
		t.Fatalf("事件数 = %d, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Kind != w.kind || got[i].Note != w.note {
			t.Errorf("事件 #%d = %s(note %d), want %s(note %d)",
				i, got[i].Kind, got[i].Note, w.kind, w.note)
		}
	}
}

func TestRetriggerEmitsNoteOffFirst(t *testing.T) {
	fc := &fakeClock{t: time.Now()}
	col := &collector{}
	evs := []Event{
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 60, Vel: 90},
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 60, Vel: 90}, // 同键重触
	}
	s := NewScheduler(evs, col.emit)
	s.SetClock(fc.Now)
	s.Start()
	s.Poll()
	defer s.Stop()

	got := col.snapshot()
	if len(got) != 3 || got[1].Kind != KindNoteOff || got[1].Note != 60 {
		t.Fatalf("重触应产生 [On, Off, On], 实得 %v", got)
	}
}

func TestStopFlushesActiveNotes(t *testing.T) {
	fc := &fakeClock{t: time.Now()}
	col := &collector{}
	evs := []Event{
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 60, Vel: 90},
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 64, Vel: 90},
		{T: 100, Kind: KindNoteOff, Ch: 0, Note: 60}, // 远在未来的释放
	}
	s := NewScheduler(evs, col.emit)
	s.SetClock(fc.Now)
	s.Start()
	s.Poll() // 只发出两个 NoteOn
	s.Stop() // flush: 两个活跃音都补 NoteOff

	got := col.snapshot()
	if len(got) != 4 {
		t.Fatalf("Stop 后应共 4 个事件 (2 On + 2 flush Off), 实得 %d: %v", len(got), got)
	}
	for _, e := range got[2:] {
		if e.Kind != KindNoteOff {
			t.Errorf("flush 事件应为 NoteOff, 实得 %s", e.Kind)
		}
	}
}

func TestRateAndSeek(t *testing.T) {
	fc := &fakeClock{t: time.Now()}
	col := &collector{}
	evs := []Event{
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 60, Vel: 90},
		{T: 10, Kind: KindMarker, Text: "mid"},
		{T: 20, Kind: KindMarker, Text: "end"},
	}
	s := NewScheduler(evs, col.emit)
	s.SetClock(fc.Now)
	s.Start()
	defer s.Stop()

	// 倍速: 前 2s 墙钟 → 曲内 2s; 变速后再 2s 墙钟 → 曲内 +4s = 6s
	fc.Advance(2 * time.Second)
	s.SetRate(2.0)
	fc.Advance(2 * time.Second)
	if st := s.SongTime(); st < 5.99 || st > 6.01 {
		t.Errorf("变速后曲内 = %v, want 6 (2 + 2×2)", st)
	}

	// Seek 到 15s: 队列指针跳至末段, 位置重锚
	s.Seek(15)
	if st := s.SongTime(); st < 14.99 || st > 15.01 {
		t.Errorf("Seek 后 songTime = %v, want 15", st)
	}
	if s.Remaining() != 1 {
		t.Errorf("Seek(15) 后应剩 1 个事件 (T=20), 实得 %d", s.Remaining())
	}
	fc.Advance(2 * time.Second) // 曲内 +4s (仍 2.0 倍速) → 19s, T=20 事件未到期
	if s.Remaining() != 1 {
		t.Errorf("19s 时 T=20 未到期, Remaining 应为 1")
	}
	fc.Advance(1 * time.Second) // → 21s
	s.Poll()
	if s.Remaining() != 0 {
		t.Errorf("21s 时 T=20 应已分发")
	}
}

func TestSeekFlushesActiveNotes(t *testing.T) {
	fc := &fakeClock{t: time.Now()}
	col := &collector{}
	evs := []Event{
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 60, Vel: 90},
		{T: 100, Kind: KindNoteOff, Ch: 0, Note: 60},
	}
	s := NewScheduler(evs, col.emit)
	s.SetClock(fc.Now)
	s.Start()
	s.Poll()   // NoteOn 发出
	s.Seek(50) // 跳过 100 处的 NoteOff → 活跃音应被补发 NoteOff
	s.Stop()

	gots := col.snapshot()
	offs := 0
	for _, e := range gots {
		if e.Kind == KindNoteOff {
			offs++
		}
	}
	// Seek 补发 1 次 NoteOff; Stop 时活跃音已被 Seek 清空, 不再补发
	// (队列里残留的 T=100 NoteOff 未被分发)
	if offs != 1 {
		t.Errorf("Seek 应补发 1 次 NoteOff, 实得 %d 个: %v", offs, gots)
	}
}

func TestNoteOffReleasesVoice(t *testing.T) {
	fc := &fakeClock{t: time.Now()}
	col := &collector{}
	evs := []Event{
		{T: 0, Kind: KindNoteOn, Ch: 1, Note: 60, Vel: 90},
		{T: 0, Kind: KindNoteOff, Ch: 1, Note: 60},
	}
	s := NewScheduler(evs, col.emit)
	s.SetClock(fc.Now)
	s.VoiceBudget = 1
	s.Start()
	s.Poll()
	// 音已被释放, 同声道的下一个 NoteOn 不应触发挤音
	s.dispatchLocked(Event{Kind: KindNoteOn, Ch: 1, Note: 62, Vel: 90})

	// 先在 Stop flush 之前断言: 不应出现针对 note 62 的挤音 NoteOff
	got := col.snapshot()
	if len(got) != 3 {
		t.Fatalf("应共 3 个事件 (On60, Off60, On62), 实得 %d: %v", len(got), got)
	}
	for i, e := range got {
		if e.Kind == KindNoteOff && e.Note == 62 {
			t.Errorf("事件 #%d: note 62 被误挤音 (NoteOff 应已释放声道 1 的活跃音表)", i)
		}
	}
	s.Stop()
}
