package score

import (
	"fmt"
	"os"
	"strings"

	"github.com/ljlsasuke/pianostage/internal/core"
)

// 文本谱面默认参数 (沿用前身 auto-piano 的验证值)。
const (
	DefaultVelocity = 100
	DefaultBeat     = 0.55 // 每拍秒数 (约 109 BPM)
)

// ---- 音名 ↔ MIDI (移植前身 mapping.py, 音域放宽到 GM 全范围) ----

var pitchClass = map[byte]int{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}

var pcNames = [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// NoteNum 音名 → MIDI 编号: "C4"→60, "A0"→21, "C#4"="Db4"→61。
// GM 音源支持 0~127, 无前身 FreePiano 的 A0~C8 限制。
func NoteNum(name string) (int, error) {
	s := strings.TrimSpace(name)
	if len(s) < 2 {
		return 0, fmt.Errorf("非法音名: %q (如 C4 / C#4 / Db4)", name)
	}
	i := 0
	acc := 0
	// 前缀式升降号 (#F4 / bB4, 前身兼容)
	if s[i] == '#' || s[i] == 'b' {
		if s[i] == '#' {
			acc++
		} else {
			acc--
		}
		i++
	}
	base := s[i] &^ 0x20 // 小写化
	pc, ok := pitchClass[base]
	if !ok {
		return 0, fmt.Errorf("非法音名: %q (支持 C D E F G A B)", name)
	}
	i++
	// 后缀式升降号 (F#4 / Bb4) — 前身正则 [#b]? 至多一个, 多个即非法
	if i < len(s) && (s[i] == '#' || s[i] == 'b') {
		if s[i] == '#' {
			acc++
		} else {
			acc--
		}
		i++
		if i < len(s) && (s[i] == '#' || s[i] == 'b') {
			return 0, fmt.Errorf("非法音名: %q (升降号至多一个)", name)
		}
	}
	// 八度号 (可为负)
	neg := false
	if i >= len(s) {
		return 0, fmt.Errorf("非法音名: %q (缺八度号)", name)
	}
	if s[i] == '-' {
		neg = true
		i++
		if i >= len(s) {
			return 0, fmt.Errorf("非法音名: %q", name)
		}
	}
	octave := 0
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("非法音名: %q", name)
		}
		octave = octave*10 + int(s[i]-'0')
	}
	if neg {
		octave = -octave
	}
	n := (octave+1)*12 + pc + acc
	if n < 0 || n > 127 {
		return 0, fmt.Errorf("音名 %s 超出 GM 音域 (0~127): %d", name, n)
	}
	return n, nil
}

// NoteName MIDI 编号 → 音名 ("C#4" 形式)。
func NoteName(n int) string {
	return fmt.Sprintf("%s%d", pcNames[n%12], n/12-1)
}

// ---- 格式声明行 ----

// SplitFormatDecl 剥离 "format: jianpu/note_seq" 声明行。
// 无论调用方是否已显式指定格式, 声明行都不会污染谱面 (前身设计)。
func SplitFormatDecl(text string) (body, declared string) {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		ls := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(ls, "format:") {
			val := strings.TrimSpace(strings.SplitN(ls, ":", 2)[1])
			switch val {
			case "jianpu", "note_seq":
				declared = val
			}
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), declared
}

func stripComments(text string) string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// tokens 归一化分隔: 去注释、去小节线、按空白切分。
func tokens(text string) []string {
	return strings.Fields(strings.ReplaceAll(stripComments(text), "|", " "))
}

// ---- 1. 音名序列 ----

// ParseNoteSeq 解析音名序列: token = 音名:拍数, 和弦用 + 连接, 休止用 R。
// 例: "C4:1 D4:1 E4:2 | 和弦 C4+E4+G4:2 | R:1"
func ParseNoteSeq(text string, beat, velocity float64) ([]core.Event, error) {
	var events []core.Event
	t := 0.0
	for _, tok := range tokens(text) {
		idx := strings.LastIndex(tok, ":")
		if idx < 0 {
			return nil, fmt.Errorf("音名序列格式错误: %q (应为 音名:拍数 如 C4:1)", tok)
		}
		notesPart, durPart := tok[:idx], tok[idx+1:]
		var dur float64
		if _, err := fmt.Sscanf(durPart, "%g", &dur); err != nil {
			return nil, fmt.Errorf("音名序列拍数非法: %q", tok)
		}
		dur *= beat
		if strings.EqualFold(notesPart, "R") {
			t += dur
			continue
		}
		for _, name := range strings.Split(notesPart, "+") {
			n, err := NoteNum(name)
			if err != nil {
				return nil, err
			}
			events = append(events,
				core.Event{T: t, Kind: core.KindNoteOn, Ch: 0, Note: n, Vel: int(velocity)},
				core.Event{T: t + dur, Kind: core.KindNoteOff, Ch: 0, Note: n},
			)
		}
		t += dur
	}
	return events, nil
}

