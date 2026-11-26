//go:build windows

package winmm

import "testing"

func TestParseShortMsg(t *testing.T) {
	// note on: 0x90|ch, note, vel
	e, ok := ParseShortMsg(uintptr(0x90 | 0x08 | 60<<8 | 100<<16))
	if !ok || e.Kind != 1 || e.Ch != 8 || e.Note != 60 || e.Vel != 100 {
		t.Errorf("note on 解析错误: %+v ok=%v", e, ok)
	}
	// note on vel=0 → note off
	e, ok = ParseShortMsg(uintptr(0x90 | 60<<8))
	if !ok || e.Kind != 2 || e.Note != 60 {
		t.Errorf("vel=0 应解析为 note off: %+v", e)
	}
	// note off
	e, ok = ParseShortMsg(uintptr(0x80 | 42<<8))
	if !ok || e.Kind != 2 || e.Note != 42 {
		t.Errorf("note off 解析错误: %+v", e)
	}
	// CC (延音踏板)
	e, ok = ParseShortMsg(uintptr(0xB0 | 64<<8 | 127<<16))
	if !ok || e.Kind != 4 || e.Ctrl != 64 || e.CtrlVal != 127 {
		t.Errorf("CC 解析错误: %+v", e)
	}
	// 其他消息忽略
	if _, ok := ParseShortMsg(uintptr(0xC0 | 5<<8)); ok {
		t.Error("program change 不该由输入解析产生事件")
	}
}
