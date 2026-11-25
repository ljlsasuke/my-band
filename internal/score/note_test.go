package score

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ljlsasuke/pianostage/internal/core"
)

// 用例直译前身 auto-piano 自测 (scores/mapping 的验证集)。

func TestNoteNum(t *testing.T) {
	valid := map[string]int{
		"C4": 60, "A0": 21, "C8": 108, "C-1": 0, "G9": 127,
		"C#4": 61, "Db4": 61, "#F4": 66, // 前缀式 #F4 = F#4 = 66
	}
	for name, want := range valid {
		got, err := NoteNum(name)
		if err != nil {
			t.Errorf("NoteNum(%q) 报错: %v", name, err)
		} else if got != want {
			t.Errorf("NoteNum(%q) = %d, want %d", name, got, want)
		}
	}
	invalid := []string{"", "H4", "C", "C#b4", "4C", "Cx4"}
	for _, name := range invalid {
		if _, err := NoteNum(name); err == nil {
			t.Errorf("NoteNum(%q) 应报错", name)
		}
	}
}

func TestNoteNameRoundTrip(t *testing.T) {
	for n := 21; n <= 108; n++ {
		back, err := NoteNum(NoteName(n))
		if err != nil || back != n {
			t.Errorf("往返失败: %d → %s → %d (err=%v)", n, NoteName(n), back, err)
		}
	}
}

func TestKeyPC(t *testing.T) {
	valid := map[string]int{"C": 0, "c": 0, "D": 2, "F#": 6, "Bb": 10, "C##": 2} // 双升号合法 (前身逐字符验证)
	for k, want := range valid {
		got, err := KeyPC(k)
		if err != nil || got != want {
			t.Errorf("KeyPC(%q) = %v (err %v), want %d", k, got, err, want)
		}
	}
	for _, k := range []string{"", "H", "Fx"} {
		if _, err := KeyPC(k); err == nil {
			t.Errorf("KeyPC(%q) 应报错", k)
		}
	}
}

func TestParseNoteSeq(t *testing.T) {
	evs, err := ParseNoteSeq("C4:1 D4:1 E4:2 | C4+E4+G4:2 | R:1 // comment", DefaultBeat, DefaultVelocity)
	if err != nil {
		t.Fatal(err)
	}
	// C4,D4,E4 各 1 拍 + 和弦 3 音 + R 休止 → NoteOn 6 个
	ons := 0
	for _, e := range evs {
		if e.Kind == core.KindNoteOn {
			ons++
		}
	}
	if ons != 6 {
		t.Fatalf("应有 6 个 NoteOn, 实得 %d: %v", ons, evs)
	}
	// 和弦同起同长: C4@0 D4@0.55 E4@1.1(两拍) → 和弦 @2.2
	chordT := 2.2
	for _, e := range evs {
		if e.Note == 67 && e.Kind == core.KindNoteOn { // G4
			if e.T != chordT {
				t.Errorf("和弦应同起: G4 @%v, want %v", e.T, chordT)
			}
		}
	}
	// 休止符推进时间轴: E4 两拍至 2.2, 和弦至 3.3, R 推至 3.85 (无事件)
	if evs[len(evs)-1].T < 3.29 || evs[len(evs)-1].T > 3.31 {
		t.Errorf("末事件 T = %v, want ~3.3", evs[len(evs)-1].T)
	}
}

