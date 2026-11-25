package score

import (
	"bytes"
	"testing"

	"github.com/ljlsasuke/pianostage/internal/core"
)

// ---- 测试夹具: 内存合成 SMF ----

func varlenBytes(n int) []byte {
	if n == 0 {
		return []byte{0}
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte(n & 0x7F)}, out...)
		n >>= 7
	}
	for i := 0; i < len(out)-1; i++ {
		out[i] |= 0x80
	}
	return out
}

func evBytes(delta int, bs ...byte) []byte {
	return append(varlenBytes(delta), bs...)
}

func noteOn(delta, ch, note, vel int) []byte {
	return evBytes(delta, byte(0x90|ch), byte(note), byte(vel))
}

func noteOff(delta, ch, note int) []byte {
	return evBytes(delta, byte(0x80|ch), byte(note), 0)
}

func tempoEv(delta int, us uint32) []byte {
	return evBytes(delta, 0xFF, 0x51, 0x03, byte(us>>16), byte(us>>8), byte(us))
}

func programEv(delta, ch, prog int) []byte {
	return evBytes(delta, byte(0xC0|ch), byte(prog))
}

func buildMidi(division uint16, tracks ...[]byte) []byte {
	var b bytes.Buffer
	b.WriteString("MThd")
	be32(&b, 6)
	be16(&b, 1)                   // format (解析器不依赖具体值, 但字段顺序必须正确)
	be16(&b, uint16(len(tracks))) // ntrks
	be16(&b, division)
	for _, tr := range tracks {
		b.WriteString("MTrk")
		be32(&b, uint32(len(tr)))
		b.Write(tr)
	}
	return b.Bytes()
}

func be16(b *bytes.Buffer, v uint16) { b.WriteByte(byte(v >> 8)); b.WriteByte(byte(v)) }
func be32(b *bytes.Buffer, v uint32) {
	be16(b, uint16(v>>16))
	be16(b, uint16(v))
}

// cat 拼接多个事件片段。
func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// ---- 断言工具 ----

func find(t *testing.T, evs []core.Event, kind core.Kind, note int) core.Event {
	t.Helper()
	for _, e := range evs {
		if e.Kind == kind && e.Note == note {
			return e
		}
	}
	t.Fatalf("找不到事件 %s(note=%d): %v", kind, note, evs)
	return core.Event{}
}

// ---- 用例 ----

func TestBasicNote120Bpm(t *testing.T) {
	// division=480: 一拍 = 0.5s (默认 120bpm)
	evs, err := ParseMidiBytes(buildMidi(480, append(noteOn(0, 0, 60, 90), noteOff(480, 0, 60)...)))
	if err != nil {
		t.Fatal(err)
	}
	on := find(t, evs, core.KindNoteOn, 60)
	off := find(t, evs, core.KindNoteOff, 60)
	if on.T != 0 || on.Vel != 90 || on.Ch != 0 {
		t.Errorf("NoteOn = %+v", on)
	}
	if off.T != 0.5 {
		t.Errorf("一拍应为 0.5s, 实得 %v", off.T)
	}
}

func TestTempoMapTwoPass(t *testing.T) {
	// format 1 经典布局: tempo 只在第 0 轨, 音符在第 1 轨 (前身旧 bug 的回归测试)
	// tempo 600000 us/拍 (100bpm): 一拍 = 0.6s
	tr0 := tempoEv(0, 600000)
	tr1 := append(noteOn(0, 0, 60, 90), noteOff(480, 0, 60)...)
	evs, err := ParseMidiBytes(buildMidi(480, tr0, tr1))
	if err != nil {
		t.Fatal(err)
	}
	if off := find(t, evs, core.KindNoteOff, 60); off.T != 0.6 {
		t.Errorf("应取第 0 轨的 tempo (0.6s), 实得 %v", off.T)
	}
}

func TestTempoChangeMidSong(t *testing.T) {
	// 分段积分: tick<960 用 500000, tick>=960 用 250000
	tr0 := append(tempoEv(0, 500000), tempoEv(960, 250000)...)
	tr1 := append(append(noteOn(0, 0, 60, 90), noteOff(960, 0, 60)...),
		append(noteOn(0, 0, 64, 90), noteOff(960, 0, 64)...)...) // 第二音从 tick960 起
	evs, err := ParseMidiBytes(buildMidi(480, tr0, tr1))
	if err != nil {
		t.Fatal(err)
	}
	if off := find(t, evs, core.KindNoteOff, 60); off.T != 1.0 {
		t.Errorf("变速前: 2 拍应 1.0s, 实得 %v", off.T)
	}
	if off := find(t, evs, core.KindNoteOff, 64); off.T != 1.5 {
		t.Errorf("变速后: 2 拍应再 0.5s (累计 1.5s), 实得 %v", off.T)
	}
}

