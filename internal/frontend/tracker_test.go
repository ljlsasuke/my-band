package frontend

import (
	"testing"

	"github.com/ljlsasuke/my-band/internal/core"
)

func TestTrackerProgramColor(t *testing.T) {
	tr := newTracker()
	// Program 先于 NoteOn → 颜色按新音色
	tr.apply(core.Event{T: 0, Kind: core.KindProgram, Ch: 0, Prog: 48}) // 弦乐
	tr.apply(core.Event{T: 1, Kind: core.KindNoteOn, Ch: 0, Note: 60, Vel: 90})
	blocks := tr.visible(1)
	if len(blocks) != 1 || blocks[0].fam != FamStrings {
		t.Fatalf("弦乐音色应染蓝: %+v", blocks)
	}
}

func TestTrackerOpenBlockGrowsAndCloses(t *testing.T) {
	tr := newTracker()
	tr.apply(core.Event{T: 0, Kind: core.KindNoteOn, Ch: 0, Note: 60, Vel: 90})
	b := tr.visible(2)[0]
	if !b.open || b.t0 != 0 {
		t.Fatalf("t=2 时块应 open 且 t0=0: %+v", b)
	}
	tr.apply(core.Event{T: 3, Kind: core.KindNoteOff, Ch: 0, Note: 60})
	b = tr.visible(3)[0]
	if b.open || b.t1 != 3 {
		t.Fatalf("t=3 时块应关闭: %+v", b)
	}
	// 离开可视窗后被 gc
	tr.gc(3 + fallAheadSec)
	if len(tr.visible(3+fallAheadSec)) != 0 {
		t.Error("旧块应被回收")
	}
}

func TestTrackerRetriggerDefense(t *testing.T) {
	tr := newTracker()
	tr.apply(core.Event{T: 0, Kind: core.KindNoteOn, Ch: 0, Note: 60, Vel: 90})
	tr.apply(core.Event{T: 1, Kind: core.KindNoteOn, Ch: 0, Note: 60, Vel: 90}) // 防御: 未配对重复 on
	blocks := tr.visible(1)
	if len(blocks) != 2 {
		t.Fatalf("截断块 (结束于此刻) + 新块 = 2, 实得 %d", len(blocks))
	}
	openCount := 0
	for _, b := range blocks {
		if b.open {
			openCount++
		}
	}
	if openCount != 1 {
		t.Errorf("防御性关闭后应只剩 1 个 open 块: %+v", blocks)
	}
}

func TestTrackerChannel9Color(t *testing.T) {
	tr := newTracker()
	tr.apply(core.Event{T: 0, Kind: core.KindNoteOn, Ch: 9, Note: 36, Vel: 100}) // 声道 9 打击
	blocks := tr.visible(0)
	if len(blocks) != 1 || blocks[0].fam != FamPerc {
		t.Fatalf("声道 9 应染红: %+v", blocks)
	}
}