func TestParseJianpu(t *testing.T) {
	evs, err := ParseJianpu("1 2 3", "C", DefaultBeat, DefaultVelocity)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 6 { // 3 音 × (on+off)
		t.Fatalf("3 音应 6 事件, 实得 %d", len(evs))
	}
	if evs[0].Note != 60 {
		t.Errorf("1=C → C4 (60), 实得 %d", evs[0].Note)
	}

	// 5- = 两拍
	evs, _ = ParseJianpu("5-", "C", DefaultBeat, DefaultVelocity)
	if evs[1].T-evs[0].T != 2*DefaultBeat {
		t.Errorf("5- 应两拍, 实得 %v", evs[1].T-evs[0].T)
	}

	// 1=G: 1 → G4 (67)
	evs, _ = ParseJianpu("1", "G", DefaultBeat, DefaultVelocity)
	if evs[0].Note != 67 {
		t.Errorf("1=G → G4 (67), 实得 %d", evs[0].Note)
	}

	// 八度: 1^ → C5 (72), 1v → C3 (48)
	evs, _ = ParseJianpu("1^", "C", DefaultBeat, DefaultVelocity)
	if evs[0].Note != 72 {
		t.Errorf("1^ → C5 (72), 实得 %d", evs[0].Note)
	}
	evs, _ = ParseJianpu("1v", "C", DefaultBeat, DefaultVelocity)
	if evs[0].Note != 48 {
		t.Errorf("1v → C3 (48), 实得 %d", evs[0].Note)
	}

	// b1 → B3 (59): 地板除关键回归 (Go 截断除会错成 B4)
	evs, _ = ParseJianpu("b1", "C", DefaultBeat, DefaultVelocity)
	if evs[0].Note != 59 {
		t.Errorf("b1 → B3 (59), 实得 %d", evs[0].Note)
	}

	// #4 → F#4 (66)
	evs, _ = ParseJianpu("#4", "C", DefaultBeat, DefaultVelocity)
	if evs[0].Note != 66 {
		t.Errorf("#4 → F#4 (66), 实得 %d", evs[0].Note)
	}

	// 0 休止: 不产事件但推进时间
	evs, _ = ParseJianpu("0 1", "C", DefaultBeat, DefaultVelocity)
	if evs[0].T != DefaultBeat {
		t.Errorf("0 应推进一拍, 实音起点 %v, want %v", evs[0].T, DefaultBeat)
	}

	// 非法 token
	if _, err := ParseJianpu("8 1", "C", DefaultBeat, DefaultVelocity); err == nil {
		t.Error("8 不在简谱度数内, 应报错")
	}
	if _, err := ParseJianpu("1x", "C", DefaultBeat, DefaultVelocity); err == nil {
		t.Error("未知后缀应报错")
	}
	if _, err := ParseJianpu("1", "H", DefaultBeat, DefaultVelocity); err == nil {
		t.Error("非法调名应报错")
	}
}

func TestSplitFormatDecl(t *testing.T) {
	text := "format: jianpu\n1 2 3\nformat: note_seq\n1 2"
	body, declared := SplitFormatDecl(text)
	// 声明行语义沿用前身: 后者覆盖 (循环赋值)
	if declared != "note_seq" {
		t.Errorf("声明行后者覆盖, 实得 %q", declared)
	}
	if body != "1 2 3\n1 2" {
		t.Errorf("声明行应剥离: %q", body)
	}
	if _, d := SplitFormatDecl("1 2 3"); d != "" {
		t.Errorf("无声明行 declared 应为空, 实得 %q", d)
	}
}

func TestLoadDispatch(t *testing.T) {
	dir := t.TempDir()

	// .mid → MIDI 路径 (复用 midi 测试的夹具构造)
	midPath := filepath.Join(dir, "x.mid")
	if err := os.WriteFile(midPath, buildMidi(480, append(noteOn(0, 0, 60, 90), noteOff(480, 0, 60)...)), 0644); err != nil {
		t.Fatal(err)
	}
	evs, err := Load(midPath, "C", DefaultBeat, DefaultVelocity)
	if err != nil || len(evs) != 2 {
		t.Fatalf("Load(.mid) = %d 事件 (err %v), want 2", len(evs), err)
	}

	// txt + 声明行 → jianpu
	txtPath := filepath.Join(dir, "y.txt")
	os.WriteFile(txtPath, []byte("format: jianpu\n1 2 3"), 0644)
	evs, err = Load(txtPath, "C", DefaultBeat, DefaultVelocity)
	if err != nil || len(evs) != 6 {
		t.Fatalf("Load(txt jianpu) = %d 事件 (err %v), want 6", len(evs), err)
	}

	// txt 无声明 → note_seq 探测 (含冒号)
	txt2 := filepath.Join(dir, "z.txt")
	os.WriteFile(txt2, []byte("C4:1 D4:1"), 0644)
	evs, err = Load(txt2, "C", DefaultBeat, DefaultVelocity)
	if err != nil || len(evs) != 4 {
		t.Fatalf("Load(txt note_seq) = %d 事件 (err %v), want 4", len(evs), err)
	}

	// 不存在
	if _, err := Load(filepath.Join(dir, "no.mid"), "C", DefaultBeat, DefaultVelocity); err == nil {
		t.Error("不存在的文件应报错")
	}
}
