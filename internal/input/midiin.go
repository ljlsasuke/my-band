//go:build windows

package input

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ljlsasuke/my-band/internal/backend/winmm"
)

// MidiIn MIDI 键盘/电钢琴输入 (真实力度)。
// winmm 回调线程把短消息丢进有界 channel (满则丢弃并计数), 渲染帧里
// Update() 排空并转发 —— 回调线程零阻塞。
type MidiIn struct {
	mu      sync.Mutex
	handle  uintptr
	name    string
	handler Handler
	ch      chan uintptr
	dropped int // 溢出丢弃计数 (诊断用)
}

// ListMidiIn 枚举系统 MIDI 输入设备 (MIDI 键盘/电钢琴)。
func ListMidiIn() (names []string, ids []int, err error) {
	n := winmm.InNumDevs()
	for i := 0; i < n; i++ {
		c, err := winmm.InDevCaps(i)
		if err != nil {
			return nil, nil, err
		}
		names = append(names, winmm.InName(c))
		ids = append(ids, i)
	}
	return names, ids, nil
}

// DetectMidiIn 自动探测: 存在输入设备即返回第一个 (id, 名字)。
func DetectMidiIn() (id int, name string, ok bool) {
	names, ids, err := ListMidiIn()
	if err != nil || len(ids) == 0 {
		return 0, "", false
	}
	return ids[0], names[0], true
}

// NewMidiIn 打开输入设备。device: "auto" = 第一个设备; 数字 = 序号; 其他 = 按名称匹配。
func NewMidiIn(device string, h Handler) (*MidiIn, error) {
	if h == nil {
		return nil, fmt.Errorf("handler 为空")
	}
	n := winmm.InNumDevs()
	if n == 0 {
		return nil, fmt.Errorf("未检测到 MIDI 输入设备 (接上 MIDI 键盘/电钢琴后重试)")
	}

	id := -1
	var name string
	d := strings.TrimSpace(device)
	switch {
	case d == "" || d == "auto":
		id, name = 0, ""
		if c, err := winmm.InDevCaps(0); err == nil {
			name = winmm.InName(c)
		}
	default:
		var idx int
		if _, convErr := fmt.Sscanf(d, "%d", &idx); convErr == nil && idx >= 0 && idx < n {
			id = idx
			if c, err := winmm.InDevCaps(id); err == nil {
				name = winmm.InName(c)
			}
		} else {
			for i := 0; i < n; i++ {
				c, err := winmm.InDevCaps(i)
				if err == nil && strings.Contains(strings.ToLower(winmm.InName(c)), strings.ToLower(d)) {
					id, name = i, winmm.InName(c)
					break
				}
			}
			if id < 0 {
				return nil, fmt.Errorf("找不到 MIDI 输入设备 %q", d)
			}
		}
	}

	m := &MidiIn{
		name:    name,
		handler: h,
		ch:      make(chan uintptr, 512),
	}
	hh, err := winmm.InOpen(id, func(dwParam1 uintptr) {
		select {
		case m.ch <- dwParam1:
		default:
			m.dropped++ // 队列满: 丢弃 (回调线程零阻塞)
		}
	})
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.handle = hh
	m.mu.Unlock()
	if err := winmm.InStart(hh); err != nil {
		winmm.InClose(hh)
		return nil, err
	}
	return m, nil
}

func (m *MidiIn) Name() string { return "MIDI 键盘: " + m.name }

// Update 排空回调队列, 解析短消息并转发。
func (m *MidiIn) Update() {
	for {
		select {
		case msg := <-m.ch:
			if e, ok := winmm.ParseShortMsg(msg); ok {
				m.handler(e)
			}
		default:
			return
		}
	}
}

func (m *MidiIn) Close() {
	m.mu.Lock()
	h := m.handle
	m.handle = 0
	m.mu.Unlock()
	if h == 0 {
		return
	}
	_ = winmm.InStop(h)
	_ = winmm.InReset(h)
	_ = winmm.InClose(h)
}
