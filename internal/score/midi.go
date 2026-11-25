// Package score 谱面解析: MIDI / 简谱 / 音名序列 → core 事件流 (PLAN.md M1/M3)。
//
// 本文件: SMF (Standard MIDI File) 解析。核心逻辑忠实移植前身项目
// auto-piano 的 parse_midi (https://github.com/ljlsasuke/auto-piano),
// 含「两遍法 tempo map」「retrigger 截断」「SMPTE 时基」三个已验证设计。
// 相对前身的增强 (GM 直连后端带来的自由, 见 PLAN.md 声音线):
//   - 保留声道 9 打击乐 (前身因 FreePiano 无鼓组而丢弃)
//   - 保留 Program Change 事件 (分声部音色的关键)
//   - 不裁剪音域 (GM 合成器支持 0~127, 前身受 FreePiano A0~C8 限制)
package score

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/ljlsasuke/pianostage/internal/core"
)

// DefaultMinDur 短于此时长(秒)的音符被丢弃——过滤解析噪声, 沿用前身参数。
const DefaultMinDur = 0.05

var errNotMidi = errors.New("不是有效的 MIDI 文件 (缺少 MThd)")

// ParseMidi 解析 MIDI 文件, 返回按曲内时间排序的 core 事件流。
func ParseMidi(path string) ([]core.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseMidiBytes(data)
}

// ParseMidiBytes 解析内存中的 SMF 数据 (便于测试)。
func ParseMidiBytes(data []byte) ([]core.Event, error) {
	return parse(data, DefaultMinDur)
}

// ---- 底层游标 ----

type reader struct {
	data []byte
	pos  int
}

func (r *reader) u8() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, errors.New("MIDI 文件损坏: 意外截断")
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

func (r *reader) u16be() (uint16, error) {
	hi, err := r.u8()
	if err != nil {
		return 0, err
	}
	lo, err := r.u8()
	if err != nil {
		return 0, err
	}
	return uint16(hi)<<8 | uint16(lo), nil
}

func (r *reader) u32be() (uint32, error) {
	hi, err := r.u16be()
	if err != nil {
		return 0, err
	}
	lo, err := r.u16be()
	if err != nil {
		return 0, err
	}
	return uint32(hi)<<16 | uint32(lo), nil
}

// varlen 读取 SMF 变长数量字段 (每字节低 7 位, 最高位为延续标志)。
func (r *reader) varlen() (int, error) {
	n := 0
	for i := 0; i < 4; i++ {
		b, err := r.u8()
		if err != nil {
			return 0, err
		}
		n = n<<7 | int(b&0x7F)
		if b&0x80 == 0 {
			return n, nil
		}
	}
	return n, nil
}

// dataByte 读取一个 MIDI 数据字节。规范要求数据字节 <0x80,
// 违反即文件损坏 —— 若不校验, 畸形字节会混进音色/力度等字段产生非法事件
// (fuzz 实测: vel=131 即源于此)。
func (r *reader) dataByte() (byte, error) {
	b, err := r.u8()
	if err != nil {
		return 0, err
	}
	if b >= 0x80 {
		return 0, fmt.Errorf("MIDI 文件损坏: 数据字节 %#04x 带状态位 (偏移 %d)", b, r.pos-1)
	}
	return b, nil
}

// ---- 主解析 ----

type noteKey struct{ track, ch, note int }

type openNote struct {
	startTick int
	vel       int
}

