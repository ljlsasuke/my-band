# My Band

可被 AI 驱动、也可被人类演奏的琴曲舞台：前端负责"演"（键盘动画），后端负责"响"
（可插拔声源），事件总线连接两者。

> 立项背景、架构设计与里程碑见 [PLAN.md](PLAN.md)。
> 前身项目: [ljlsasuke/auto-piano](https://github.com/ljlsasuke/auto-piano)。

## 架构

七个模块，一条总线：三类事件生产者 → core 统一调度 → 两路消费者。

```
┌─────────────── 事件生产者（三路输入）───────────────┐
│                                                      │
│  ① internal/score      谱面解析端                     │
│     MIDI 文件 / 简谱 / 音名序列 → core.Event 流       │
│     (tick→秒 tempo map, 音色/力度/声道保留)            │
│                                                      │
│  ② internal/input      键盘适配端                     │
│     ComputerKeys: 电脑键盘 (FreePiano 式全白键四排,    │
│                   F3/F4 调号转调, F5/F6 八度)          │
│     MidiIn:       MIDI 键盘/电钢琴 (真实力度)          │
│                                                      │
│  ③ internal/protocol   通信端 (AP/1, JSONL over stdio)│
│     AI 远程驱动: load/play/note/query/quit            │
└──────────────────────┬───────────────────────────────┘
                       ↓ 全部归一化成 core.Event（统一货币）
┌──────────────────────────────────────────────────────┐
│  ④ internal/core    核心：事件模型 + 主时钟调度器       │
│     Scheduler: 前瞻 50ms 分发 / 复音预算防削波         │
│     / 暂停·跳转·变速 / 挤音策略                        │
└──────────────────────┬───────────────────────────────┘
                 ┌─────┴──────┐
                 ↓            ↓
┌──────────────────┐  ┌──────────────────────────┐
│ ⑤ internal/backend│  │ ⑥ internal/frontend      │
│    发声端          │  │    动画端 (Ebiten)        │
│                   │  │                          │
│  winmm.midiOut    │  │  88 键钢琴 + 下落音符     │
│  → 实时 MIDI 消息  │  │  + 键帽面板(FreePiano 式  │
│  → 系统合成器发声  │  │    简谱数字, 物理键高亮)   │
│  (GS Wavetable /  │  │  + 设备状态条             │
│   VirtualMIDISynth)│  │                          │
└──────────────────┘  └──────────────────────────┘
```

辅助模块：`internal/library`（用户曲库）、`internal/cli`（命令组装：
play / serve / doctor / dump / test-note，main.go 仅 3 行）。

**关键设计**：

- **三路生产者平级**：谱面回放、人类按键、AI 即时弹奏产出同一种
  `core.Event`，互不指挥——因此"四手联弹"（人 + AI 同时演奏）天然成立。
- **core 不懂 MIDI，backend 不懂时机**：调度器决定"什么时候发"，
  cli 接线层做"Event → MIDI 字节"的唯一翻译，换声源零改动。
- **MIDI 文件 ≠ 实时消息**：文件存配对音符 + tempo 元信息（数据蓝图），
  实时流只有"现在按下/松开"（即时指令）；解析器和调度器负责两者间的翻译。

## 使用

两个产物（python.exe/pythonw.exe 模式）：`my-band-cli.exe` 控制台版（终端 & AI 用），
`my-band.exe` GUI 版（双击用，无黑框）。

```bash
# 打开就是琴 (双击 build\my-band.exe, 或终端跑控制台版)
build\my-band-cli.exe

# 看AI弹曲子 (默认带动画窗口, 播完窗口保留转自由弹奏; --headless 无头播放)
build\my-band-cli.exe play merry_go_round_of_life

# AI 协议驱动 (AP/1, JSONL over stdio, 见 docs/AP1.md; --headless 纯后端无窗口)
echo {"op":"play"} | build\my-band-cli.exe serve --headless

# 环境自检 / 解析调试
build\my-band-cli.exe doctor
build\my-band-cli.exe dump 曲名
```

键盘操作：四排全白键映射（数字行/Q/A/Z），`F3`/`F4` 调号转调，
`F5`/`F6` 八度，`Tab` 键位面板开关。详见 docs/AP1.md 与 PLAN.md。

## 状态

- ✅ M0 协议与骨架
- ✅ M1 先有声 (headless MIDI 播放，命运交响曲分声部验收)
- ✅ M2 前端 MVP (Ebiten 88 键 + 按下动画 + 音画同步)
- ✅ M3 协议完整化 (serve + 曲库/简谱解析 + 黑盒集成测试)
- ✅ M4 人类演奏 + 下落音符 (四手联弹成立)
- **M4.5 键盘适配模块 (FreePiano 式键位 + MidiIn + 键帽面板) ← 当前**
- M5 应用壳 (侧栏 + 演奏模式矩阵 + 混音台)
- M6 发布流水线 (GoReleaser + skill 一键安装) → v1.0.0

## 构建

```
go build ./cmd/my-band
go test ./...
scripts/release.sh           # 发版 (bump 版本 + tag + 构建双产物)
```

双产物：`my-band-cli.exe` 控制台子系统（终端/AI 主入口），
`my-band.exe` GUI 子系统（`-H windowsgui`，双击无黑框；
出错走 MessageBox，见 internal/cli/console_windows.go）。

## 许可证

MIT
