package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ljlsasuke/pianostage/internal/backend"
	"github.com/ljlsasuke/pianostage/internal/core"
	"github.com/ljlsasuke/pianostage/internal/frontend"
	"github.com/ljlsasuke/pianostage/internal/library"
	"github.com/ljlsasuke/pianostage/internal/score"
)

// cmdPlay M1 主路径: 解析谱面 → 调度器按主时钟前瞻分发 → winmm 出声。
// 这是「验收标准」的直接实现: 命运交响曲分声部音色正确、无削波杂音、
// VirtualMIDISynth 自动识别 (PLAN.md 里程碑 M1)。
func cmdPlay(args []string) error {
	fs := flag.NewFlagSet("play", flag.ExitOnError)
	device := fs.String("device", "auto", "设备: auto | 序号 | 名称")
	maxSec := fs.Float64("max-sec", 0, "只演奏前 N 秒 (预览)")
	lookAhead := fs.Int("look-ahead-ms", int(core.DefaultLookAhead/time.Millisecond), "前瞻窗口 (毫秒)")
	show := fs.Bool("show", false, "显示演奏窗口 (Ebiten 前端, M2)")
	key := fs.String("key", "C", "简谱的 1 = 哪个音 (默认 C)")
	if len(args) == 0 {
		return fmt.Errorf("用法: pianostage play <文件.mid> [--device auto] [--max-sec N]")
	}
	// Go flag 包遇到首个位置参数即停, 手动分离 flags 与文件路径,
	// 使 `play 文件 --max-sec N` 与 `play --max-sec N 文件` 均可用
	var flagArgs, positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(args[i], "-") {
			flagArgs = append(flagArgs, args[i])
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
		} else {
			positional = append(positional, args[i])
		}
	}
	if fs.Parse(flagArgs) != nil || len(positional) != 1 {
		return fmt.Errorf("用法: pianostage play <文件.mid> [--device auto] [--max-sec N]")
	}

	// 1. 解析: 裸曲名走曲库 (可省扩展名), .mid/.txt 按格式分发 (M3 能力接入 play)
	path, err := library.Resolve(positional[0])
	if err != nil {
		return err
	}
	events, err := score.Load(path, *key, score.DefaultBeat, score.DefaultVelocity)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return fmt.Errorf("谱面解析结果为空")
	}
	total := events[len(events)-1].T
	if *maxSec > 0 {
		filtered := events[:0]
		for _, e := range events {
			if e.T <= *maxSec { // 越界的 NoteOff 丢弃, 由调度器 Stop flush 兜底
				filtered = append(filtered, e)
			}
		}
		events = filtered
		total = *maxSec
	}
	fmt.Printf("解析完成: %d 个事件, 时长约 %.1fs\n", len(events), total)

	// 2. 后端
	b := &backend.MidiOut{}
	if err := b.Open(*device); err != nil {
		return err
	}
	defer b.Close()

	// 3. 调度: 事件直接映射为后端调用 (Tempo/Marker 对发声无意义, 忽略);
	// --show 时同一事件流分发给前端动画队列 (音频主时钟, 动画追帧)
	var stage *frontend.Stage
	// 谱面事件: 调度器只管发声; 动画由 stage 自驱事件流 (1.5s 前瞻)
	s := core.NewScheduler(events, func(e core.Event) {
		var err error
		switch e.Kind {
		case core.KindNoteOn:
			err = b.NoteOn(e.Ch, e.Note, e.Vel)
		case core.KindNoteOff:
			err = b.NoteOff(e.Ch, e.Note)
		case core.KindProgram:
			err = b.Program(e.Ch, e.Prog)
		case core.KindControl:
			err = b.Control(e.Ch, e.Ctrl, e.CtrlVal)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "投递失败:", err)
		}
	})
	s.LookAhead = time.Duration(*lookAhead) * time.Millisecond
	if *show {
		stage = frontend.New()
		stage.SetEvents(events)
		var playing atomic.Bool
		startWall := time.Now()
		clock := func() float64 {
			if playing.Load() {
				return s.SongTime()
			}
			return time.Since(startWall).Seconds() // 空闲时动画走墙钟
		}
		stage.SetClock(clock)
		stage.SetInputHandler(func(note int, on bool, vel int) {
			var err error
			if on {
				err = b.NoteOn(0, note, vel)
			} else {
				err = b.NoteOff(0, note)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "人类演奏投递失败:", err)
			}
			kind := core.KindNoteOff
			if on {
				kind = core.KindNoteOn
			}
			stage.Push(core.Event{T: clock(), Kind: kind, Ch: 0, Note: note, Vel: vel})
		})
		playing.Store(true)
	}
	start := time.Now()
	s.Start()

	if stage != nil {
		// M2 可视模式: GUI 占主 goroutine (Ebiten 要求), 调度器/后端在
		// 自己的 goroutine 里继续跑; 用户关窗 → 停止演奏
		if err := stage.Run("PianoStage — " + positional[0]); err != nil {
			return err
		}
		s.Stop()
		stage.Clear()
		_ = b.AllOff()
		fmt.Printf("演奏结束 (窗口关闭) ✓ 实际耗时 %.1fs\n", time.Since(start).Seconds())
		return nil
	}

	// 4. 等待: 全部事件分发完 (含前瞻) 再留出尾部余量
	for s.Remaining() > 0 {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond) // 尾音余量
	s.Stop()
	_ = b.AllOff() // 双保险

	fmt.Printf("演奏完成 ✓ (实际耗时 %.1fs)\n", time.Since(start).Seconds())
	return nil
}