func parse(data []byte, minDur float64) ([]core.Event, error) {
	r := &reader{data: data}
	if magic, err := r.u16be(); err != nil {
		return nil, errNotMidi
	} else if magic != 0x4D54 /* "MT" */ {
		return nil, errNotMidi
	}
	if s, err := r.u16be(); err != nil {
		return nil, err
	} else if s != 0x6864 /* "hd" (注意小写 h=0x68, 前身同) */ {
		return nil, errNotMidi
	}
	if _, err := r.u32be(); err != nil { // header 长度, 固定 6, 不校验以容忍怪文件
		return nil, err
	}
	if _, err := r.u16be(); err != nil { // 格式号 0/1/2: 共用同一套轨遍历 (格式 2 的多序列语义不支持)
		return nil, err
	}
	ntrk, err := r.u16be()
	if err != nil {
		return nil, err
	}
	division, err := r.u16be()
	if err != nil {
		return nil, err
	}
	if division == 0 {
		return nil, errors.New("MIDI 文件损坏: time division 为 0")
	}
	smpte := division&0x8000 != 0

	type rawNote struct {
		start, end, note, vel, ch int
	}
	var raw []rawNote
	openNotes := make(map[noteKey]openNote) // 重复 on 按 retrigger 截断 (前身已验证设计)
	tempos := make(map[int]uint32)          // 绝对 tick -> us/四分音符 (set tempo 元事件)
	var programs []core.Event               // Program Change (tick→秒在第二遍换算)
	lastTick := 0

	for ti := 0; ti < int(ntrk); ti++ {
		if err := expect(r, 0x4D54, fmt.Sprintf("第 %d 轨缺少 MTrk 标记", ti+1)); err != nil {
			return nil, err
		}
		if err := expect(r, 0x726B, fmt.Sprintf("第 %d 轨缺少 MTrk 标记", ti+1)); err != nil {
			return nil, err
		}
		length, err := r.u32be()
		if err != nil {
			return nil, err
		}
		if r.pos+int(length) > len(r.data) {
			return nil, errors.New("MIDI 文件损坏: 轨数据越界")
		}
		end := r.pos + int(length)
		tick := 0
		var running byte // running status: meta/sysex 不更新 (SMF 规范, 前身同)
		for r.pos < end {
			dt, err := r.varlen()
			if err != nil {
				return nil, err
			}
			tick += dt
			status, err := r.u8()
			if err != nil {
				return nil, err
			}
			if status&0x80 == 0 {
				// 数据字节顶替了状态字节 → running status 接管
				if running == 0 {
					return nil, fmt.Errorf("MIDI running status 异常 (偏移 %d)", r.pos-1)
				}
				r.pos-- // 回退数据字节, 按 running status 统一处理
				status = running
			} else if status < 0xF0 {
				running = status
			}

			switch {
			case status == 0xFF: // meta 事件 (不受 running status 干扰)
				meta, err := r.u8()
				if err != nil {
					return nil, err
				}
				ln, err := r.varlen() // meta 长度是变长字段, 不能只读 1 字节 (前身踩坑记录)
				if err != nil {
					return nil, err
				}
				if meta == 0x51 && ln >= 3 { // set tempo: 3 字节 us/四分音符
					payload, err := take(r, 3)
					if err != nil {
						return nil, err
					}
					tempos[tick] = uint32(payload[0])<<16 | uint32(payload[1])<<8 | uint32(payload[2])
					ln -= 3
				}
				if r.pos+ln > len(r.data) {
					return nil, errors.New("MIDI 文件损坏: meta 越界")
				}
				r.pos += ln

			case status == 0xF0 || status == 0xF7: // sysex: 跳过变长长度字段及其数据
				ln, err := r.varlen()
				if err != nil {
					return nil, err
				}
				if r.pos+ln > len(r.data) {
					return nil, errors.New("MIDI 文件损坏: sysex 越界")
				}
				r.pos += ln

			default:
				typ := status & 0xF0
				ch := int(status & 0x0F)
				switch typ {
				case 0x90, 0x80:
					note, err := r.dataByte()
					if err != nil {
						return nil, err
					}
					vel, err := r.dataByte()
					if err != nil {
						return nil, err
					}
					key := noteKey{ti, ch, int(note)}
					if typ == 0x90 && vel > 0 {
						if prev, ok := openNotes[key]; ok {
							// 未配对 off 的重复 on (非法但常见): 按 retrigger 截断前一个, 不丢音
							raw = append(raw, rawNote{prev.startTick, tick, int(note), prev.vel, ch})
						}
						openNotes[key] = openNote{tick, int(vel)}
					} else if prev, ok := openNotes[key]; ok {
						delete(openNotes, key)
						raw = append(raw, rawNote{prev.startTick, tick, int(note), prev.vel, ch})
					}
				case 0xC0:
					prog, err := r.dataByte()
					if err != nil {
						return nil, err
					}
					programs = append(programs, core.Event{
						T: float64(tick), Kind: core.KindProgram, Ch: ch, Prog: int(prog),
					})
				case 0xA0, 0xB0, 0xE0: // poly AT / CC / pitch bend: 2 数据字节
					if _, err := r.dataByte(); err != nil {
						return nil, err
					}
					if _, err := r.dataByte(); err != nil {
						return nil, err
					}
				case 0xD0: // channel AT: 1 数据字节
					if _, err := r.dataByte(); err != nil {
						return nil, err
					}
				default:
					return nil, fmt.Errorf("MIDI 未知状态字节 %#04x (偏移 %d)", status, r.pos-1)
				}
			}
		}
		if tick > lastTick {
			lastTick = tick
		}
		r.pos = end
	}

	// ---- tempo map: tick → 秒 (分段累加; 禁止「当前 tempo × 绝对 tick」, 前身旧 bug) ----
	tick2sec := func(tick int) float64 {
		if smpte {
			fps := 256 - int(division>>8) // 高字节是负的每秒帧数 (二补码)
			rate := fps * int(division&0xFF)
			return float64(tick) / float64(rate)
		}
		type seg struct {
			tick    int
			baseSec float64
			us      uint32
		}
		var segs []seg
		prevTick, us, acc := 0, uint32(500000), 0.0 // 默认 120bpm 起算
		ticks := make([]int, 0, len(tempos))
		for t := range tempos {
			ticks = append(ticks, t)
		}
		sort.Ints(ticks)
		for _, tk := range ticks {
			acc += float64(tk-prevTick) * float64(us) / 1e6 / float64(division)
			segs = append(segs, seg{tk, acc, tempos[tk]})
			prevTick, us = tk, tempos[tk]
		}
		lo := sort.Search(len(segs), func(i int) bool { return segs[i].tick > tick })
		if lo == 0 {
			return float64(tick) * 500000 / 1e6 / float64(division)
		}
		s := segs[lo-1]
		return s.baseSec + float64(tick-s.tick)*float64(s.us)/1e6/float64(division)
	}

	// ---- 第二遍: 换算秒并生成事件 ----
	var events []core.Event
	for _, n := range raw {
		t0, t1 := tick2sec(n.start), tick2sec(n.end)
		if t1-t0 < minDur {
			continue
		}
		events = append(events,
			core.Event{T: t0, Kind: core.KindNoteOn, Ch: n.ch, Note: n.note, Vel: n.vel},
			core.Event{T: t1, Kind: core.KindNoteOff, Ch: n.ch, Note: n.note},
		)
	}
	// 没关掉的音在曲末关闭
	for k, n := range openNotes {
		t0 := tick2sec(n.startTick)
		t1 := tick2sec(lastTick)
		if t0+1.0 > t1 {
			t1 = t0 + 1.0
		}
		if t1-t0 < minDur {
			continue
		}
		events = append(events,
			core.Event{T: t0, Kind: core.KindNoteOn, Ch: k.ch, Note: k.note, Vel: n.vel},
			core.Event{T: t1, Kind: core.KindNoteOff, Ch: k.ch, Note: k.note},
		)
	}
	for _, p := range programs {
		p.T = tick2sec(int(p.T)) // T 暂存了 tick
		events = append(events, p)
	}

	// 稳定排序 + 同刻优先级: NoteOff 必须先于同刻 NoteOn (否则调度器的重触
	// 逻辑会把刚起音的音误关), Program/Control 先于 NoteOn (音色先就位)。
	prio := func(k core.Kind) int {
		switch k {
		case core.KindNoteOff:
			return 0
		case core.KindProgram:
			return 1
		case core.KindControl:
			return 2
		case core.KindTempo:
			return 3
		case core.KindNoteOn:
			return 4
		}
		return 5
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].T != events[j].T {
			return events[i].T < events[j].T
		}
		return prio(events[i].Kind) < prio(events[j].Kind)
	})
	return events, nil
}

func expect(r *reader, magic uint16, msg string) error {
	got, err := r.u16be()
	if err != nil {
		return err
	}
	if got != magic {
		return fmt.Errorf("MIDI 文件损坏: %s", msg)
	}
	return nil
}

func take(r *reader, n int) ([]byte, error) {
	if r.pos+n > len(r.data) {
		return nil, errors.New("MIDI 文件损坏: 意外截断")
	}
	out := r.data[r.pos : r.pos+n]
	r.pos += n
	return out, nil
}
