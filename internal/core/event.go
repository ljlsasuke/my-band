// Package core 定义 My Band 的核心事件模型、时钟与调度器。
//
// 设计原则 (PLAN.md §3): 一切皆事件流——谱面解析、现场演奏、AI 协议
// 最终都归一化为 Event, 走同一条总线; 前端动画与后端发声都是下游消费者。
package core

import "fmt"

// Kind 事件类型。
type Kind uint8

const (
	KindNoteOn  Kind = iota + 1 // 按下音符
	KindNoteOff                 // 释放音符
	KindProgram                 // 音色变更 (GM Program Change)
	KindControl                 // 控制器 (CC: 64=延音踏板, 123=全部音符关)
	KindTempo                   // 速度变更 (BPM)
	KindMarker                  // 文本标记
)

func (k Kind) String() string {
	switch k {
	case KindNoteOn:
		return "note_on"
	case KindNoteOff:
		return "note_off"
	case KindProgram:
		return "program"
	case KindControl:
		return "control"
	case KindTempo:
		return "tempo"
	case KindMarker:
		return "marker"
	}
	return fmt.Sprintf("kind(%d)", uint8(k))
}

// Event 是总线上的单一事实源。
//
// T 为曲内时间轴 (秒), 由解析层从 MIDI tick 经 tempo map 换算而来;
// 现场演奏 / 协议即时事件的 T 为 0 (立即生效)。
type Event struct {
	T       float64 // 曲内时间 (秒), >= 0
	Kind    Kind
	Ch      int    // 声道 0~15, 9 = GM 打击乐
	Note    int    // 音符 0~127 (FreePiano 音域 A0~C8 即 21~108, 由解析层裁剪)
	Vel     int    // 力度 0~127
	Prog    int    // Program: GM 乐器编号 0~127
	Ctrl    int    // Control: 控制器编号
	CtrlVal int    // Control: 控制器值 0~127
	Text    string // Marker 文本
}

// Validate 校验事件字段范围, 违规返回错误。
func (e Event) Validate() error {
	if e.T < 0 {
		return fmt.Errorf("时间轴越界: T=%.3f", e.T)
	}
	if e.Ch < 0 || e.Ch > 15 {
		return fmt.Errorf("声道越界: ch=%d", e.Ch)
	}
	switch e.Kind {
	case KindNoteOn, KindNoteOff:
		if e.Note < 0 || e.Note > 127 {
			return fmt.Errorf("音符越界: note=%d", e.Note)
		}
		if e.Vel < 0 || e.Vel > 127 {
			return fmt.Errorf("力度越界: vel=%d", e.Vel)
		}
	case KindProgram:
		if e.Prog < 0 || e.Prog > 127 {
			return fmt.Errorf("音色越界: prog=%d", e.Prog)
		}
	case KindControl:
		if e.Ctrl < 0 || e.Ctrl > 127 {
			return fmt.Errorf("控制器越界: ctrl=%d", e.Ctrl)
		}
		if e.CtrlVal < 0 || e.CtrlVal > 127 {
			return fmt.Errorf("控制器值越界: ctrlval=%d", e.CtrlVal)
		}
	}
	return nil
}
