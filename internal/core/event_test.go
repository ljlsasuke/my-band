package core

import "testing"

// 测试策略 (PLAN.md M0-④): 表驱动, 用例直译前身项目 auto-piano 的自测习惯。
func TestEventValidate(t *testing.T) {
	valid := []Event{
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 60, Vel: 100},
		{T: 1.5, Kind: KindNoteOff, Ch: 9, Note: 36, Vel: 0},
		{T: 2, Kind: KindProgram, Ch: 0, Prog: 73},
		{T: 2, Kind: KindControl, Ch: 0, Ctrl: 64, CtrlVal: 127},
		{T: 0, Kind: KindTempo, Ch: 0},
		{T: 0, Kind: KindMarker, Text: "verse-1"},
	}
	for i, e := range valid {
		if err := e.Validate(); err != nil {
			t.Errorf("合法事件 #%d 被误判: %v (%s)", i, err, e.Kind)
		}
	}

	invalid := []Event{
		{T: -0.1, Kind: KindNoteOn, Ch: 0, Note: 60, Vel: 100},   // 时间越界
		{T: 0, Kind: KindNoteOn, Ch: 16, Note: 60, Vel: 100},     // 声道越界
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 128, Vel: 100},     // 音符越界
		{T: 0, Kind: KindNoteOn, Ch: 0, Note: 60, Vel: 128},      // 力度越界
		{T: 0, Kind: KindProgram, Ch: 0, Prog: 128},              // 音色越界
		{T: 0, Kind: KindControl, Ch: 0, Ctrl: 128, CtrlVal: 1},  // 控制器越界
		{T: 0, Kind: KindControl, Ch: 0, Ctrl: 64, CtrlVal: 128}, // 控制器值越界
	}
	for i, e := range invalid {
		if err := e.Validate(); err == nil {
			t.Errorf("非法事件 #%d 未被拦截: %s ch=%d note=%d vel=%d prog=%d ctrl=%d",
				i, e.Kind, e.Ch, e.Note, e.Vel, e.Prog, e.Ctrl)
		}
	}
}

func TestKindString(t *testing.T) {
	tests := map[Kind]string{
		KindNoteOn:  "note_on",
		KindNoteOff: "note_off",
		KindProgram: "program",
		KindControl: "control",
		KindTempo:   "tempo",
		KindMarker:  "marker",
	}
	for k, want := range tests {
		if got := k.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", uint8(k), got, want)
		}
	}
}
