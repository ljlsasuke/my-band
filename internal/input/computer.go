package input

import (
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ljlsasuke/my-band/internal/core"
)

// FreePiano 式全白键四排映射 (对齐 freepiano.keymap/freepiano.map):
//
//	数字行: 1 2 3 4 5 6 7 8 9 0 - = ⌫   → 最高
//	Q 行:   Q W E R T Y U I O P [ ] \    → 高
//	A 行:   A S D F G H J K L ; ' ↵      → 中
//	Z 行:   Z X C V B N M , . /          → 低
//
// 设计哲学 (FreePiano 同款): 电脑键盘 = 简谱白键盘, 每排顺次 1 2 3 4 5 6 7;
// 黑键不直接映射 —— F3/F4 升降调 (KeySignature), 黑键音由白键转调而来。
// 零乐理用户永远按"顺次的键", 调性靠转调按钮。

// keyCap 一枚键帽。
type keyCap struct {
	key  ebiten.Key
	cap  string
	note int // 相对基准 (Z 键, 默认 C2) 的白键偏移
}

// capRows 四排键帽, 显示顺序: 数字行(最高) → Q → A → Z(最低)。
// note = 相对基准 (Z 键, 默认 C2) 的白键偏移, 只含白键半音。
var capRows = [][]keyCap{
	{ // 数字行 (最高)
		{ebiten.KeyDigit1, "1", 36}, {ebiten.KeyDigit2, "2", 38}, {ebiten.KeyDigit3, "3", 40},
		{ebiten.KeyDigit4, "4", 41}, {ebiten.KeyDigit5, "5", 43}, {ebiten.KeyDigit6, "6", 45},
		{ebiten.KeyDigit7, "7", 47}, {ebiten.KeyDigit8, "8", 48}, {ebiten.KeyDigit9, "9", 50},
		{ebiten.KeyDigit0, "0", 52}, {ebiten.KeyMinus, "-", 53}, {ebiten.KeyEqual, "=", 55},
		{ebiten.KeyBackspace, "BS", 57},
	},
	{ // Q 行
		{ebiten.KeyQ, "Q", 24}, {ebiten.KeyW, "W", 26}, {ebiten.KeyE, "E", 28},
		{ebiten.KeyR, "R", 29}, {ebiten.KeyT, "T", 31}, {ebiten.KeyY, "Y", 33},
		{ebiten.KeyU, "U", 35}, {ebiten.KeyI, "I", 36}, {ebiten.KeyO, "O", 38},
		{ebiten.KeyP, "P", 40}, {ebiten.KeyBracketLeft, "[", 41}, {ebiten.KeyBracketRight, "]", 43},
		{ebiten.KeyBackslash, "\\", 45},
	},
	{ // A 行
		{ebiten.KeyA, "A", 12}, {ebiten.KeyS, "S", 14}, {ebiten.KeyD, "D", 16},
		{ebiten.KeyF, "F", 17}, {ebiten.KeyG, "G", 19}, {ebiten.KeyH, "H", 21},
		{ebiten.KeyJ, "J", 23}, {ebiten.KeyK, "K", 24}, {ebiten.KeyL, "L", 26},
		{ebiten.KeySemicolon, ";", 28}, {ebiten.KeyQuote, "'", 29}, {ebiten.KeyEnter, "↵", 31},
	},
	{ // Z 行 (最低)
		{ebiten.KeyZ, "Z", 0}, {ebiten.KeyX, "X", 2}, {ebiten.KeyC, "C", 4},
		{ebiten.KeyV, "V", 5}, {ebiten.KeyB, "B", 7}, {ebiten.KeyN, "N", 9},
		{ebiten.KeyM, "M", 11}, {ebiten.KeyComma, ",", 12}, {ebiten.KeyPeriod, ".", 14},
		{ebiten.KeySlash, "/", 16},
	},
}

// flattenCaps 四排展开 (轮询用)。
func flattenCaps() []keyCap {
	var out []keyCap
	for _, row := range capRows {
		out = append(out, row...)
	}
	return out
}

// ComputerKeys 电脑键盘输入源 (FreePiano 式全白键 + 调号转调)。
// 按键固定力度; 仅窗口聚焦时生效。
type ComputerKeys struct {
	base    int // Z 键对应的 MIDI 音符 (默认 C2=36, FreePiano 同款)
	keySig  int // 调号 (半音): 所有键发音整体平移 (F3 升 / F4 降)
	held    map[ebiten.Key]int
	handler Handler
	f3, f4  bool // F 键边沿检测
	f5, f6  bool
}

// NewComputerKeys base 为 Z 键对应的 MIDI 音符 (默认 36 = C2)。
func NewComputerKeys(base int, h Handler) *ComputerKeys {
	return &ComputerKeys{base: base, keySig: 0, held: make(map[ebiten.Key]int), handler: h}
}

