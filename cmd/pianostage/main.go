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
	"time"

	"github.com/ljlsasuke/pianostage/internal/backend"
	"github.com/ljlsasuke/pianostage/internal/protocol"
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
	case "play", "serve":
		err = fmt.Errorf("%s: 未实现 (M1/M3 里程碑, 见 PLAN.md)", os.Args[1])
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
  pianostage play <文件>          (M1) 播放谱面
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
