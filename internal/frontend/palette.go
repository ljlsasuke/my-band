package frontend

// 乐器族 → 颜色 (PLAN.md §8-7 记录的设计决策, 2025-11-25):
//   - 族划分: GM Level 1 硬标准 (16 族 + 声道 10 打击乐)
//   - 颜色: 无官方标准, 此处为设计决定 —— 16 族坍缩为 5+1 视觉组
//     (管弦乐常用编制恰好五类 + "其他"兜底), 联觉约定 (铜管→金, 打击→红)
//   - 工程约束: 色板收敛为本文件单一常量表 + FamilyOf 单一映射函数,
//     改配色/分组只动这里; 可配置化 (--palette) 留待需要时暴露

// Family 视觉乐器组。
type Family int

const (
	FamKeys     Family = iota // 键盘类: 钢琴/钟琴/风琴 → 琥珀
	FamPerc                   // 打击乐 (含声道 9) → 红
	FamStrings                // 贝斯/弦乐/弦乐合奏 → 蓝
	FamWoodwind               // 簧片/长笛类 → 绿
	FamBrass                  // 铜管 → 金
	FamOther                  // 吉他/民族/合成器/音效 → 紫
)

// RGB 三原色 (0~255)。
type RGB struct{ R, G, B uint8 }

// palette 色板常量表 (改配色只动这里)。
var palette = map[Family]RGB{
	FamKeys:     {0xE8, 0xA3, 0x3D}, // 琥珀
	FamPerc:     {0xE8, 0x4E, 0x4E}, // 红
	FamStrings:  {0x4E, 0x9A, 0xE8}, // 蓝
	FamWoodwind: {0x4E, 0xC9, 0x6B}, // 绿
	FamBrass:    {0xF2, 0xD0, 0x4A}, // 金
	FamOther:    {0xA6, 0x7E, 0xD9}, // 紫
}

// Color 返回乐器组的色板色。
func (f Family) Color() RGB { return palette[f] }

// FamilyOf 声道 + 当前音色编号 → 视觉乐器组。
// 崩塌规则 (16 GM 族 → 5+1):
//   声道 9 与 112~119 (Percussive)     → 打击
//   0~7 (Piano) 8~15 (钟琴类) 16~23 (Organ) → 键盘
//   32~55 (Bass/Strings/Ensemble)      → 弦乐 (贝斯属弦乐声部)
//   56~63 (Brass)                      → 铜管
//   64~79 (Reed/Pipe)                  → 木管
//   其余 (吉他/合成器/民族/音效)        → 其他
func FamilyOf(ch, prog int) Family {
	if ch == 9 {
		return FamPerc
	}
	switch {
	case prog == 47:
		return FamPerc // 定音鼓: 官方归 Strings 族, 但音乐语义是打击乐 (命运交响曲 Timpani 轨实测)
	case prog < 8:
		return FamKeys
	case prog < 16:
		return FamKeys // 钟琴类: 键盘演奏方式, 归键盘色
	case prog < 24:
		return FamKeys // 风琴
	case prog < 32:
		return FamOther // 吉他
	case prog < 56:
		return FamStrings // 贝斯 + 弦乐 + 合奏
	case prog < 64:
		return FamBrass
	case prog < 80:
		return FamWoodwind
	case prog < 112:
		return FamOther // 合成器 + 民族
	case prog < 120:
		return FamPerc // Percussive
	default:
		return FamOther // Sound Effects
	}
}