func TestRetriggerTruncates(t *testing.T) {
	// 未配对 off 的重复 on: 前一个按 retrigger 截断, 不丢音
	tr := append(noteOn(0, 0, 60, 90), noteOn(240, 0, 60, 80)...)
	evs, err := ParseMidiBytes(buildMidi(480, tr))
	if err != nil {
		t.Fatal(err)
	}
	onCount := 0
	for _, e := range evs {
		if e.Kind == core.KindNoteOn && e.Note == 60 {
			onCount++
			if onCount == 2 && e.Vel != 80 {
				t.Errorf("第二个 NoteOn 力度应 80, 实得 %d", e.Vel)
			}
		}
	}
	if onCount != 2 {
		t.Fatalf("retrigger 应保留两个音, 实得 %d", onCount)
	}
	// 第一个音在 tick240 被截断 (0.25s), 第二个音 unclosed → 曲末关闭 (≥1s)
	offAtQuarter := false
	offAtEnd := false
	for _, e := range evs {
		if e.Kind == core.KindNoteOff && e.Note == 60 {
			if e.T == 0.25 {
				offAtQuarter = true
			}
			if e.T >= 1.0 {
				offAtEnd = true
			}
		}
	}
	if !offAtQuarter || !offAtEnd {
		t.Errorf("retrigger 截断不完整: offAtQuarter=%v offAtEnd=%v: %v", offAtQuarter, offAtEnd, evs)
	}
}

func TestChannel9PercussionKept(t *testing.T) {
	// 前身丢弃声道 9 (FreePiano 无鼓组); GM 直连后必须保留
	tr := append(noteOn(0, 9, 36, 100), noteOff(480, 9, 36)...)
	evs, err := ParseMidiBytes(buildMidi(480, tr))
	if err != nil {
		t.Fatal(err)
	}
	on := find(t, evs, core.KindNoteOn, 36)
	if on.Ch != 9 {
		t.Errorf("声道 9 打击乐被丢弃: %+v", on)
	}
}

func TestProgramChange(t *testing.T) {
	tr := append(append(programEv(0, 3, 73), noteOn(0, 3, 76, 80)...), noteOff(480, 3, 76)...)
	evs, err := ParseMidiBytes(buildMidi(480, tr))
	if err != nil {
		t.Fatal(err)
	}
	var prog *core.Event
	for i, e := range evs {
		if e.Kind == core.KindProgram {
			prog = &evs[i]
		}
	}
	if prog == nil {
		t.Fatalf("Program Change 丢失: %v", evs)
	}
	if prog.Prog != 73 || prog.Ch != 3 {
		t.Errorf("Program = %+v", *prog)
	}
	// 同刻优先级: Program 必须先于 NoteOn (音色先就位)
	on := find(t, evs, core.KindNoteOn, 76)
	if prog.T > on.T {
		t.Errorf("Program(%v) 应不晚于 NoteOn(%v)", prog.T, on.T)
	}
}

func noteOffs(delta, ch int, notes ...int) []byte {
	var out []byte
	for _, n := range notes {
		out = append(out, noteOff(delta, ch, n)...)
	}
	return out
}

func TestNoteOffBeforeNoteOnAtSameTick(t *testing.T) {
	// 音符无缝衔接 (off 与 on 同 tick): 同刻排序必须 NoteOff 在前,
	// 否则调度器重触逻辑会把刚起音的音误关
	tr := append(append(noteOn(0, 0, 60, 90), noteOff(480, 0, 60)...),
		append(noteOn(0, 0, 62, 90), noteOff(480, 0, 62)...)...)
	evs, err := ParseMidiBytes(buildMidi(480, tr))
	if err != nil {
		t.Fatal(err)
	}
	last := -1.0
	for i, e := range evs {
		if e.T < last {
			t.Fatalf("事件乱序: #%d T=%v < 前一个 %v", i, e.T, last)
		}
		last = e.T
	}
	// 关键顺序断言: T=0.5 处 NoteOff(60) 必须在 NoteOn(62) 之前
	off60, on62 := -1, -1
	for i, e := range evs {
		if e.T == 0.5 {
			if e.Kind == core.KindNoteOff && e.Note == 60 {
				off60 = i
			}
			if e.Kind == core.KindNoteOn && e.Note == 62 {
				on62 = i
			}
		}
	}
	if off60 < 0 || on62 < 0 || off60 > on62 {
		t.Errorf("同刻 NoteOff(60)@#%d 应在 NoteOn(62)@#%d 之前: %v", off60, on62, evs)
	}
}

