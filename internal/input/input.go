// Package input 键盘输入适配层 (PLAN.md M4.5)。
//
// 抽象出唯一的事件生产者接口 Source, 屏蔽三种来源的差异:
//   - ComputerKeys: 电脑键盘 (三排映射, 固定力度, 仅窗口聚焦时生效)
//   - MidiIn:       MIDI 键盘/电钢琴 (真实力度, 异步回调)
//
// 所有人类演奏事件最终都以 core.Event 进入总线 —— 与 AI 协议事件、
// 谱面回放共用同一套动画与发声管线 (四手联弹即由此而来)。
package input

import (
	"fmt"

	"github.com/ljlsasuke/my-band/internal/core"
)

// Handler 人类演奏事件出口 (cli 接后端发声 + 前端动画)。
// 可能在 winmm 回调线程被调用, 实现须并发安全。
type Handler func(e core.Event)

// Source 键盘输入源。Update 由渲染帧每帧调用。
type Source interface {
	Name() string // 展示名 (键位显示条/日志用)
	Update()      // 轮询/排空待处理事件, 转发给 Handler
	Close()
}

// KeyLabel 一条键位标注。
type KeyLabel struct {
	KeyText string // 键帽 (如 "Z", "Q")
	Note    int    // 对应 MIDI 音符
	Held    bool   // 该物理键当前是否被按下 (高亮用: 按物理键高亮而非按音符)
}

// RowSource 可提供分排键帽 (键帽面板渲染用, FreePiano 式)。
type RowSource interface {
	Source
	Rows() [][]KeyLabel
}

// BaseSetter 计算机键盘支持谱面自适应八度。
type BaseSetter interface{ SetBase(base int) }

// Open 按类型构造输入源。
// kind: "auto" (检测到 MIDI 键盘优先, 否则电脑键盘) / "computer" / "midi"。
// events 用于电脑键盘的谱面自适应八度 (可为 nil)。
func Open(kind string, h Handler, events []core.Event) (Source, string, error) {
	switch kind {
	case "computer":
		return newComputer(h, events)
	case "midi":
		m, err := NewMidiIn("auto", h)
		if err != nil {
			return nil, "", err
		}
		return m, m.Name(), nil
	case "auto", "":
		if _, _, ok := DetectMidiIn(); ok {
			m, err := NewMidiIn("auto", h)
			if err != nil {
				return nil, "", err
			}
			return m, m.Name(), nil
		}
		return newComputer(h, events)
	}
	return nil, "", fmt.Errorf("未知输入源类型 %q (auto|computer|midi)", kind)
}

func newComputer(h Handler, events []core.Event) (Source, string, error) {
	c := NewComputerKeys(60, h)
	if len(events) > 0 {
		c.SetBase(AutoBase(events))
	}
	return c, c.Name() + fmt.Sprintf(" (基准 %s, -/= 切八度)", noteC(c.Base())), nil
}

func noteC(base int) string { return noteName(base) }

// noteName 本地实现避免反向依赖 score 包。
func noteName(n int) string {
	names := [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	return names[n%12] + itoa(n/12-1)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [8]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
