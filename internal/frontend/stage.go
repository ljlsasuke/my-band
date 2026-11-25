package frontend

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/ljlsasuke/my-band/internal/core"
)

// 下落音符可视参数 (PLAN.md M4): 音符提前 fallAheadSec 出现在顶部,
// 以恒速落向键位线, 落达瞬间 = NoteOn 时刻。
const fallAheadSec = 1.5

// Stage 是 Ebiten 薄壳: 总线的动画消费者 (只"演", 不发声)。
// 事件经 Push 进入前瞻队列, Update 每帧按主时钟 SongTime 取出到期事件
// 迁移视觉状态 —— 音频是主时钟, 动画追它 (PLAN.md §5.3 时钟模型)。
// M4: 顶部下落音符分色 (按 GM 乐器族) + 人类键盘演奏输入。
type Stage struct {
	kb      *keyboardState
	tr      *tracker
	pending []core.Event // 即时事件 (人类演奏 / 协议 note), T = 当前时刻
	events  []core.Event // 完整谱面事件 (自驱动画前瞻 1.5s, 独立于音频 50ms 窗口)
	eIdx    int          // 下一个待应用谱面事件下标
	clock   func() float64
	human   *HumanInput
	inputFn func(note int, on bool, vel int)
}

func New() *Stage {
	return &Stage{kb: newKeyboardState(), tr: newTracker()}
}

// SetClock 注入主时钟查询。需并发安全。
func (s *Stage) SetClock(f func() float64) { s.clock = f }

// SetInputHandler 注入人类演奏事件出口 (cli 接后端发声)。
// handler 在渲染帧内被调用, 应快速返回。
func (s *Stage) SetInputHandler(fn func(note int, on bool, vel int)) {
	s.inputFn = fn
	s.human = NewHumanInput(60, func(note int, on bool, vel int) {
		fn(note, on, vel)
	})
}

// Push 事件入队 (调度器 emit 回调调用, 早于实际发声 ~前瞻窗口)。
func (s *Stage) Push(e core.Event) {
	s.pending = append(s.pending, e)
}

// SetEvents 设置完整谱面事件流 (自驱动画前瞻)。须按 T 升序。
func (s *Stage) SetEvents(events []core.Event) {
	s.events = events
	s.eIdx = 0
}

// Seek 谱面时间轴跳转 (同步调度器), 丢弃即时队列。
func (s *Stage) Seek(pos float64) {
	s.eIdx = 0
	for s.eIdx < len(s.events) && s.events[s.eIdx].T < pos {
		s.eIdx++
	}
	s.pending = nil
}

// Clear 清空即时队列。
func (s *Stage) Clear() { s.pending = nil }

// ResetVisual 停止播放时的全量视觉复位 (时间轴即将切换)。
func (s *Stage) ResetVisual() {
	s.pending = nil
	s.eIdx = 0
	s.tr = newTracker()
	s.kb = newKeyboardState()
}

func (s *Stage) now() float64 {
	if s.clock == nil {
		return 0
	}
	return s.clock()
}

// Update 每帧: 人类输入轮询 → 谱面事件自驱 (前瞻 1.5s) + 即时事件 → 状态迁移。
func (s *Stage) Update() error {
	t := s.now()
	if s.human != nil {
		s.human.Update()
	}
	// 谱面事件流: 自驱指针, 提前 fallAheadSec 应用 —— 状态机全时间戳驱动,
	// 未来按压 level=0 不提前显形, 但下落块得以提前诞生 (否则块只在落达瞬间创建,
	// 永远没有“下落中”状态, 这就是下落区全黑的根因)
	for s.eIdx < len(s.events) && s.events[s.eIdx].T <= t+fallAheadSec {
		e := s.events[s.eIdx]
		s.eIdx++
		s.kb.apply(e)
		s.tr.apply(e)
	}
	// 即时事件 (人类/协议 note, T = 入队时刻)
	kept := s.pending[:0]
	for _, e := range s.pending {
		if e.T <= t {
			s.kb.apply(e)
			s.tr.apply(e)
		} else {
			kept = append(kept, e)
		}
	}
	s.pending = kept
	s.tr.gc(t)
	return nil
}

