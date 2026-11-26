package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/ljlsasuke/my-band/internal/backend"
	"github.com/ljlsasuke/my-band/internal/core"
	"github.com/ljlsasuke/my-band/internal/frontend"
	"github.com/ljlsasuke/my-band/internal/input"
)

// cmdFreePlay 自由弹奏模式 (PLAN.md 演奏模式矩阵: "打开应用 = 自由弹奏")。
// 裸调用 my-band (无参数/双击 exe) 直接进这里: 无谱面、无协议、无菜单,
// 打开就是一架琴 —— 电脑键盘/MIDI 键盘演奏, 关窗退出。
func cmdFreePlay() error {
	releaseInstance, already, err0 := acquireSingleInstance()
	if err0 != nil {
		return err0
	}
	if already {
		return fmt.Errorf("My Band 已有实例在运行 (占用音频设备)——请先关闭它, 或直接使用那个窗口")
	}
	defer releaseInstance()

	b := &backend.MidiOut{}
	if err := b.Open("auto"); err != nil {
		return err
	}
	defer b.Close()

	stage := frontend.New()
	start := time.Now()
	// 自由弹奏没有曲内时间轴, 动画时钟 = 墙钟
	stage.SetClock(func() float64 { return time.Since(start).Seconds() })

	// 人类演奏事件: 后端发声 + 动画队列 (与 play/serve 共用同一管线)
	src, srcName, err := input.Open("auto", func(e core.Event) {
		var err error
		switch e.Kind {
		case core.KindNoteOn:
			err = b.NoteOn(e.Ch, e.Note, e.Vel)
		case core.KindNoteOff:
			err = b.NoteOff(e.Ch, e.Note)
		case core.KindControl:
			err = b.Control(e.Ch, e.Ctrl, e.CtrlVal)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "演奏投递失败:", err)
		}
		e.T = time.Since(start).Seconds()
		stage.Push(e)
	}, nil)
	if err != nil {
		return err
	}
	defer src.Close()
	stage.SetInputSource(src)
	fmt.Println("自由弹奏模式 · 输入源:", srcName, "(Tab 显示/隐藏键位)")

	if err := stage.Run("My Band — 自由弹奏 (Tab 键位 · -/= 八度)"); err != nil {
		return err
	}
	stage.Clear()
	_ = b.AllOff() // 关窗时把悬着的音放掉
	fmt.Println("自由弹奏结束 ✓")
	return nil
}
