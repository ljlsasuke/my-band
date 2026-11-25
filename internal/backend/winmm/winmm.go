//go:build windows

// Package winmm 封装 Windows 多媒体 MIDI 输出 API (winmm.dll)。
//
// 只包 4 件武器 (PLAN.md 声音线设计):
// midiOutGetNumDevs / midiOutGetDevCapsW 枚举设备,
// midiOutOpen / midiOutShortMsg / midiOutClose 投递指令。
// 声音合成完全委托给系统合成器设备 (GS Wavetable / VirtualMIDISynth)。
package winmm

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modWinmm        = syscall.NewLazyDLL("winmm.dll")
	procNumDevs     = modWinmm.NewProc("midiOutGetNumDevs")
	procGetDevCapsW = modWinmm.NewProc("midiOutGetDevCapsW")
	procOpen        = modWinmm.NewProc("midiOutOpen")
	procClose       = modWinmm.NewProc("midiOutClose")
	procShortMsg    = modWinmm.NewProc("midiOutShortMsg")
)

// MIDI_MAPPER = (UINT_PTR)-1: 打开默认设备映射器。
const MIDI_MAPPER = ^uintptr(0)

// Caps 对应 MIDIOUTCAPSW, MAXPNAMELEN = 32。
type Caps struct {
	ManufacturerID uint16
	ProductID      uint16
	DriverVersion  uint32
	ProductName    [32]uint16
	Technology     uint16
	Voices         uint16 // 合成器复音数 (GS Wavetable = 32)
	Notes          uint16
	ChannelMask    uint16
	Support        uint32
}

// Name 返回设备名。
func (c Caps) Name() string { return syscall.UTF16ToString(c.ProductName[:]) }

func mmErr(what string, r uintptr) error {
	return fmt.Errorf("%s: MMRESULT=%d", what, r)
}

// NumDevs 返回 MIDI 输出设备数。
func NumDevs() int {
	r, _, _ := procNumDevs.Call()
	return int(r)
}

// DevCaps 查询第 deviceID 个设备的描述。
func DevCaps(deviceID int) (Caps, error) {
	var c Caps
	r, _, _ := procGetDevCapsW.Call(uintptr(deviceID), uintptr(unsafe.Pointer(&c)), unsafe.Sizeof(c))
	if r != 0 {
		return Caps{}, mmErr("midiOutGetDevCapsW", r)
	}
	return c, nil
}

// Open 打开设备, 返回句柄。deviceID 用 MIDI_MAPPER 表示系统默认映射。
func Open(deviceID int) (uintptr, error) {
	var h uintptr
	r, _, _ := procOpen.Call(uintptr(unsafe.Pointer(&h)), uintptr(deviceID), 0, 0, 0)
	if r != 0 {
		return 0, mmErr("midiOutOpen", r)
	}
	return h, nil
}

// ShortMsg 投递一条即发即忘的 MIDI 消息 (低 3 字节: status, d1, d2)。
func ShortMsg(h uintptr, status, d1, d2 byte) error {
	msg := uint32(status) | uint32(d1)<<8 | uint32(d2)<<16
	r, _, _ := procShortMsg.Call(h, uintptr(msg))
	if r != 0 {
		return mmErr("midiOutShortMsg", r)
	}
	return nil
}

// Close 关闭句柄。
func Close(h uintptr) error {
	r, _, _ := procClose.Call(h)
	if r != 0 {
		return mmErr("midiOutClose", r)
	}
	return nil
}
