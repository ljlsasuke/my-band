//go:build !windows

package input

import "fmt"

// ListMidiIn 非 Windows 占位 (MVP 仅 Windows, PLAN.md ADR-5)。
func ListMidiIn() (names []string, ids []int, err error) {
	return nil, nil, fmt.Errorf("MIDI 输入当前仅支持 Windows")
}

// DetectMidiIn 非 Windows 占位。
func DetectMidiIn() (id int, name string, ok bool) { return 0, "", false }
