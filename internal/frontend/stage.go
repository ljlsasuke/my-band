package frontend

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/ljlsasuke/pianostage/internal/core"
)

// Stage 是 Ebiten 薄壳: 总线的动画消费者 (只"演", 不发声)。
// 事件经 Push 进入前瞻队列, Update 每帧按主时钟 SongTime 取出到期事件
// 迁移视觉状态 —— 音频是主时钟, 动画追它 (PLAN.md §5.3 时钟模型)。
type Stage struct {
	kb      keyboardState
	pending []core.Event
	clock   func() float64 // 主时钟 (Scheduler.SongTime)
}

func New() *Stage {
	return &Stage{}
}

// SetClock 注入主时钟查询 (Scheduler.SongTime)。必须在自己 goroutine
// 更新前调用; 返回值需并发安全。
func (s *Stage) SetClock(f func() float64) { s.clock = f }

// Push 事件入队 (调度器 emit 回调调用, 早于实际发声 ~前瞻窗口)。
// 回调持调度器锁, 故只做快速入队, 不做任何重活。
func (s *Stage) Push(e core.Event) {
	s.pending = append(s.pending, e)
}

func (s *Stage) now() float64 {
	if s.clock == nil {
		return 0
	}
	return s.clock()
}

// Update 每帧: 取出到期事件 → 迁移视觉状态。
func (s *Stage) Update() error {
	t := s.now()
	// pending 里的事件按 T 有序 (emit 顺序 ≈ T 序), 到期即取;
	// 未到期留在队首, 下帧再看 (前瞻窗口内的事件量很小)
	kept := s.pending[:0]
	for _, e := range s.pending {
		if e.T <= t {
			s.kb.apply(e)
		} else {
			kept = append(kept, e)
		}
	}
	s.pending = kept
	return nil
}

// Draw 渲染整幅键盘。
func (s *Stage) Draw(screen *ebiten.Image) {
	t := s.now()
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// 背景
	vector.DrawFilledRect(screen, 0, 0, float32(w), float32(h),
		color.RGBA{0x14, 0x14, 0x16, 0xff}, false)

	rects := ComputeLayout(MinNote, MaxNote)
	// 两遍绘制: 先白键后黑键 (黑键叠在白键上方)
	for _, r := range rects {
		if r.Black {
			continue
		}
		drawKey(screen, r, s.kb.level(r.Note, t), w, h)
	}
	for _, r := range rects {
		if r.Black {
			drawKey(screen, r, s.kb.level(r.Note, t), w, h)
		}
	}
}

func drawKey(screen *ebiten.Image, r KeyRect, level float64, W, H int) {
	x := r.X * float64(W)
	y := r.Y * float64(H)
	kw := r.W * float64(W)
	kh := r.H * float64(H)

	if r.Black {
		// 黑键: 深灰 → 琥珀 (按压); 满按压时整体轻微下沉 (缩短底边)
		base := uint8(0x1a)
		sink := 3.0 / float64(H) * level
		c := lerpColor([3]uint8{base, base, base}, [3]uint8{0xe8, 0xa3, 0x3d}, level*0.85)
		vector.DrawFilledRect(screen, float32(x), float32(y), float32(kw), float32(kh*(1-sink)), rgba(c), true)
		return
	}

	// 白键: 米白 → 琥珀; 下沉模拟: 底边整体上移, 露出下方深色"键槽"
	sink := 4.0 / float64(H) * level
	c := lerpColor([3]uint8{0xf5, 0xf5, 0xf0}, [3]uint8{0xe8, 0xa3, 0x3d}, level)
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(kw), float32(kh*(1-sink)), rgba(c), false)
	// 键槽阴影: 底部常驻深色带, 按压越深露出越多
	vector.DrawFilledRect(screen, float32(x), float32(y+kh*(1-sink)), float32(kw), float32(kh*sink),
		color.RGBA{0x08, 0x08, 0x0a, 0xff}, false)
	// 分隔缝
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
	ebiten.SetWindowSize(1100, 280)
	return ebiten.RunGame(s)
}
