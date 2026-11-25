package frontend

import (
	"testing"

	"github.com/ljlsasuke/pianostage/internal/core"
)

func TestStageFallingVisible(t *testing.T) {
	st := New()
	tNow := 0.0
	st.SetClock(func() float64 { return tNow })
	evs := []core.Event{
		{T: 0.1, Kind: core.KindNoteOn, Ch: 0, Note: 60, Vel: 90},
		{T: 1.0, Kind: core.KindNoteOff, Ch: 0, Note: 60},
		{T: 2.0, Kind: core.KindNoteOn, Ch: 0, Note: 64, Vel: 90},
		{T: 3.0, Kind: core.KindNoteOff, Ch: 0, Note: 64},
	}
	st.SetEvents(evs)

	// t=0.5: note60 发声中, note64 在 1.5s 预告窗内 (2.0-0.5=1.5 边界)
	tNow = 0.5
	if err := st.Update(); err != nil {
		t.Fatal(err)
	}
	blocks := st.tr.visible(tNow)
	t.Logf("t=0.5 visible=%d blocks=%d eIdx=%d", len(blocks), len(st.tr.blocks), st.eIdx)
	if len(blocks) < 1 {
		t.Fatalf("t=0.5 应至少可见 1 块 (发声中)")
	}

	// t=1.2: note64 落达前 0.8s → 应出现在下落区
	tNow = 1.2
	st.Update()
	blocks = st.tr.visible(tNow)
	t.Logf("t=1.2 visible=%d blocks=%d", len(blocks), len(st.tr.blocks))
	if len(blocks) < 1 {
		t.Fatalf("t=1.2 应见下落中的 note64")
	}
}
