package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ljlsasuke/pianostage/internal/backend"
	"github.com/ljlsasuke/pianostage/internal/core"
	"github.com/ljlsasuke/pianostage/internal/frontend"
	"github.com/ljlsasuke/pianostage/internal/library"
	"github.com/ljlsasuke/pianostage/internal/protocol"
	"github.com/ljlsasuke/pianostage/internal/score"
)

// cmdServe AP/1 协议服务 (M3): JSONL over stdio, 规范见 docs/AP1.md。
//
// 状态机: load(谱面) → play/pause/stop/seek/tempo → note/program/cc 即时事件。
// 后端惰性打开: 无 MIDI 设备也能起服务 (协议流程可测, 发声按需报错)。
func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	device := fs.String("device", "auto", "设备: auto | 序号 | 名称")
	show := fs.Bool("show", false, "同时驱动前端动画 (音乐 + 动画)")
	fs.Parse(args)

	p := &player{device: *device, backend: &backend.MidiOut{}}
	if *show {
		p.stage = frontend.New()
		p.stage.SetClock(p.songTime)
	}

	// stdout 帧发送 (主循环与时钟广播并发, 加锁)
	out := &frameWriter{enc: json.NewEncoder(os.Stdout)}
	send := func(m protocol.Message) { out.write(m) }
	send(protocol.Hello())

	// 主时钟广播 (~20Hz, 仅播放中)
	done := make(chan struct{})
	defer close(done)
	go func() {
		tk := time.NewTicker(50 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-done:
				return
			case <-tk.C:
				if t, playing := p.songTimePlaying(); playing {
					send(protocol.TimeTick(t))
				}
			}
		}
	}()

	// 命令循环
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		var cmd protocol.Command
		if err := json.Unmarshal(line, &cmd); err != nil {
			send(protocol.Err("", protocol.ErrProto, "非法 JSON: "+err.Error()))
			continue
		}
		reply := p.handle(cmd)
		send(reply)
		if cmd.Op == "quit" && reply.Type == "ok" {
			return nil
		}
	}
	// stdin 关闭 (客户端断开) → 清理退出
	p.mu.Lock()
	p.stopLocked()
	p.mu.Unlock()
	if p.opened {
		_ = p.backend.AllOff()
		p.backend.Close()
		p.opened = false
	}
	return nil
}

// ---- 播放器状态机 ----

type player struct {
	mu      sync.Mutex
	device  string
	backend *backend.MidiOut
	opened  bool
	events  []core.Event
	sched   *core.Scheduler
	loaded  bool
	stage   *frontend.Stage
}

func (p *player) songTime() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sched == nil {
		return 0
	}
	return p.sched.SongTime()
}

func (p *player) songTimePlaying() (float64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sched == nil || !p.runningLocked() || p.pausedLocked() {
		return 0, false
	}
	return p.sched.SongTime(), true
}

func (p *player) runningLocked() bool { return p.sched != nil && p.loaded }
func (p *player) pausedLocked() bool  { return p.sched != nil && p.sched.Paused() }

// ensureBackend 惰性打开后端 (协议流程无设备也可测; 发声时才报错)。
func (p *player) ensureBackend() error {
	if p.opened {
		return nil
	}
	if err := p.backend.Open(p.device); err != nil {
		return err
	}
	p.opened = true
	return nil
}

// dispatch 音频出口: 调度器事件与即时事件共用 (动画已在 emit 里 Push)。
func (p *player) dispatch(e core.Event) {
	if err := p.ensureBackend(); err != nil {
		fmt.Fprintln(os.Stderr, "后端不可用, 跳过发声:", err)
		return
	}
	var err error
	switch e.Kind {
	case core.KindNoteOn:
		err = p.backend.NoteOn(e.Ch, e.Note, e.Vel)
	case core.KindNoteOff:
		err = p.backend.NoteOff(e.Ch, e.Note)
	case core.KindProgram:
		err = p.backend.Program(e.Ch, e.Prog)
	case core.KindControl:
		err = p.backend.Control(e.Ch, e.Ctrl, e.CtrlVal)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "投递失败:", err)
	}
}

