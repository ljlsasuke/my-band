package frontend

import (
	"github.com/ljlsasuke/pianostage/internal/core"
)

// noteBlock 一枚下落音符块: 从顶部落到键位线, 落达时刻 = t0 (NoteOn),
// 抬起时刻 = t1 (NoteOff); 未释放时 t1 持续增长 (长音块拉长)。
type noteBlock struct {
	ch, note int
	t0, t1   float64
	fam      Family
	open     bool
}

// tracker 下落音符追踪器 (纯逻辑, 渲染层每帧查询 visible)。
// 同时维护各声道当前音色 (Program 变更 → 影响颜色)。
type tracker struct {
	blocks   []noteBlock
	programs map[int]int // ch -> 当前 GM program
}

func newTracker() *tracker {
	return &tracker{programs: make(map[int]int)}
}

// programFor 声道当前音色 (默认大钢琴)。
func (tr *tracker) programFor(ch int) int { return tr.programs[ch] }

// apply 迁移状态。同刻顺序由解析层保证 (NoteOff 先于 NoteOn / Program 先于 NoteOn)。
func (tr *tracker) apply(e core.Event) {
	if e.Note < 0 || e.Note > 127 || e.Ch < 0 || e.Ch > 15 {
		return
	}
	switch e.Kind {
	case core.KindProgram:
		tr.programs[e.Ch] = e.Prog
	case core.KindNoteOn:
		// 同 (ch,note) 未释放的重复 on: 解析层已截断, 此处防御性关闭
		for i := range tr.blocks {
			b := &tr.blocks[i]
			if b.open && b.ch == e.Ch && b.note == e.Note {
				b.t1, b.open = e.T, false
			}
		}
		tr.blocks = append(tr.blocks, noteBlock{
			ch: e.Ch, note: e.Note, t0: e.T,
			fam:  FamilyOf(e.Ch, tr.programFor(e.Ch)),
			open: true,
		})
	case core.KindNoteOff:
		// 关闭最近开启的同 (ch,note) 块
		for i := len(tr.blocks) - 1; i >= 0; i-- {
			b := &tr.blocks[i]
			if b.open && b.ch == e.Ch && b.note == e.Note {
				b.t1, b.open = e.T, false
				return
			}
		}
	}
}

// gc 清理已结束且完全离开可视区的旧块。
func (tr *tracker) gc(t float64) {
	kept := tr.blocks[:0]
	for _, b := range tr.blocks {
		if b.open || b.t1 > t-fallAheadSec {
			kept = append(kept, b)
		}
	}
	tr.blocks = kept
}

// visible 取出与可视时间窗 [t, t+ahead] 相交的音符块快照。
func (tr *tracker) visible(t float64) []noteBlock {
	var out []noteBlock
	for _, b := range tr.blocks {
		t1 := b.t1
		if b.open {
			t1 = t // 未释放: 视为延伸到当前 (块随长音增长)
		}
		if t1 < t || b.t0 > t+fallAheadSec {
			continue
		}
		out = append(out, b)
	}
	return out
}
