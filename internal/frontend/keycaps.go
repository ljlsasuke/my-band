package frontend

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/ljlsasuke/my-band/internal/input"
)

// 键帽面板 (FreePiano 式, M4.5): 画出电脑键盘的三排键帽, 每帽显示它弹的
// 简谱数字 (1~7 + 高低音点) —— 零乐理也能看懂哪个键弹哪个音。
// 弹响的键帽高亮为琥珀色; 下落音符落向哪个键, 对应键帽同步高亮。

// jianpuOf 音符 → 简谱显示 (数字 + 八度点数, 正=上加点, 负=下加点)。
// 基准: C4 (60) 无点; 黑键显示其所属度数 (C#→1, D#→2, F#→5, G#→6, A#→7)。
func jianpuOf(note int) (num string, dots int) {
	pc := ((note % 12) + 12) % 12
	switch pc {
	case 0, 1:
		num = "1"
	case 2, 3:
		num = "2"
	case 4:
		num = "3"
	case 5, 6:
		num = "4"
	case 7, 8:
		num = "5"
	case 9, 10:
		num = "6"
	case 11:
		num = "7"
	}
	dots = note/12 - 5 // C4=60 → oct 5 → 0 点
	return num, dots
}

// capAreaHeight 键帽面板高度 (三排)。无数据时 0。
func capAreaHeight(src input.Source) int {
	if rs, ok := src.(input.RowSource); ok {
		return len(rs.Rows())*30 + 16
	}
	return 0
}

// drawKeycaps 渲染键帽面板。capAreaY/capAreaH 为面板区域。
func (s *Stage) drawKeycaps(screen *ebiten.Image, w, capAreaY, capAreaH int) {
	rs, ok := s.src.(input.RowSource)
	if !ok {
		return
	}
	rows := rs.Rows()
	if len(rows) == 0 {
		return
	}
	// capRows 本身就是物理顺序 (数字行最前 = 面板最上排), 不再反转
	// (此前错误反转导致数字行的键帽跑到了面板底部)
	if len(rows) < 4 {
		return
	}

	// 面板底色
	vector.DrawFilledRect(screen, 0, float32(capAreaY), float32(w), float32(capAreaH),
		color.RGBA{0x0c, 0x0c, 0x10, 0xff}, false)

	// 键帽尺寸: 等宽网格, 各排左对齐 (FreePiano 式整齐布局)
	const (
		capH   = 28
		gapY   = 4
		gapX   = 4
		margin = 10
	)
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	capW := (w - margin*2 - gapX*(maxCols-1)) / maxCols
	if capW < 34 {
		capW = 34
	}

	rowY := capAreaY + 6
	for _, row := range rows {
		for i, l := range row {
			x := margin + i*(capW+gapX)
			y := rowY

			// 高亮: 只按物理键判定 (数字行与 Q 行有同音重键,
			// 按音符高亮会让两个键帽齐亮 —— 用户实测困惑, 已改)
			fill := color.RGBA{0x2e, 0x2e, 0x36, 0xff}
			txtCol := color.RGBA{0xd8, 0xd8, 0xe0, 0xff}
			if l.Held {
				fc := s.kb.famAt(&s.kb.keys[l.Note], tNow(s)).Color()
				fill = color.RGBA{fc.R, fc.G, fc.B, 0xff}
				txtCol = color.RGBA{0x14, 0x14, 0x14, 0xff}
			}
			vector.DrawFilledRect(screen, float32(x), float32(y), float32(capW), capH, fill, false)
			vector.DrawFilledRect(screen, float32(x), float32(y), float32(capW), 1,
				color.RGBA{0x44, 0x44, 0x4c, 0xff}, false)

			// 简谱数字 + 八度点 (点在数字上/下方的空白带, 不与字形重叠)
			num, dots := jianpuOf(l.Note)
			numX := x + capW/2 - 4
			numY := y + 20 // basicfont 基线: 字形占 y+7..y+20
			text.Draw(screen, num, s.face, numX, numY, txtCol)
			dotR := float32(1.8)
			for d := 0; d < dots && d < 2; d++ {
				vector.DrawFilledCircle(screen, float32(numX+4), float32(y+6)-float32(d)*4, dotR, txtCol, false)
			}
			for d := 0; d < -dots && d < 2; d++ {
				vector.DrawFilledCircle(screen, float32(numX+4), float32(y+24)+float32(d)*4, dotR, txtCol, false)
			}
		}
		rowY += capH + gapY
	}
}

func tNow(s *Stage) float64 { return s.now() }