// handle 命令分发 → 回执。
func (p *player) handle(cmd protocol.Command) protocol.Message {
	switch cmd.Op {
	case "ping":
		return protocol.OK("ping")

	case "list":
		names, err := library.List()
		if err != nil {
			return protocol.Err("list", protocol.ErrInternal, err.Error())
		}
		m := protocol.OK("list")
		m.Data = names
		return m

	case "load":
		path, err := library.Resolve(cmd.Source)
		if err != nil {
			return protocol.Err("load", protocol.ErrLoad, err.Error())
		}
		events, err := score.Load(path, "C", score.DefaultBeat, score.DefaultVelocity)
		if err != nil {
			return protocol.Err("load", protocol.ErrLoad, err.Error())
		}
		if len(events) == 0 {
			return protocol.Err("load", protocol.ErrLoad, "谱面解析结果为空")
		}
		p.mu.Lock()
		p.stopLocked()
		p.events = events
		p.loaded = true
		duration := events[len(events)-1].T
		p.mu.Unlock()
		m := protocol.OK("load")
		m.Data = map[string]any{"notes": countNotes(events), "duration": duration, "path": path}
		return m

	case "play":
		p.mu.Lock()
		defer p.mu.Unlock()
		if !p.loaded {
			return protocol.Err("play", protocol.ErrState, "未加载谱面 (先 load)")
		}
		if p.sched == nil {
			p.sched = p.newSchedLocked()
		}
		p.sched.Start()
		return protocol.OK("play")

	case "pause":
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.sched == nil {
			return protocol.Err("pause", protocol.ErrState, "未在播放")
		}
		p.sched.Pause()
		return protocol.OK("pause")

	case "stop":
		p.mu.Lock()
		defer p.mu.Unlock()
		p.stopLocked()
		return protocol.OK("stop")

	case "seek":
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.sched == nil {
			return protocol.Err("seek", protocol.ErrState, "未在播放")
		}
		p.sched.Seek(cmd.Position)
		return protocol.OK("seek")

	case "tempo":
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.sched == nil {
			return protocol.Err("tempo", protocol.ErrState, "未在播放")
		}
		if cmd.Tempo <= 0 {
			return protocol.Err("tempo", protocol.ErrRange, "速率倍率必须 > 0")
		}
		p.sched.SetRate(cmd.Tempo)
		return protocol.OK("tempo")

	case "note":
		p.mu.Lock()
		t := 0.0
		if p.sched != nil {
			t = p.sched.SongTime()
		}
		p.mu.Unlock()
		if cmd.Note < 0 || cmd.Note > 127 || cmd.Vel < 1 || cmd.Vel > 127 {
			return protocol.Err("note", protocol.ErrRange, "note/vel 越界")
		}
		if p.stage != nil { // 即时事件: 以当前曲内时间进动画队列
			kind := core.KindNoteOff
			if cmd.On {
				kind = core.KindNoteOn
			}
			p.stage.Push(core.Event{T: t, Kind: kind, Ch: cmd.Ch, Note: cmd.Note, Vel: cmd.Vel})
		}
		if err := p.ensureBackend(); err != nil {
			return protocol.Err("note", protocol.ErrDevice, err.Error())
		}
		var err error
		if cmd.On {
			err = p.backend.NoteOn(cmd.Ch, cmd.Note, cmd.Vel)
		} else {
			err = p.backend.NoteOff(cmd.Ch, cmd.Note)
		}
		if err != nil {
			return protocol.Err("note", protocol.ErrDevice, err.Error())
		}
		return protocol.OK("note")

	case "program":
		if err := p.ensureBackend(); err != nil {
			return protocol.Err("program", protocol.ErrDevice, err.Error())
		}
		if err := p.backend.Program(cmd.Ch, cmd.Prog); err != nil {
			return protocol.Err("program", protocol.ErrDevice, err.Error())
		}
		return protocol.OK("program")

	case "cc":
		if err := p.ensureBackend(); err != nil {
			return protocol.Err("cc", protocol.ErrDevice, err.Error())
		}
		if err := p.backend.Control(cmd.Ch, cmd.Ctrl, cmd.CtrlVal); err != nil {
			return protocol.Err("cc", protocol.ErrDevice, err.Error())
		}
		return protocol.OK("cc")

	case "backend":
		p.mu.Lock()
		defer p.mu.Unlock()
		p.stopLocked()
		if p.opened {
			p.backend.Close()
			p.opened = false
		}
		p.device = cmd.Device
		if err := p.ensureBackend(); err != nil {
			return protocol.Err("backend", protocol.ErrDevice, err.Error())
		}
		m := protocol.OK("backend")
		m.Data = map[string]any{"device": p.deviceName()}
		return m

	case "query":
		return p.query(cmd.Query)

	case "quit":
		p.mu.Lock()
		p.stopLocked()
		p.mu.Unlock()
		if p.opened {
			_ = p.backend.AllOff()
			p.backend.Close()
			p.opened = false
		}
		return protocol.OK("quit")
	}
	return protocol.Err(cmd.Op, protocol.ErrOp, "未知 op")
}

// newSchedLocked 为当前已加载谱面创建调度器 (emit → 前端 Push + 音频 dispatch)。
// 调用方持有 p.mu。
func (p *player) newSchedLocked() *core.Scheduler {
	return core.NewScheduler(p.events, func(e core.Event) {
		if p.stage != nil {
			p.stage.Push(e)
		}
		p.dispatch(e)
	})
}

// stopLocked 停止播放并复位调度器 (下次 play 从头开始)。
func (p *player) stopLocked() {
	if p.sched != nil {
		p.sched.Stop()
		p.sched = nil
	}
}

func (p *player) deviceName() string {
	if !p.opened {
		return ""
	}
	if d, err := p.backend.CurrentDevice(); err == nil {
		return d
	}
	return p.device
}

func (p *player) query(q string) protocol.Message {
	switch q {
	case "devices":
		b := &backend.MidiOut{}
		devs, err := b.Devices()
		if err != nil {
			return protocol.Err("query", protocol.ErrDevice, err.Error())
		}
		m := protocol.OK("query")
		m.Data = devs
		return m
	case "time":
		m := protocol.OK("query")
		m.Data = map[string]any{"t": p.songTime()}
		return m
	case "status":
		p.mu.Lock()
		defer p.mu.Unlock()
		data := map[string]any{
			"loaded":   p.loaded,
			"playing":  p.loaded && p.sched != nil,
			"paused":   p.sched != nil && p.sched.Paused(),
			"backend":  "midiout",
			"device":   p.deviceName(),
			"songtime": 0.0,
		}
		if p.sched != nil {
			data["songtime"] = p.sched.SongTime()
		}
		if p.loaded {
			data["notes"] = countNotes(p.events)
			data["duration"] = p.events[len(p.events)-1].T
		}
		m := protocol.OK("query")
		m.Data = data
		return m
	}
	return protocol.Err("query", protocol.ErrRange, "未知查询项: "+q+" (devices|status|time)")
}

func countNotes(events []core.Event) int {
	n := 0
	for _, e := range events {
		if e.Kind == core.KindNoteOn {
			n++
		}
	}
	return n
}

// frameWriter stdout 帧写 (并发安全)。
type frameWriter struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func (w *frameWriter) write(m protocol.Message) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.enc.Encode(m)
}
