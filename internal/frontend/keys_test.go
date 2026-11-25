package frontend

import (
	"math/rand"
	"testing"

	"github.com/ljlsasuke/my-band/internal/core"
)

func TestComputeLayout88Keys(t *testing.T) {
	rects := ComputeLayout(MinNote, MaxNote)
	if len(rects) != 88 {
		t.Fatalf("88 键, 实得 %d", len(rects))
	}
	whites := 0
	for _, r := range rects {
		if !r.Black {
			whites++
		}
	}
	if whites != 52 || WhiteCount(MinNote, MaxNote) != 52 {
		t.Fatalf("88 键布局应含 52 白键, 实得 %d", whites)
	}
}

func TestBlackKeyCenteredOnBoundary(t *testing.T) {
	rects := ComputeLayout(MinNote, MaxNote)
	ww := 1.0 / 52.0
	bw := ww * 0.62
	wIdx := 0
	for _, r := range rects {
		if r.Black {
			continue
		}
		wIdx++
	}
	// 重算: 黑键中心应落在某条白键分界线上 (wIdx*ww)
	wIdx = 0
	for _, r := range rects {
		if !r.Black {
			wIdx++
			continue
		}
		center := r.X + r.W/2
		boundary := float64(wIdx) * ww
		if diff := center - boundary; diff < -1e-9 || diff > 1e-9 {
			t.Fatalf("note %d 黑键中心 %v 未对齐分界线 %v", r.Note, center, boundary)
		}
	}
	if bw <= 0 {
		t.Fatal("unreachable")
	}
}

func TestKeysWithinBounds(t *testing.T) {
	for _, r := range ComputeLayout(MinNote, MaxNote) {
		if r.X < -1e-9 || r.X+r.W > 1+1e-9 {
			t.Fatalf("note %d 越出键盘区域: [%v, %v]", r.Note, r.X, r.X+r.W)
		}
	}
}

func TestAttackDecayLifecycle(t *testing.T) {
	var k keyboardState
	// 强按压 (vel=127): attack 后满量
	k.apply(core.Event{T: 0, Kind: core.KindNoteOn, Ch: 0, Note: 60, Vel: 127})
	if l := k.level(60, 0.0); l != 0 {
		t.Errorf("按压瞬间 level = %v, want 0", l)
	}
	if l := k.level(60, AttackSec); l != 1 {
		t.Errorf("attack 完成后 level = %v, want 1", l)
	}
	// 释放: 180ms 线性衰减
	k.apply(core.Event{T: 1.0, Kind: core.KindNoteOff, Ch: 0, Note: 60})
	if l := k.level(60, 1.0); l != 1 {
		t.Errorf("释放瞬间 level = %v, want 1 (从满量开始衰减)", l)
	}
	if l := k.level(60, 1.09); l < 0.49 || l > 0.51 {
		t.Errorf("衰减半程 level = %v, want ~0.5", l)
	}
	if l := k.level(60, 1.18); l > 1e-9 {
		t.Errorf("衰减完成 level = %v, want ≈0 (浮点残差容忍)", l)
	}
}

func TestVelocityControlsDepth(t *testing.T) {
	var k keyboardState
	k.apply(core.Event{T: 0, Kind: core.KindNoteOn, Ch: 0, Note: 60, Vel: 32}) // 弱奏 ~0.25
	if l := k.level(60, 1.0); l < 0.24 || l > 0.26 {
		t.Errorf("弱奏深度 = %v, want ~0.25", l)
	}
	// 同键强奏覆盖: 重触后按最强按压计
	k.apply(core.Event{T: 2.0, Kind: core.KindNoteOn, Ch: 1, Note: 60, Vel: 127})
	if l := k.level(60, 3.0); l != 1 {
		t.Errorf("强奏叠加后 depth = %v, want 1", l)
	}
	// 只释放声道 1 的按压, 声道 0 仍按着 → 不衰减
	k.apply(core.Event{T: 3.0, Kind: core.KindNoteOff, Ch: 1, Note: 60})
	if l := k.level(60, 3.5); l < 0.24 || l > 0.26 {
		t.Errorf("多声部同键: 释放其一不应抬键, level = %v", l)
	}
}

func TestSustainPedalVisual(t *testing.T) {
	var k keyboardState
	k.apply(core.Event{T: 0, Kind: core.KindNoteOn, Ch: 0, Note: 60, Vel: 127})
	k.apply(core.Event{T: 1, Kind: core.KindControl, Ch: 0, Ctrl: 64, CtrlVal: 127}) // 踏板踩下
	k.apply(core.Event{T: 2, Kind: core.KindNoteOff, Ch: 0, Note: 60})               // 音符结束
	if l := k.level(60, 3.0); l != 1 {
		t.Errorf("踏板挂住的键应保持按下, level = %v", l)
	}
	k.apply(core.Event{T: 4, Kind: core.KindControl, Ch: 0, Ctrl: 64, CtrlVal: 0}) // 抬踏板
	if l := k.level(60, 4.2); l != 0 {
		t.Errorf("踏板抬起 + 衰减完成, level = %v, want 0", l)
	}
}

func TestNoteOffWithoutOnIgnored(t *testing.T) {
	var k keyboardState
	k.apply(core.Event{T: 0, Kind: core.KindNoteOff, Ch: 0, Note: 60}) // 无配对 off: 忽略
	if l := k.level(60, 0.5); l != 0 {
		t.Errorf("无配对 off 不应产生视觉变化, level = %v", l)
	}
}

func TestEventsSortedInternally(t *testing.T) {
	// 随机事件流: apply 不得 panic, level 始终在 [0,1]
	var k keyboardState
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 10000; i++ {
		note := 21 + rng.Intn(88)
		switch rng.Intn(3) {
		case 0:
			k.apply(core.Event{T: float64(i) / 100, Kind: core.KindNoteOn, Ch: rng.Intn(16), Note: note, Vel: rng.Intn(127) + 1})
		case 1:
			k.apply(core.Event{T: float64(i) / 100, Kind: core.KindNoteOff, Ch: rng.Intn(16), Note: note})
		case 2:
			k.apply(core.Event{T: float64(i) / 100, Kind: core.KindControl, Ch: 0, Ctrl: 64, CtrlVal: rng.Intn(128)})
		}
		for n := MinNote; n <= MaxNote; n++ {
			if l := k.level(n, float64(i)/100); l < 0 || l > 1 {
				t.Fatalf("level 越界: %v", l)
			}
		}
	}
}
