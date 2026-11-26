package frontend

import (
	"image/color"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"

	"github.com/ljlsasuke/my-band/internal/core"
	"github.com/ljlsasuke/my-band/internal/input"
)

// 下落音符可视参数 (PLAN.md M4): 音符提前 fallAheadSec 出现在顶部,
// 以恒速落向键位线, 落达瞬间 = NoteOn 时刻。
const fallAheadSec = 1.5

// Stage 是 Ebiten 薄壳: 总线的动画消费者 (只"演", 不发声)。
// 事件经 Push 进入前瞻队列, Update 每帧按主时钟 SongTime 取出到期事件
// 迁移视觉状态 —— 音频是主时钟, 动画追它 (PLAN.md §5.3 时钟模型)。
// M4: 顶部下落音符分色 (按 GM 乐器族) + M4.5: 可插拔键盘输入源 +
// 键位显示条 (Tab 切换)。
type Stage struct {
	kb      *keyboardState
	tr      *tracker
	pending []core.Event // 即时事件 (人类演奏 / 协议 note), T = 当前时刻
	pendMu  sync.Mutex   // 保护 pending: MidiIn 回调线程 Push, 渲染帧排空
	events  []core.Event // 完整谱面事件 (自驱动画前瞻 1.5s, 独立于音频 50ms 窗口)
	eIdx    int          // 下一个待应用谱面事件下标
	clock   func() float64
	src     input.Source // 当前键盘输入源 (M4.5: 电脑键盘 / MIDI 键盘)

	showKeymap bool // 键位显示条开关 (Tab 切换)
	tabPrev    bool // Tab 上一帧状态 (边沿检测)
	face       *basicfont.Face

	statusMu  sync.Mutex
	statusMsg string // 设备/后端错误提示 (GUI 可见化, 远程用户看不到控制台)
}

func New() *Stage {
	return &Stage{kb: newKeyboardState(), tr: newTracker(), showKeymap: true, face: basicfont.Face7x13}
}

// SetClock 注入主时钟查询。需并发安全。
func (s *Stage) SetClock(f func() float64) { s.clock = f }

// SetInputSource 注入键盘输入源 (M4.5)。Update 每帧驱动它。
func (s *Stage) SetInputSource(src input.Source) { s.src = src }

// SetStatusMessage 设置/清除状态提示 (设备错误等)。远程用户看不到控制台,
// 错误必须在 GUI 里可见。传空字符串清除。
func (s *Stage) SetStatusMessage(msg string) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	s.statusMsg = msg
}

// Push 事件入队。可能来自 winmm 回调线程 (MIDI 键盘), 加锁保护。
func (s *Stage) Push(e core.Event) {
	s.pendMu.Lock()
	defer s.pendMu.Unlock()
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
	s.pendMu.Lock()
	s.pending = nil
	s.pendMu.Unlock()
}

// Clear 清空即时队列。
func (s *Stage) Clear() {
	s.pendMu.Lock()
	s.pending = nil
	s.pendMu.Unlock()
}

// ResetVisual 停止播放时的全量视觉复位 (时间轴即将切换)。
func (s *Stage) ResetVisual() {
	s.Clear()
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

// Update 每帧: 输入源轮询 → Tab 切换键位条 → 谱面事件自驱 (前瞻 1.5s)
// + 即时事件 → 状态迁移。
func (s *Stage) Update() error {
	t := s.now()

	// 键位显示条开关 (Tab 边沿检测)
	tab := ebiten.IsKeyPressed(ebiten.KeyTab)
	if tab && !s.tabPrev {
		s.showKeymap = !s.showKeymap
	}
	s.tabPrev = tab

	// 输入源: 电脑键盘在此帧轮询; MIDI 键盘在 Update 里排空回调队列
	if s.src != nil {
		s.src.Update()
	}

	// 谱面事件流: 自驱指针, 提前 fallAheadSec 应用 —— 状态机全时间戳驱动,
	// 未来按压 level=0 不提前显形, 但下落块得以提前诞生 (否则块只在落达瞬间创建,
	// 永远没有"下落中"状态, 这就是下落区全黑的根因)
	for s.eIdx < len(s.events) && s.events[s.eIdx].T <= t+fallAheadSec {
		e := s.events[s.eIdx]
		s.eIdx++
		s.kb.apply(e)
		s.tr.apply(e)
	}

	// 即时事件 (人类/协议 note): 它们产生于"此刻", 无条件立即应用。
	// 此前按 T<=帧起点 过滤, 而事件时间戳比帧起点晚几微秒 → 滞留到下一帧;
	// 远程桌面下帧率极低, 表现为"键帽亮了但钢琴键不亮" (用户实测踩坑)。
	s.pendMu.Lock()
	for _, e := range s.pending {
		s.kb.apply(e)
		s.tr.apply(e)
	}
	s.pending = s.pending[:0]
	s.pendMu.Unlock()

	s.tr.gc(t)
	return nil
}

// Draw 渲染: 上半 = 下落音符区, 中下 = 键盘, 底部 = 键位显示条 (可开关)。
func (s *Stage) Draw(screen *ebiten.Image) {
	t := s.now()
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	vector.DrawFilledRect(screen, 0, 0, float32(w), float32(h),
		color.RGBA{0x10, 0x10, 0x14, 0xff}, false)

	// 底部键帽面板高度 (开启时键盘整体上移让位)
	stripH := 0
	if s.showKeymap {
		stripH = capAreaHeight(s.src)
	}

	kbTop := float64(h) * 0.45
	kbH := float64(h) - kbTop - float64(stripH)
	rects := ComputeLayout(MinNote, MaxNote)

	// ---- 下落音符 (M4) ----
	pps := kbTop / fallAheadSec
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
		bottom := kbTop - (b.t0-t)*pps
		top := kbTop - (t1-t)*pps
		if top < 0 {
			top = 0
		}
		if bottom > kbTop {
			bottom = kbTop
		}
		if top >= bottom {
			continue
		}
		c := b.fam.Color()
		alpha := uint8(0xd0)
		if b.open {
			alpha = 0xff
		}
		vector.DrawFilledRect(screen, float32(x), float32(top), float32(kw), float32(bottom-top),
			color.RGBA{c.R, c.G, c.B, alpha}, true)
	}

	// ---- 状态提示 (设备错误等, 严格可见) ----
	s.statusMu.Lock()
	msg := s.statusMsg
	s.statusMu.Unlock()
	if msg != "" {
		vector.DrawFilledRect(screen, 0, float32(kbTop-20), float32(w), 20,
			color.RGBA{0x5a, 0x1e, 0x1e, 0xe8}, false)
		text.Draw(screen, "⚠ "+msg, s.face, 10, int(kbTop)-6,
			color.RGBA{0xff, 0xc0, 0xc0, 0xff})
	}

	// ---- 键盘 ----
	for _, r := range rects {
		if !r.Black {
			s.drawKey(screen, r, t, w, kbTop, kbH)
		}
	}
	for _, r := range rects {
		if r.Black {
			s.drawKey(screen, r, t, w, kbTop, kbH)
		}
	}

	// ---- 键帽面板 (FreePiano 式, M4.5) ----
	if stripH > 0 {
		s.drawKeycaps(screen, w, int(kbTop+kbH), stripH)
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
	// 关闭垂直同步: 远程桌面 (RDP) 下 vsync 会把帧率拖到极低,
	// 本应用渲染极简, 无撕裂顾虑
	ebiten.SetVsyncEnabled(false) // 远程桌面 (RDP) 下 vsync 拖垮帧率; 渲染极简无撕裂顾虑
	return ebiten.RunGame(s)
}
