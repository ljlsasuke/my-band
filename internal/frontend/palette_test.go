package frontend

import "testing"

func TestFamilyOfPercussion(t *testing.T) {
	// 声道 9 无条件打击
	if FamilyOf(9, 0) != FamPerc {
		t.Error("声道 9 应恒为打击")
	}
	if FamilyOf(9, 48) != FamPerc {
		t.Error("声道 9 换音色仍应打击")
	}
	// Percussive 族 (112~119)
	if FamilyOf(0, 118) != FamPerc {
		t.Error("program 118 应为打击")
	}
}

func TestFamilyOfOrchestra(t *testing.T) {
	// 命运交响曲的典型编制
	cases := []struct {
		prog int
		want Family
	}{
		{0, FamKeys},     // 钢琴
		{73, FamWoodwind}, // 长笛
		{68, FamWoodwind}, // 双簧管
		{71, FamWoodwind}, // 单簧管
		{70, FamWoodwind}, // 巴松
		{60, FamBrass},   // 圆号
		{56, FamBrass},   // 小号
		{47, FamPerc},    // 定音鼓
		{48, FamStrings}, // 弦乐合奏
		{45, FamStrings}, // Tremolo Strings
		{42, FamStrings}, // 大提琴 (哦不对, 42 是 Cello)
	}
	for _, c := range cases {
		if got := FamilyOf(0, c.prog); got != c.want {
			t.Errorf("FamilyOf(0,%d) = %v, want %v", c.prog, got, c.want)
		}
	}
}

func TestPaletteComplete(t *testing.T) {
	// 每个族都必须有颜色, 且颜色互不相同 (可区分性)
	seen := map[RGB]bool{}
	for f := FamKeys; f <= FamOther; f++ {
		c := f.Color()
		if c == (RGB{}) {
			t.Errorf("族 %v 无色板", f)
		}
		if seen[c] {
			t.Errorf("族 %v 颜色与之前重复: %v", f, c)
		}
		seen[c] = true
	}
}
