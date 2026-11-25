//go:build windows

package backend

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/ljlsasuke/pianostage/internal/backend/winmm"
)

// MidiOut 基于 winmm 的 MIDI 直连后端 (PLAN.md §5.2 / ADR-3)。
//
// 设备选择策略 (auto): VirtualMIDISynth 优先 (用户挂了 SoundFont, 音质最优)
// → Microsoft GS Wavetable Synth (Windows 自带兜底) → 报错并列出可用设备。
// 音质升级路径 = 用户装 VirtualMIDISynth, 本后端零改动自动受益。
type MidiOut struct {
	mu      sync.Mutex
	handle  uintptr
	devID   int
	devName string
}

func (m *MidiOut) Name() string { return "midiout" }

// Devices 枚举系统 MIDI 输出设备。
func (m *MidiOut) Devices() ([]DeviceInfo, error) {
	n := winmm.NumDevs()
	devs := make([]DeviceInfo, 0, n)
	for i := 0; i < n; i++ {
		c, err := winmm.DevCaps(i)
		if err != nil {
			return nil, err
		}
		devs = append(devs, DeviceInfo{ID: i, Name: c.Name()})
	}
	return devs, nil
}

// PickAuto 自动选设备: VirtualMIDISynth → GS Wavetable → 报错。
func PickAuto(devs []DeviceInfo) (DeviceInfo, error) {
	contains := func(d DeviceInfo, sub string) bool {
		return strings.Contains(strings.ToLower(d.Name), strings.ToLower(sub))
	}
	for _, d := range devs {
		if contains(d, "VirtualMIDISynth") {
			return d, nil
		}
	}
	for _, d := range devs {
		if contains(d, "GS Wavetable") {
			return d, nil
		}
	}
	if len(devs) > 0 {
		return devs[0], nil
	}
	return DeviceInfo{}, fmt.Errorf("未找到任何 MIDI 输出设备 (可安装 VirtualMIDISynth 或检查音频设备)")
}

// Open 打开设备: "auto"/"" = 自动; 数字 = 设备序号; 其他 = 按名称匹配。
func (m *MidiOut) Open(device string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handle != 0 {
		return fmt.Errorf("设备已打开 (%s), 请先 Close", m.devName)
	}

	devs, err := m.Devices()
	if err != nil {
		return err
	}

	var id = -1
	var name string
	d := strings.TrimSpace(device)
	switch {
	case d == "" || d == "auto":
		pick, err := PickAuto(devs)
		if err != nil {
			return err
		}
		id, name = pick.ID, pick.Name
	default:
		if idx, convErr := strconv.Atoi(d); convErr == nil {
			if idx < 0 || idx >= len(devs) {
				return fmt.Errorf("设备序号越界: %d (共 %d 个)", idx, len(devs))
			}
			id, name = devs[idx].ID, devs[idx].Name
		} else {
			for _, di := range devs {
				if strings.EqualFold(di.Name, d) {
					id, name = di.ID, di.Name
					break
				}
			}
			if id < 0 {
				return fmt.Errorf("找不到设备 %q (可用: %v)", d, deviceNames(devs))
			}
		}
	}

	h, err := winmm.Open(id)
	if err != nil {
		return err
	}
	m.handle, m.devID, m.devName = h, id, name
	return nil
}

func deviceNames(devs []DeviceInfo) []string {
	names := make([]string, len(devs))
	for i, d := range devs {
		names[i] = d.Name
	}
	return names
}

func (m *MidiOut) send(status, ch, d1, d2 int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handle == 0 {
		return fmt.Errorf("设备未打开")
	}
	return winmm.ShortMsg(m.handle, byte(status)|byte(ch), byte(d1), byte(d2))
}

func (m *MidiOut) Program(ch, prog int) error { return m.send(0xC0, ch, prog, 0) }
func (m *MidiOut) NoteOn(ch, note, vel int) error {
	return m.send(0x90, ch, note, vel)
}
func (m *MidiOut) NoteOff(ch, note int) error { return m.send(0x80, ch, note, 0) }
func (m *MidiOut) Control(ch, ctrl, val int) error {
	return m.send(0xB0, ch, ctrl, val)
}

// AllOff 急停: 全声道 All Notes Off (CC123) + 显式 NoteOff 双保险。
func (m *MidiOut) AllOff() error {
	for ch := 0; ch < 16; ch++ {
		if err := m.Control(ch, 123, 0); err != nil {
			return err
		}
		for note := 0; note < 128; note++ {
			_ = m.NoteOff(ch, note)
		}
	}
	return nil
}

func (m *MidiOut) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handle == 0 {
		return nil
	}
	err := winmm.Close(m.handle)
	m.handle = 0
	return err
}
