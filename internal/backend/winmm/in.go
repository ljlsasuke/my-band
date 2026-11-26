//go:build windows

package winmm

import (
	"syscall"
	"unsafe"

	"github.com/ljlsasuke/my-band/internal/core"
)

// MIDI 输入端 (midiIn*): 与输出端对称的另一半。
// 短消息 (note on/off, CC) 经 CALLBACK_FUNCTION 异步送达 —— winmm 用它自己的
// 线程回调, Go 侧用 syscall.NewCallback 适配; 回调里只做入队, 不做任何重活。

var (
	procInNumDevs     = modWinmm.NewProc("midiInGetNumDevs")
	procInGetDevCapsW = modWinmm.NewProc("midiInGetDevCapsW")
	procInOpen        = modWinmm.NewProc("midiInOpen")
	procInStart       = modWinmm.NewProc("midiInStart")
	procInStop        = modWinmm.NewProc("midiInStop")
	procInReset       = modWinmm.NewProc("midiInReset")
	procInClose       = modWinmm.NewProc("midiInClose")
)

// MM_MIM_DATA: 回调收到的短消息类型。
const MIM_DATA = 0x3C3

// CALLBACK_FUNCTION 标志位。
const CALLBACK_FUNCTION = 0x30000

// inCallbackHandler 当前打开设备的消息出口 (同一时刻至多一个设备)。
// 在 midiInOpen 前设置; 回调线程与 Go 主线程并发, 由使用方保证安全。
var inCallbackHandler func(dwParam1 uintptr)

// midiInCallback winmm 回调入口 (syscall.NewCallback 要求 uintptr 签名)。
// 过滤出短消息后转发; 返回 0 表示已处理。
func midiInCallback(hMidiIn, wMsg, dwInstance, dwParam1, dwParam2 uintptr) uintptr {
	if wMsg == MIM_DATA && inCallbackHandler != nil {
		inCallbackHandler(dwParam1)
	}
	return 0
}

var inCallbackPtr = syscall.NewCallback(midiInCallback)

// InNumDevs 返回 MIDI 输入设备数 (MIDI 键盘等)。
func InNumDevs() int {
	r, _, _ := procInNumDevs.Call()
	return int(r)
}

// InDevCaps 查询第 deviceID 个输入设备的描述。
func InDevCaps(deviceID int) (Caps, error) {
	var c Caps
	r, _, _ := procInGetDevCapsW.Call(uintptr(deviceID), uintptr(unsafe.Pointer(&c)), unsafe.Sizeof(c))
	if r != 0 {
		return Caps{}, mmErr("midiInGetDevCapsW", r)
	}
	return c, nil
}

// InName 设备名。
func InName(c Caps) string { return c.Name() }

// InOpen 打开输入设备并注册消息回调 (短消息异步到达 onMsg)。
// deviceID 用 MIDI_MAPPER 表示系统默认。开始接收需再调 InStart。
func InOpen(deviceID int, onMsg func(dwParam1 uintptr)) (uintptr, error) {
	inCallbackHandler = onMsg
	var h uintptr
	r, _, _ := procInOpen.Call(
		uintptr(unsafe.Pointer(&h)),
		uintptr(deviceID),
		inCallbackPtr,
		0,
		CALLBACK_FUNCTION,
	)
	if r != 0 {
		inCallbackHandler = nil
		return 0, mmErr("midiInOpen", r)
	}
	return h, nil
}

// InStart 开始接收。
func InStart(h uintptr) error {
	r, _, _ := procInStart.Call(h)
	if r != 0 {
		return mmErr("midiInStart", r)
	}
	return nil
}

// InStop 停止接收。
func InStop(h uintptr) error {
	r, _, _ := procInStop.Call(h)
	if r != 0 {
		return mmErr("midiInStop", r)
	}
	return nil
}

// InReset 丢弃未处理数据 (Close 前必须调, 否则 midiInClose 可能挂起)。
func InReset(h uintptr) error {
	r, _, _ := procInReset.Call(h)
	if r != 0 {
		return mmErr("midiInReset", r)
	}
	return nil
}

// InClose 关闭设备。
func InClose(h uintptr) error {
	r, _, _ := procInClose.Call(h)
	if r != 0 {
		return mmErr("midiInClose", r)
	}
	return nil
}

// ParseShortMsg 解析 winmm 短消息 (dwParam1 = status | d1<<8 | d2<<16)。
// 只关心音符与延音踏板; 其他控制器/系统消息忽略。声道保留原始值。
func ParseShortMsg(p uintptr) (e core.Event, ok bool) {
	status := byte(p)
	d1 := byte(p >> 8)
	d2 := byte(p >> 16)
	hi := status & 0xF0
	ch := int(status & 0x0F)
	switch hi {
	case 0x90:
		if d2 > 0 {
			return core.Event{Kind: core.KindNoteOn, Ch: ch, Note: int(d1), Vel: int(d2)}, true
		}
		return core.Event{Kind: core.KindNoteOff, Ch: ch, Note: int(d1)}, true
	case 0x80:
		return core.Event{Kind: core.KindNoteOff, Ch: ch, Note: int(d1)}, true
	case 0xB0:
		return core.Event{Kind: core.KindControl, Ch: ch, Ctrl: int(d1), CtrlVal: int(d2)}, true
	}
	return core.Event{}, false
}
