package frontend

import (
	"github.com/ljlsasuke/pianostage/internal/core"
)

// 动画参数 (PLAN.md §5.1 MVP 定稿): 按下快速上升, 释放 180ms 线性衰减。
const (
	AttackSec = 0.04
	DecaySec  = 0.18
)

// press 一次活跃按压。
type press struct {
	vel int
	t   float64 // 曲内按压时刻
	fam Family  // 乐器族 (M4: 按键/下落音符按族着色)
}

// keyState 单键视觉状态。多声部同键按压用栈计数 (弦乐+钢琴同弹一个音
// 时, 释放其中之一不应抬起琴键)。
type keyState struct {
	presses        []press
	pressT         float64 // 最近一次按压时刻 (attack 动画基准)
	releaseT       float64 // 最近一次释放时刻 (decay 动画基准)
	levelAtRelease float64
	pedalHeld      bool    // 延音踏板挂住: 视觉保持按下, 踏板抬起时才衰减
}

// keyboardState 全键盘视觉状态机 (纯逻辑, 渲染层每帧查询 levelAt)。
type keyboardState struct {
	keys     [128]keyState
	programs map[int]int // ch -> 当前 GM program (M4 按族着色)
	pedal    bool        // CC64 延音踏板
}

func newKeyboardState() *keyboardState {
	return &keyboardState{programs: make(map[int]int)}
}

// apply 应用一个已到期的事件, 迁移视觉状态。
// 同刻顺序由解析层保证 (NoteOff 先于 NoteOn), 此处不必重排。
func (k *keyboardState) apply(e core.Event) {
	if e.Note < 0 || e.Note > 127 {
		return
	}
	key := &k.keys[e.Note]
	switch e.Kind {
	case core.KindProgram:
		k.programs[e.Ch] = e.Prog
		return
	case core.KindNoteOn:
		if len(key.presses) == 0 {
			key.pressT = e.T // 从完全释放状态重新起按
		}
		key.presses = append(key.presses, press{vel: e.Vel, t: e.T, fam: FamilyOf(e.Ch, k.programs[e.Ch])})
	case core.KindNoteOff:
		if len(key.presses) == 0 {
			return // 无配对的 off (防御): 忽略
		}
		key.levelAtRelease = k.levelAt(key, e.T)
		key.releaseT = e.T
		key.presses = key.presses[:len(key.presses)-1]
		if len(key.presses) == 0 && k.pedal {
			key.pedalHeld = true // 音符结束但踏板挂住: 视觉保持
		}
	case core.KindControl:
		if e.Ctrl == 64 { // 延音踏板
			down := e.CtrlVal >= 64
			if k.pedal && !down {
				// 踏板抬起: 所有被挂住的键统一进入释放衰减
				for i := range k.keys {
					if k.keys[i].pedalHeld {
						k.keys[i].pedalHeld = false
						k.keys[i].releaseT = e.T
						k.keys[i].levelAtRelease = 1 // 踏板期间保持满按压视觉
					}
				}
			}
			k.pedal = down
		}
	}
}

// famAt 键在时刻 t 的着色乐器族: 取视觉量最大的按压的族;
// 无按压时返回 FamKeys (默认)。
func (k *keyboardState) famAt(key *keyState, t float64) Family {
	best, bestLvl := FamKeys, -1.0
	for _, p := range key.presses {
		if l := float64(p.vel) / 127 * clamp01((t-p.t)/AttackSec); l > bestLvl {
			best, bestLvl = p.fam, l
		}
	}
	return best
}

// levelAt 计算某键在曲内时刻 t 的按压视觉量 (0..1)。
// 力度 → 目标深度: vel/127; attack 从按压时刻起快速上升;
// release 从释放时刻起 180ms 线性衰减; 暂停时 t 冻结 → 动画自然冻结。
func (k *keyboardState) levelAt(key *keyState, t float64) float64 {
	if len(key.presses) > 0 {
		target := 0.0
		for _, p := range key.presses {
			l := float64(p.vel) / 127 * clamp01((t-p.t)/AttackSec)
			if l > target {
				target = l
			}
		}
		return target
	}
	if key.pedalHeld {
		return key.levelAtRelease
	}
	d := (t - key.releaseT) / DecaySec
	if d >= 1 {
		return 0
	}
	return clamp01(key.levelAtRelease * (1 - d))
}

// level 对外便捷查询。
func (k *keyboardState) level(note int, t float64) float64 {
	if note < 0 || note > 127 {
		return 0
	}
	return k.levelAt(&k.keys[note], t)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