// Draw 渲染: 上半 = 下落音符区, 下半 = 键盘。
func (s *Stage) Draw(screen *ebiten.Image) {
	t := s.now()
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// 背景分两层色, 视觉上区分下落区与键盘区
	vector.DrawFilledRect(screen, 0, 0, float32(w), float32(h),
		color.RGBA{0x10, 0x10, 0x14, 0xff}, false)

	kbTop := float64(h) * 0.55
	kbH := float64(h) - kbTop
	rects := ComputeLayout(MinNote, MaxNote)

	// ---- 下落音符 (M4) ----
	pps := kbTop / fallAheadSec // 像素/秒: 提前 fallAheadSec 出现在顶部
	for _, b := range s.tr.visible(t) {
		t1 := b.t1
		if b.open {
			t1 = t
		}
		var kr *KeyRect
		for i := range rects {
			if rects[i].Note == b.note {
				kr = &rects[i]
				break
			}
		}
		if kr == nil {
			continue
		}
		x := kr.X * float64(w)
		kw := kr.W * float64(w)
		bottom := kbTop - (b.t0-t)*pps // t0 后继续下沉, 被键盘区裁剪 (Synthesia 式)
		top := kbTop - (t1-t)*pps
		if top < 0 {
			top = 0
		}
		if bottom > kbTop {
			bottom = kbTop // 发声中的块钉在键位线上, 随释放临近收缩
		}
		if top >= bottom {
			continue // 已完全越过键位线 (释放后)
		}
		c := b.fam.Color()
		alpha := uint8(0xd0)
		if b.open {
			alpha = 0xff // 正在响的音最实
		}
		vector.DrawFilledRect(screen, float32(x), float32(top), float32(kw), float32(bottom-top),
			color.RGBA{c.R, c.G, c.B, alpha}, true)
	}

	// ---- 键盘 (两遍: 先白后黑) ----
	for _, r := range rects {
		if r.Black {
			continue
		}
		s.drawKey(screen, r, t, w, kbTop, kbH)
	}
	for _, r := range rects {
		if r.Black {
			s.drawKey(screen, r, t, w, kbTop, kbH)
		}
	}
}

func (s *Stage) drawKey(screen *ebiten.Image, r KeyRect, t float64, W int, kbTop, kbH float64) {
	level := s.kb.level(r.Note, t)
	fam := s.kb.famAt(&s.kb.keys[r.Note], t)
	x := r.X * float64(W)
	kw := r.W * float64(W)
	kh := r.H * float64(kbH)
	y := kbTop + r.Y*kbH

	fc := fam.Color()
	if r.Black {
		base := [3]uint8{0x1a, 0x1a, 0x1a}
		sink := 3.0 / kbH * level
		c := lerpColor(base, [3]uint8{fc.R, fc.G, fc.B}, level*0.85)
		vector.DrawFilledRect(screen, float32(x), float32(y), float32(kw), float32(kh*(1-sink)), rgba(c), true)
		return
	}
	white := [3]uint8{0xf5, 0xf5, 0xf0}
	sink := 4.0 / kbH * level
	c := lerpColor(white, [3]uint8{fc.R, fc.G, fc.B}, level)
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(kw), float32(kh*(1-sink)), rgba(c), false)
	vector.DrawFilledRect(screen, float32(x), float32(y+kh*(1-sink)), float32(kw), float32(kh*sink),
		color.RGBA{0x08, 0x08, 0x0a, 0xff}, false)
	vector.DrawFilledRect(screen, float32(x+kw-1), float32(y), 1, float32(kh),
		color.RGBA{0x2a, 0x2a, 0x2e, 0xff}, false)
}

func lerpColor(from, to [3]uint8, t float64) [3]uint8 {
	var out [3]uint8
	for i := 0; i < 3; i++ {
		d := float64(to[i]) - float64(from[i])
		out[i] = uint8(float64(from[i]) + d*t)
	}
	return out
}

func rgba(c [3]uint8) color.RGBA { return color.RGBA{c[0], c[1], c[2], 0xff} }

// Layout 跟随窗口尺寸。
func (s *Stage) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

// Run 阻塞运行 GUI (必须在主 goroutine 调用)。窗口关闭即返回。
func (s *Stage) Run(title string) error {
	ebiten.SetWindowTitle(title)
	ebiten.SetWindowSize(1100, 500)
	return ebiten.RunGame(s)
}