// ---- 2. 简谱 ----

var degreeSemis = map[byte]int{'1': 0, '2': 2, '3': 4, '4': 5, '5': 7, '6': 9, '7': 11}

// KeyPC 调名 → "1" 对应的音级: "C"/"c"/"F#"/"Bb" 均可 (移植前身)。
func KeyPC(key string) (int, error) {
	k := strings.TrimSpace(key)
	if k == "" {
		return 0, fmt.Errorf("调名为空, 应为 C/D/E/F/G/A/B (可带 # 或 b)")
	}
	base := k[0] &^ 0x20
	pc, ok := pitchClass[base]
	if !ok {
		return 0, fmt.Errorf("非法调名: %q (支持 C D E F G A B, 可带 # 或 b, 如 F# / Bb)", key)
	}
	for _, c := range k[1:] {
		if c != '#' && c != 'b' {
			return 0, fmt.Errorf("非法调名: %q (调名只能带 # 或 b)", key)
		}
		if c == '#' {
			pc++
		} else {
			pc--
		}
	}
	return ((pc % 12) + 12) % 12, nil
}

// floorDiv 向下取整除 (Go 整除是向零截断, 简谱 b1 需要落到 B3 而非 B4,
// 前身用的是 Python 的地板除 —— 移植时必须显式实现)。
func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// ParseJianpu 解析简谱 (语法沿用前身):
//   - 1~7 为音, 0 为休止
//   - 后缀 '-' 每个延长一拍 (5- = 两拍)
//   - 后缀 '^' 升八度, 'v' 降八度 (可叠加)
//   - 前缀 '#'/'b' 升/降半音 (例: #4 b7)
//   - 注释行以 // 开头, '|' 小节线忽略, 空白分隔音符
func ParseJianpu(text, key string, beat, velocity float64) ([]core.Event, error) {
	kpc, err := KeyPC(key)
	if err != nil {
		return nil, err
	}
	var events []core.Event
	t := 0.0
	for _, tok := range tokens(text) {
		m := tok
		acc := 0
		if m[0] == '#' || m[0] == 'b' {
			if m[0] == '#' {
				acc = 1
			} else {
				acc = -1
			}
			m = m[1:]
		}
		if m == "" {
			return nil, fmt.Errorf("简谱 token 无法解析: %q", tok)
		}
		degree := m[0]
		if _, ok := degreeSemis[degree]; !ok && degree != '0' {
			return nil, fmt.Errorf("简谱 token 无法解析: %q", tok)
		}
		octUp, octDn, ext := 0, 0, 0
		for _, c := range m[1:] {
			switch c {
			case '^':
				octUp++
			case 'v':
				octDn++
			case '-':
				ext++
			default:
				return nil, fmt.Errorf("简谱 token 无法解析: %q", tok)
			}
		}
		dur := float64(1+ext) * beat
		if degree != '0' {
			semi := degreeSemis[degree] + acc
			octave := 4 + octUp - octDn + floorDiv(semi, 12) // b1 → B3 而非 B4
			n := (octave+1)*12 + ((kpc + semi) % 12 + 12) % 12
			if n < 0 || n > 127 {
				return nil, fmt.Errorf("简谱音越出 GM 音域: %q → %d", tok, n)
			}
			events = append(events,
				core.Event{T: t, Kind: core.KindNoteOn, Ch: 0, Note: n, Vel: int(velocity)},
				core.Event{T: t + dur, Kind: core.KindNoteOff, Ch: 0, Note: n},
			)
		}
		t += dur
	}
	return events, nil
}

// Load 统一入口: 按扩展名/声明行分发解析 (MIDI / 音名序列 / 简谱)。
func Load(source string, key string, beat, velocity float64) ([]core.Event, error) {
	lower := strings.ToLower(source)
	if strings.HasSuffix(lower, ".mid") || strings.HasSuffix(lower, ".midi") {
		return ParseMidi(source)
	}
	body, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}
	text, declared := SplitFormatDecl(string(body))
	switch declared {
	case "jianpu":
		return ParseJianpu(text, key, beat, velocity)
	case "note_seq":
		return ParseNoteSeq(text, beat, velocity)
	}
	// 无声明行: 探测 (含 ':' 且以音名开头 → note_seq; 否则按简谱)
	probe := tokens(text)
	if len(probe) > 0 && strings.Contains(probe[0], ":") {
		return ParseNoteSeq(text, beat, velocity)
	}
	return ParseJianpu(text, key, beat, velocity)
}
