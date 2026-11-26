package input

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ljlsasuke/my-band/internal/core"
)

// 物理键唯一绑定
func TestPhysicalKeysUnique(t *testing.T) {
	seen := map[ebiten.Key]bool{}
	for _, m := range flattenCaps() {
		if seen[m.key] {
			t.Errorf("物理键重复绑定: %s", m.cap)
		}
		seen[m.key] = true
	}
	if len(flattenCaps()) < 45 {
		t.Errorf("FreePiano 式四排应有 ~48 键, 实得 %d", len(flattenCaps()))
	}
}

// 全部键帽都是白键偏移 (FreePiano 哲学: 无黑键映射)
func TestAllWhiteKeys(t *testing.T) {
	white := map[int]bool{0: true, 2: true, 4: true, 5: true, 7: true, 9: true, 11: true}
	for _, m := range flattenCaps() {
		pc := m.note % 12
		if !white[pc] {
			t.Errorf("键帽 %s 偏移 %d 是黑键位置 (FreePiano 式映射应为纯白键)", m.cap, m.note)
		}
		if m.note < 0 || m.note > 57 {
			t.Errorf("偏移越界: %s → %d (应为 0..57)", m.cap, m.note)
		}
	}
}

// 每个八度的每个白键都被覆盖 (0..57 的全部白音)
func TestWhiteCoverage(t *testing.T) {
	covered := map[int]bool{}
	for _, m := range flattenCaps() {
		covered[m.note] = true
	}
	for n := 0; n <= 57; n++ {
		pc := n % 12
		isWhite := pc == 0 || pc == 2 || pc == 4 || pc == 5 || pc == 7 || pc == 9 || pc == 11
		if isWhite && !covered[n] {
			t.Errorf("白键偏移 %d 无键覆盖", n)
		}
	}
}

// 调号转调: F3/F4 的效果
func TestKeySignature(t *testing.T) {
	var fired []core.Event
	c := NewComputerKeys(36, func(e core.Event) { fired = append(fired, e) })

	// keySig=+2: Z 键 (偏移0) 发 D2
	c.SetKeySignature(2)
	var zCap keyCap
	found := false
	for _, m := range flattenCaps() {
		if m.key == ebiten.KeyZ {
			zCap, found = m, true
		}
	}
	if !found || zCap.note != 0 {
		t.Fatal("未找到 Z 键键帽 (偏移0)")
	}
	n, ok := c.noteOf(zCap)
	if !ok || n != 38 {
		t.Errorf("keySig=+2 时 Z 键应发 D2 (38), 实得 %d", n)
	}

	// 越界键帽被隐藏
	for _, row := range c.Rows() {
		for _, l := range row {
			if l.Note < 0 || l.Note > 127 {
				t.Fatalf("转调后出现越界音符: %d", l.Note)
			}
		}
	}

	// 极值: keySig 大幅上调时, 高音排键帽应被隐藏而非越界
	c.SetKeySignature(11)
	for _, row := range c.Rows() {
		for _, l := range row {
			if l.Note < 0 || l.Note > 127 {
				t.Fatalf("keySig=11 出现越界音符: %d", l.Note)
			}
		}
	}
}

func TestAutoBaseWhiteLayout(t *testing.T) {
	// 谱面集中在 C4~C5: 中位数 ~62 → base 应使窗口中心 (base+28) 罩住
	var evs []core.Event
	for n := 60; n <= 72; n++ {
		evs = append(evs, core.Event{Kind: core.KindNoteOn, Ch: 0, Note: n, Vel: 100})
	}
	base := AutoBase(evs)
	if base < 24 || base > 51 {
		t.Fatalf("base 越界: %d", base)
	}
	if median := 66; median < base || median > base+57 {
		t.Errorf("中位数 %d 不在窗口 [%d, %d]", median, base, base+57)
	}
	if b := AutoBase(nil); b != 36 {
		t.Errorf("空谱面 base = %d, want 36 (C2)", b)
	}
}
