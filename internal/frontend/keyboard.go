// Package frontend 演奏前端 (PLAN.md §5.1)。
//
// 分层: keyboard/keys 是纯几何与纯视觉状态机 (可单测, 不依赖渲染库),
// stage.go 是 Ebiten 薄壳 (总线消费者: 只"演", 不发声)。
package frontend

// 音域: 88 键 A0~C8 (PLAN.md §5.1)。
const (
	MinNote = 21 // A0
	MaxNote = 108 // C8
)

// IsBlack 该 MIDI 音符是否黑键 (半音 1/3/6/8/10: C#/D#/F#/G#/A#)。
func IsBlack(n int) bool {
	switch n % 12 {
	case 1, 3, 6, 8, 10:
		return true
	}
	return false
}

// KeyRect 单键几何 (归一化坐标 0..1, 相对键盘区域, 绘制时乘画布尺寸)。
type KeyRect struct {
	Note  int
	Black bool
	X, Y, W, H float64
}

// ComputeLayout 计算音域内全部键的几何。
// 白键等宽平铺; 黑键宽为白键的 62%、高为 62%, 中心对齐其左侧白键与
// 右侧白键的分界线 —— 与真实钢琴排列一致。
func ComputeLayout(minNote, maxNote int) []KeyRect {
	whites := 0
	for n := minNote; n <= maxNote; n++ {
		if !IsBlack(n) {
			whites++
		}
	}
	ww := 1.0 / float64(whites)
	bw := ww * 0.62
	bh := 0.62

	rects := make([]KeyRect, 0, maxNote-minNote+1)
	wIdx := 0 // 已越过的白键数 = 黑键所在的分界线序号
	for n := minNote; n <= maxNote; n++ {
		if IsBlack(n) {
			rects = append(rects, KeyRect{
				Note: n, Black: true,
				X: float64(wIdx)*ww - bw/2, Y: 0, W: bw, H: bh,
			})
		} else {
			rects = append(rects, KeyRect{
				Note: n, Black: false,
				X: float64(wIdx) * ww, Y: 0, W: ww, H: 1,
			})
			wIdx++
		}
	}
	return rects
}

// WhiteCount 音域内白键数 (88 键标准音域 = 52)。
func WhiteCount(minNote, maxNote int) int {
	n := 0
	for k := minNote; k <= maxNote; k++ {
		if !IsBlack(k) {
			n++
		}
	}
	return n
}
