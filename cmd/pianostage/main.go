// PianoStage —— 可被 AI 驱动、也可被人类演奏的琴曲舞台。
//
// M0 命令: version / doctor / test-note; serve / play 于 M1/M3 实现。
// 设计与里程碑见仓库根 PLAN.md。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ljlsasuke/pianostage/internal/backend"
	"github.com/ljlsasuke/pianostage/internal/core"
	"github.com/ljlsasuke/pianostage/internal/protocol"
	"github.com/ljlsasuke/pianostage/internal/score"
)

const version = "0.1.0-m0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "version":
		err = cmdVersion(os.Args[2:])
	case "doctor":
		err = cmdDoctor(os.Args[2:])
	case "test-note":
		err = cmdTestNote(os.Args[2:])
	case "dump":
		err = cmdDump(os.Args[2:])
	case "play":
		err = cmdPlay(os.Args[2:])
	case "serve":
		err = fmt.Errorf("serve: 未实现 (M3 里程碑, 见 PLAN.md)")
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `PianoStage — 可被 AI 驱动的琴曲舞台

用法:
  pianostage version [-json]      打印版本 (skill 做版本协商用)
  pianostage doctor               环境自检: 设备枚举/自动选择/复音数
  pianostage test-note [-device 自动|序号|名称]  试弹一个中央 C
  pianostage dump <文件>           调试: 逐行输出解析后的事件 (JSONL)
  pianostage play <文件>           播放 MIDI 谱面 (headless, 走系统 GM 音源)
     [--device auto|序号|名称] [--max-sec N] [--look-ahead-ms N]
  pianostage serve                (M3) AP/1 协议服务, JSONL over stdio
`)
}

func cmdVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "输出 JSON (协议协商用)")
	fs.Parse(args)
	if *asJSON {
		b, _ := json.Marshal(map[string]string{"version": version, "ap": fmt.Sprint(protocol.Version)})
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("pianostage %s (AP/%d)\n", version, protocol.Version)
	return nil
}

func cmdDoctor(args []string) error {
	fmt.Printf("== PianoStage doctor ==\n")
	fmt.Printf("[OK] pianostage %s (AP/%d)\n", version, protocol.Version)

	b := &backend.MidiOut{}
	devs, err := b.Devices()
	if err != nil {
		return fmt.Errorf("MIDI 设备枚举失败: %w", err)
	}
	fmt.Printf("[--] MIDI 输出设备 %d 个:\n", len(devs))
	for _, d := range devs {
		fmt.Printf("     %d: %s\n", d.ID, d.Name)
	}
	if len(devs) == 0 {
		return fmt.Errorf("无任何 MIDI 输出设备 (可安装 VirtualMIDISynth)")
	}

	pick, err := backend.PickAuto(devs)
	if err != nil {
		return err
	}
	fmt.Printf("[OK] auto 设备选择: %s\n", pick.Name)
	if pick.Name != devs[0].Name {
		fmt.Printf("[--] 提示: 当前音源为系统兜底 (32 复音)。安装 VirtualMIDISynth + SoundFont 可显著提升音质\n")
	}
	fmt.Println("== 自检结束 ==")
	return nil
}

func cmdTestNote(args []string) error {
	fs := flag.NewFlagSet("test-note", flag.ExitOnError)
	device := fs.String("device", "auto", "设备: auto | 序号 | 名称")
	fs.Parse(args)

	b := &backend.MidiOut{}
	if err := b.Open(*device); err != nil {
		return err
	}
	defer b.Close()

	if err := b.Program(0, 0); err != nil { // 大钢琴
		return err
	}
	fmt.Println("中央 C ...")
	if err := b.NoteOn(0, 60, 90); err != nil {
		return err
	}
	time.Sleep(600 * time.Millisecond)
	if err := b.NoteOff(0, 60); err != nil {
		return err
	}
	fmt.Println("OK: 链路正常")
	return nil
}

type dumpEvent struct {
	T       float64 `json:"t"`
	Kind    string  `json:"kind"`
	Ch      int     `json:"ch"`
	Note    int     `json:"note,omitempty"`
	Vel     int     `json:"vel,omitempty"`
	Prog    int     `json:"prog,omitempty"`
	Ctrl    int     `json:"ctrl,omitempty"`
	CtrlVal int     `json:"ctrlval,omitempty"`
}

// cmdDump 调试用: 输出解析后的完整事件流, 供交叉验证 (如与前身 Python
// 解析器逐音符 diff) 与人工检查。
func cmdDump(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("用法: pianostage dump <文件.mid>")
	}
	events, err := score.ParseMidi(args[0])
	if err != nil {
		return err
	}
	w := json.NewEncoder(os.Stdout)
	for _, e := range events {
		if err := w.Encode(dumpEvent{
			T: e.T, Kind: e.Kind.String(), Ch: e.Ch, Note: e.Note,
			Vel: e.Vel, Prog: e.Prog, Ctrl: e.Ctrl, CtrlVal: e.CtrlVal,
		}); err != nil {
			return err
		}
	}
	return nil
}

// cmdPlay M1 主路径: 解析谱面 → 调度器按主时钟前瞻分发 → winmm 出声。
// 这是「验收标准」的直接实现: 命运交响曲分声部音色正确、无削波杂音、
// VirtualMIDISynth 自动识别 (PLAN.md 里程碑 M1)。
func cmdPlay(args []string) error {
	fs := flag.NewFlagSet("play", flag.ExitOnError)
	device := fs.String("device", "auto", "设备: auto | 序号 | 名称")
	maxSec := fs.Float64("max-sec", 0, "只演奏前 N 秒 (预览)")
	lookAhead := fs.Int("look-ahead-ms", int(core.DefaultLookAhead/time.Millisecond), "前瞻窗口 (毫秒)")
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

	// 1. 解析
	events, err := score.ParseMidi(positional[0])
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

	// 3. 调度: 事件直接映射为后端调用 (Tempo/Marker 对发声无意义, 忽略)
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
	start := time.Now()
	s.Start()

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
