package frontend

import "github.com/hajimehoshi/ebiten/v2"

// 计算机键盘 → 音符映射 (两排布局, FreePiano/Synthesia 惯例):
//   下排 (基准八度): Z S X D C V G B H N J M ,  → 白黑相间 C..B
//   上排 (+1 八度):  Q 2 W 3 E R 5 T 6 Y 7 U I
//   - / = 切换八度 (下移/上移)
// 仅在窗口聚焦时生效 (Ebiten 输入特性) —— 打字不会误触发演奏。

type keyMapping struct {
	key  ebiten.Key
	note int // 相对基准八度的半音偏移
}

var keyMap = []keyMapping{
	// 下排
	{ebiten.KeyZ, 0}, {ebiten.KeyS, 1}, {ebiten.KeyX, 2}, {ebiten.KeyD, 3},
	{ebiten.KeyC, 4}, {ebiten.KeyV, 5}, {ebiten.KeyG, 6}, {ebiten.KeyB, 7},
	{ebiten.KeyH, 8}, {ebiten.KeyN, 9}, {ebiten.KeyJ, 10}, {ebiten.KeyM, 11},
	{ebiten.KeyComma, 12},
	// 上排 (+12)
	{ebiten.KeyQ, 12}, {ebiten.KeyDigit2, 13}, {ebiten.KeyW, 14}, {ebiten.KeyDigit3, 15},
	{ebiten.KeyE, 16}, {ebiten.KeyR, 17}, {ebiten.KeyDigit5, 18}, {ebiten.KeyT, 19},
	{ebiten.KeyDigit6, 20}, {ebiten.KeyY, 21}, {ebiten.KeyDigit7, 22}, {ebiten.KeyU, 23},
	{ebiten.KeyI, 24},
}

// HumanVelocity 人类键盘演奏的固定力度 (键盘无法表达触键力度)。
const HumanVelocity = 100

// HumanInput 人类输入状态机: 按键状态 → 音符 on/off 事件流。
// 纯状态迁移, 事件产生交给回调 (渲染层调用)。
type HumanInput struct {
	base    int // 基准八度的 MIDI 音符 (默认 C3=48? 前身默认中央区)
	held    map[ebiten.Key]int
	handler func(note int, on bool, vel int)
}

// NewHumanInput base 为基准八度根音 (Z 键对应), handler 收到 on/off 事件。
func NewHumanInput(base int, handler func(note int, on bool, vel int)) *HumanInput {
	return &HumanInput{base: base, held: make(map[ebiten.Key]int), handler: handler}
}

// SetBase 调整基准八度根音; 已按住的键先全部释放。
func (h *HumanInput) SetBase(base int) {
	if base == h.base {
		return
	}
	for k, n := range h.held {
		h.handler(n, false, 0)
		delete(h.held, k)
	}
	h.base = base
}

// Update 轮询物理键盘状态, 迁移 on/off。ebiten.Update 每帧调用。
func (h *HumanInput) Update() {
	// 八度切换
	if ebiten.IsKeyPressed(ebiten.KeyMinus) {
		if h.base-12 >= 0 {
			h.SetBase(h.base - 12)
		}
	} else if ebiten.IsKeyPressed(ebiten.KeyEqual) {
		if h.base+12+24 <= MaxNote {
			h.SetBase(h.base + 12)
		}
	}
	// 按键 → 音符
	for _, m := range keyMap {
		note := h.base + m.note
		if note < MinNote || note > MaxNote {
			continue
		}
		pressed := ebiten.IsKeyPressed(m.key)
		_, wasHeld := h.held[m.key]
		switch {
		case pressed && !wasHeld:
			h.held[m.key] = note
			h.handler(note, true, HumanVelocity)
		case !pressed && wasHeld:
			delete(h.held, m.key)
			h.handler(note, false, 0)
		}
	}
}
