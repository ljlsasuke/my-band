//go:build !windows

package backend

import "fmt"

// MidiOut 非 Windows 占位 (MVP 仅 Windows, 见 PLAN.md ADR-5)。
type MidiOut struct{}

func (m *MidiOut) Name() string                    { return "midiout" }
func (m *MidiOut) CurrentDevice() (string, error)  { return "", errUnsupported() }
func (m *MidiOut) Devices() ([]DeviceInfo, error)  { return nil, errUnsupported() }
func (m *MidiOut) Open(device string) error        { return errUnsupported() }
func (m *MidiOut) Program(ch, prog int) error      { return errUnsupported() }
func (m *MidiOut) NoteOn(ch, note, vel int) error  { return errUnsupported() }
func (m *MidiOut) NoteOff(ch, note int) error      { return errUnsupported() }
func (m *MidiOut) Control(ch, ctrl, val int) error { return errUnsupported() }
func (m *MidiOut) AllOff() error                   { return errUnsupported() }
func (m *MidiOut) Close() error                    { return errUnsupported() }

func errUnsupported() error {
	return fmt.Errorf("midiout 后端当前仅支持 Windows (见 PLAN.md ADR-5)")
}