func TestSMPTEDivision(t *testing.T) {
	// 0xE728: 25fps × 40 ticks/frame = 1000 tick/s; delta 1000 → 1.0s (与 tempo 无关)
	tr := append(noteOn(0, 0, 60, 90), noteOff(1000, 0, 60)...)
	evs, err := ParseMidiBytes(buildMidi(0xE728, tr))
	if err != nil {
		t.Fatal(err)
	}
	if off := find(t, evs, core.KindNoteOff, 60); off.T != 1.0 {
		t.Errorf("SMPTE 1000 tick 应为 1.0s, 实得 %v", off.T)
	}
}

func TestRunningStatus(t *testing.T) {
	// 状态字节省略: 0x90 后跟两组 [delta, note, vel]
	tr := cat(noteOn(0, 0, 60, 90), evBytes(480, 64, 90), noteOff(480, 0, 60), evBytes(0, 64, 0))
	evs, err := ParseMidiBytes(buildMidi(480, tr))
	if err != nil {
		t.Fatal(err)
	}
	if on := find(t, evs, core.KindNoteOn, 64); on.Vel != 90 || on.T != 0.5 {
		t.Errorf("running status 第二音 = %+v", on)
	}
	if off := find(t, evs, core.KindNoteOff, 64); off.T != 1.0 {
		t.Errorf("running status 第二音释放 = %v, want 1.0", off.T)
	}
}

func TestNoteOnWithZeroVelocityIsNoteOff(t *testing.T) {
	tr := cat(noteOn(0, 0, 60, 90), evBytes(480, 0x90, 60, 0))
	evs, err := ParseMidiBytes(buildMidi(480, tr))
	if err != nil {
		t.Fatal(err)
	}
	if evs[0].Kind != core.KindNoteOn || evs[1].Kind != core.KindNoteOff {
		t.Errorf("vel=0 的 0x90 应按 NoteOff 处理: %v", evs)
	}
}

func TestMinDurFilter(t *testing.T) {
	// 12 tick @480div ≈ 0.0125s < 0.05s → 丢弃
	tr := cat(noteOn(0, 0, 60, 90), noteOff(12, 0, 60), noteOn(468, 0, 64, 90), noteOff(480, 0, 64))
	evs, err := ParseMidiBytes(buildMidi(480, tr))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Note == 60 {
			t.Errorf("短音应被过滤: %v", evs)
		}
	}
	if find(t, evs, core.KindNoteOn, 64).T != 0.5 {
		t.Errorf("正常音不应受影响")
	}
}

func TestInvalidFiles(t *testing.T) {
	if _, err := ParseMidiBytes([]byte("not a midi")); err == nil {
		t.Error("非 MIDI 数据应报错")
	}
	if _, err := ParseMidiBytes(nil); err == nil {
		t.Error("空数据应报错")
	}
	bad := buildMidi(480, []byte{0x00}) // 轨内孤立 delta 无 status → running status 异常
	if _, err := ParseMidiBytes(bad); err == nil {
		t.Error("running status 异常应报错")
	}
}

func TestRealMidiFiles(t *testing.T) {
	// 真实文件回归: 用户曲库里的命运交响曲 (管弦乐 14 轨) 必须解析成功
	evs, err := ParseMidi(`C:\Users\sasuke\AppData\Roaming\auto_piano\songs\mingyun_jiaoxiangqu.mid`)
	if err != nil {
		t.Skipf("用户曲库不可用, 跳过: %v", err)
	}
	ons, progs := 0, 0
	var lastT float64
	for _, e := range evs {
		if e.T < lastT {
			t.Fatalf("事件乱序: #%v T=%v < %v", e, e.T, lastT)
		}
		lastT = e.T
		if e.Kind == core.KindNoteOn {
			ons++
		}
		if e.Kind == core.KindProgram {
			progs++
		}
	}
	if ons == 0 || progs == 0 {
		t.Errorf("命运交响曲应含音符与音色变更, 实得 on=%d prog=%d", ons, progs)
	}
}