func (c *ComputerKeys) Name() string { return "电脑键盘 (FreePiano 式全白键)" }

// Base 当前基准 (Z 键音符)。
func (c *ComputerKeys) Base() int { return c.base }

// KeySignature 当前调号 (半音)。
func (c *ComputerKeys) KeySignature() int { return c.keySig }

// SetBase 调整基准八度; 已按住的键先全部释放 (避免悬音)。
func (c *ComputerKeys) SetBase(base int) {
	if base == c.base || base < core.MinNote || base+57 > core.MaxNote {
		return
	}
	for k, n := range c.held {
		c.handler(core.Event{Kind: core.KindNoteOff, Ch: 0, Note: n})
		delete(c.held, k)
	}
	c.base = base
}

// SetKeySignature 设置调号 (-11..+11 半音)。只影响后续发音, 不产生事件。
func (c *ComputerKeys) SetKeySignature(sig int) {
	if sig < -11 {
		sig = -11
	}
	if sig > 11 {
		sig = 11
	}
	c.keySig = sig
}

// noteOf 键帽的发音音符 = 基准 + 白键偏移 + 调号。越界返回 false。
func (c *ComputerKeys) noteOf(m keyCap) (int, bool) {
	n := c.base + m.note + c.keySig
	if n < 0 || n > 127 {
		return 0, false
	}
	return n, true
}

// Rows 键帽面板数据: 四排键帽 (数字为转调后的简谱)。
// Held 标记物理键按下态 —— 高亮按物理键, 而非按音符 (避免同音多键帽齐亮)。
func (c *ComputerKeys) Rows() [][]KeyLabel {
	rows := make([][]KeyLabel, 0, len(capRows))
	for _, row := range capRows {
		var out []KeyLabel
		for _, m := range row {
			if n, ok := c.noteOf(m); ok {
				_, held := c.held[m.key]
				out = append(out, KeyLabel{KeyText: m.cap, Note: n, Held: held})
			}
		}
		rows = append(rows, out)
	}
	return rows
}

// Labels 扁平键位 (兼容旧接口)。
func (c *ComputerKeys) Labels() []KeyLabel {
	var out []KeyLabel
	for _, row := range c.Rows() {
		out = append(out, row...)
	}
	return out
}

// Update 每帧轮询: F 键功能 (FreePiano 同款) + 按键状态迁移 → on/off 事件。
func (c *ComputerKeys) Update() {
	// F3/F4: 调号升降
	f3 := ebiten.IsKeyPressed(ebiten.KeyF3)
	if f3 && !c.f3 {
		c.SetKeySignature(c.keySig + 1)
	}
	c.f3 = f3
	f4 := ebiten.IsKeyPressed(ebiten.KeyF4)
	if f4 && !c.f4 {
		c.SetKeySignature(c.keySig - 1)
	}
	c.f4 = f4
	// F5/F6: 八度升降
	f5 := ebiten.IsKeyPressed(ebiten.KeyF5)
	if f5 && !c.f5 {
		c.SetBase(c.base + 12)
	}
	c.f5 = f5
	f6 := ebiten.IsKeyPressed(ebiten.KeyF6)
	if f6 && !c.f6 {
		c.SetBase(c.base - 12)
	}
	c.f6 = f6

	for _, m := range flattenCaps() {
		note, ok := c.noteOf(m)
		if !ok {
			continue
		}
		pressed := ebiten.IsKeyPressed(m.key)
		_, wasHeld := c.held[m.key]
		switch {
		case pressed && !wasHeld:
			c.held[m.key] = note
			c.handler(core.Event{Kind: core.KindNoteOn, Ch: 0, Note: note, Vel: HumanVelocity})
		case !pressed && wasHeld:
			delete(c.held, m.key)
			c.handler(core.Event{Kind: core.KindNoteOff, Ch: 0, Note: note})
		}
	}
}

func (c *ComputerKeys) Close() {}

// HumanVelocity 电脑键盘固定力度。
const HumanVelocity = 100

// AutoBase 谱面自适应基准: 取谱面音符中位数, 使四排白键窗口
// [base, base+57] 的中心 (base+28) 对准中位数, 向下取整到 C,
// 夹到合法范围 [24, 51] (Z 行最低不低于 C1, 顶部不超过 C8)。
func AutoBase(events []core.Event) int {
	notes := make([]int, 0, len(events))
	for _, e := range events {
		if e.Kind == core.KindNoteOn {
			notes = append(notes, e.Note)
		}
	}
	if len(notes) == 0 {
		return 36
	}
	sort.Ints(notes)
	median := notes[len(notes)/2]
	base := median - 28
	base -= ((base % 12) + 12) % 12 // 向下取整到 C
	if base < 24 {
		base = 24
	}
	if base > 51 {
		base = 51
	}
	return base
}
