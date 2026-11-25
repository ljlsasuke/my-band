// Package backend 定义可插拔声源接口 (PLAN.md §5.2 / ADR-3)。
//
// 后端只负责"响": 接收总线事件并投递给声源; 何时发 (调度) 属于 core,
// 怎么出声 (合成) 委托给系统设备——本项目不实现采样/混音引擎。
package backend

import "fmt"

// DeviceInfo 一个可选 MIDI 输出设备。
type DeviceInfo struct {
	ID   int    // winmm 设备序号
	Name string // 设备名 (如 "VirtualMIDISynth #1")
}

// Backend 是声源抽象。实现必须并发安全 (调度器与现场演奏会并发调用)。
type Backend interface {
	Name() string
	Devices() ([]DeviceInfo, error)
	// Open 打开设备: "auto" / "" = 自动选择, 数字 = 设备序号, 其他 = 按名称匹配。
	Open(device string) error
	Program(ch, prog int) error
	NoteOn(ch, note, vel int) error
	NoteOff(ch, note int) error
	Control(ch, ctrl, val int) error
	// AllOff 急停: 关闭全部声道全部音符。
	AllOff() error
	Close() error
}

// Registry 按名称构造后端。MVP 只有 midiout; 二期增加 fluidsynth 等。
func Registry() map[string]func() Backend {
	return map[string]func() Backend{
		"midiout": func() Backend { return &MidiOut{} },
	}
}

// Open 按名称构造并打开后端。device 为 "auto"/"" 时由后端自行选择。
func Open(name, device string) (Backend, error) {
	f, ok := Registry()[name]
	if !ok {
		return nil, fmt.Errorf("未知后端 %q (可用: midiout)", name)
	}
	b := f()
	if err := b.Open(device); err != nil {
		return nil, err
	}
	return b, nil
}
