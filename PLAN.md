# PianoStage 项目计划（草案 v0.1）

> 新项目立项计划。前身项目 `auto-piano`（FreePiano 键盘点击模拟方案）的完整经验教训已吸收进本文档。
> 本文档的目标读者：接手实施的 AI/开发者，要求读完即可开工。

---

## 1. 背景与动机

### 1.1 前身项目

前身项目 **auto-piano**（https://github.com/ljlsasuke/auto-piano），
仓库内含完整的踩坑记录与已验证的解析器实现，是本项目全部经验教训的来源。

它通过驱动第三方软件 FreePiano（虚拟键盘钢琴）实现自动演奏：
解析谱面 → 换算屏幕坐标 → 向 FreePiano 窗口注入鼠标点击"按琴键"。

实战暴露的痛点（全部为真实踩坑记录）：

| 痛点 | 具体表现 |
|---|---|
| 不拥有被控软件 | 启动超时、安装路径探测、进程/窗口/DPI 管理、键盘钩子过滤注入输入（只能用鼠标点击模拟）等一整套 workaround |
| 音色天花板 | FreePiano 单一钢琴音色。管弦乐 MIDI（命运交响曲 14 轨、旋转木马含弦乐/打击乐）被硬转成钢琴，编制感全失 |
| 削波失真 | 32 个强奏音同时响时总电平超 0dBFS，产生"滋滋"噪声（已用力度缩放缓解，但治标） |
| 力度粗糙 | 键盘钩子过滤注入按键，只能用鼠标点击，力度靠点击纵向位置模拟（1~127 只有几何分辨率） |
| 时序精度 | Python sleep 调度 ±10ms；且点击吞吐有上限 |

### 1.2 已验证的技术事实（前身项目实测结论）

1. **winmm MIDI 直连可行且零依赖**：纯 ctypes/syscall 调 `midiOutOpen/midiOutShortMsg` 即可驱动 Windows 内置 GS 合成器，实测命运交响曲分声部（弦乐/圆号/长笛）播放效果良好。
2. **Windows 自带 `Microsoft GS Wavetable Synth`**：每台 Windows 都有，作为兜底音源零安装成本。
3. **VirtualMIDISynth 是理想升级项**：注册为标准 MIDI Out 设备，挂 SoundFont 后音质显著提升；未安装时程序应自动回落 GS。
4. **削波根因**：多个强奏音叠加超 0dBFS；MIDI 播放器（如 WMP）靠分轨混音规避。
5. **用户真实需求**："说一句话，曲子正确地响起来"；钢琴曲要钢琴声，管弦乐要有管弦乐的编制感；观赏性（看琴键被按下）是加分项。

### 1.3 立项结论

"发声"与"演奏动作"必须解耦。新项目 = 自研演奏前端（可视化）+ 可插拔发声后端 + AI 优先的自动化接口。

---

## 2. 项目定位

> **一句话**：一个可被 AI 驱动、也可被人类演奏的琴曲舞台——前端负责"演"，后端负责"响"，事件总线连接两者。

- 不是 FreePiano 克隆（不做可弹奏乐器的采样引擎/VST 加载等乐器功能）
- 不是通用 MIDI 播放器竞品（不做播放器 UI 全家桶，聚焦演奏可视化）
- 核心场景：AI 助手（如 pi skill）说曲名 → 带动画演奏；人类也能上手弹

---

## 3. 总体架构

```
┌─ 前端 frontend（自研，可运行可缺席）────────────┐
│  键盘渲染 + 按键动画 +（二期）音符下落/分色音轨   │
└──────────────┬─────────────────────────┘
               │ 事件总线（单一事实源）
┌──────────────┴─────────────────────────┐
│  core: 事件模型 / 时钟调度 / 谱面解析 / 曲库  │
└──┬───────────────┬───────────────┬─────┘
   │               │               │
┌─┴────────┐ ┌────┴─────────┐ ┌───┴──────────┐
│ 谱面输入  │ │ 声音后端(可插拔)│ │ 控制接口(AP协议)│
│ MIDI/简谱 │ │ midiout / ... │ │ AI·人类·CLI   │
│ 现场演奏  │ │ winmm→GS/SF… │ │ stdio/socket  │
└──────────┘ └──────────────┘ └──────────────┘
```

**设计原则：一切皆事件流。** AI 自动演奏、人类现场弹奏、文件回放，产生的都是同一种事件，走同一条总线；前端动画与后端发声都是事件的下游消费者。

---

## 4. 决策记录（ADR）

| # | 决策 | 结论 | 理由 | 备选（未采纳原因） |
|---|---|---|---|---|
| ADR-1 | 语言 | **Go**（✅ 已确认，2025-11-25） 单静态 exe 分发最优（决策 #4）；Ebiten 是演奏可视化主场（决策 #1）；syscall 调 winmm 零依赖（决策 #2） | Python（前身语言，但分发需运行时）；Rust（GUI 生态摩擦大）；TS/Electron（体积大） |
| ADR-2 | GUI | **Ebiten**（github.com/hajimehoshi/ebiten） | 2D 游戏引擎，键盘渲染/动画/输入正是其主场，跨平台 | pygame（打包体验差）；Fyne（非游戏型渲染） |
| ADR-3 | 声音 MVP | **winmm MIDI-out 直连** | 零依赖、每台 Windows 有 GS 兜底、VirtualMIDISynth 自动升级 | 纯 Go SF2 合成（生态弱，二期评估）；cgo fluidsynth（构建链复杂） |
| ADR-4 | 控制协议 | **AP/1：JSON Lines over stdio**（+可选 TCP localhost） | 文本协议 AI 易生成/解析；stdio 天然进程管道；人类弹奏事件走同一协议 | WebSocket（首版过度设计，预留升级） |
| ADR-5 | 平台范围 | **MVP 仅 Windows**（winmm/Ebiten 均已支持），接口层保持跨平台友好 | 目标用户环境明确 | macOS/Linux（二期，换 rtmidi/ALSA） |

---

## 5. 五项核心考量设计

### 5.1 前端：钢琴按下动画与事件

**键盘渲染**
- 88 键布局（A0~C8，MIDI 21~108），白键/黑键正确错位排列，窗口缩放自适应
- 按键状态由事件总线驱动：`NoteOn` → 按下态，`NoteOff` → 释放态

**按下动画（MVP）**
- 按下：键体下沉（白色键 ~4px、黑键 ~3px）+ 亮度/着色变化
- 力度映射：velocity → 下沉深度与高亮强度（如 vel/127 线性插值），强奏按键更明显
- 释放：~180ms 线性回弹衰减，避免"瞬跳"的机械感
- 踏板：`CC64` sustain 时被延音的音在键上显示保持环/微光，视觉上区分"已松键但仍在响"
- 同键重触：重新触发按下动画并重置衰减计时

**二期动画**
- Synthesia 式音符下落：每声道一条道，按 GM 乐器族配色（钢琴=黄、弦乐=蓝、木管=绿、铜管=金、打击=红、其他=紫），音符块落到琴键位置触发按下高亮——这同时回答了"管弦乐前端如何表现"
- 时间基准见 5.3 的时钟同步

**事件模型（core，前后端共享）**
```go
type Event struct {
    T      float64 // 秒, 曲内时间轴; 现场演奏事件 T 为 0(即时)
    Kind   Kind    // NoteOn|NoteOff|Program|Control|Tempo|Marker
    Ch     int     // 0~15, 9=打击乐
    Note   int     // 0~127
    Vel    int     // 0~127
    Prog   int     // Program 变更时
    Ctrl   int; CtrlVal int  // CC 时
}
```

### 5.2 后端：声音（可插拔）

**后端接口（backend 接口即"声源"抽象）**
```go
type Backend interface {
    Name() string
    Devices() []DeviceInfo          // 枚举可选设备
    Open(device string) error
    Program(ch, prog int) error
    NoteOn(ch, note, vel int) error
    NoteOff(ch, note int) error
    Control(ch, ctrl, val int) error   // 踏板/音量等 CC
    AllOff() error                     // 急停
    Close() error
}
```

**MVP 内置实现：`midiout`（winmm syscall）**
- 直接 P/Invoke `midiOutGetDevCapsW / midiOutOpen / midiOutShortMsg / midiOutClose`（参考：前身项目 ctypes 版已验证，约 30 行核心代码）
- **设备选择策略**：`--midi-out <名称|序号|auto>`；`auto` 时枚举设备，名称含 `VirtualMIDISynth` 优先 → 否则 `Microsoft GS Wavetable Synth` → 否则报错并列出可用设备
- VirtualMIDISynth 挂 SoundFont 后音质自动升级，本项目零额外代码——这是"接入 VirtualMIDISynth"的正确姿势（**不自己实现 SF2 渲染**，把它当外部音源）
- 注意：GS 合成器为 32 复音，超密段落仍可能挤音；文档建议管弦乐用户装 VirtualMIDISynth

**二期可选后端（明确非 MVP）**
- `fluidsynth` 子进程模式：外挂 fluidsynth.exe + SF2 文件，本项目通过其 MIDI 接口驱动（补足无 VirtualMIDISynth 用户的音质升级路径）
- 纯 Go SF2 渲染：生态不成熟，仅评估不承诺

### 5.3 AI 友好：自动化接口（AP/1 协议）

**接入层级（对应"只接后端 / 前端后端都接"）**

| 模式 | 启动方式 | AI 得到 | 场景 |
|---|---|---|---|
| **headless** | `pianostage serve --headless` | 只有声音，无 GUI | 后台听曲（前身项目主场景） |
| **full** | `pianostage serve` | 声音 + 动画窗口 | 演示/观赏 |
| **play 文件** | `pianostage play x.mid [--headless]` | 一条命令直接播 | skill 最常用路径，不进协议也能用 |

**AP/1 协议要点**
- 传输：stdio（主）。每行一个 JSON 对象；进程退出即会话结束
- AI → 程序（命令）：`load`(文件/曲名) `play` `pause` `stop` `seek` `tempo` `note`(单音 on/off) `program` `cc` `backend`(选后端/设备) `query`(devices/status/time)
- 程序 → AI（回执/事件）：`ok/err`、`state`、`time`（节拍时钟广播，供外部 UI 同步）、`note`（回显，供第三方前端挂载）
- **时钟同步策略（细化）**：`midiOutShortMsg` 是即发即忘的，合成器不回报播放位置，所以**不存在可读的“音频时钟”**——主时钟只能是 core 调度器的单调时钟 + 曲内时间轴。两个衍生设计：
  1. **前瞻调度（look-ahead）**：winmm 实测延迟 5~20ms，调度器对音符事件提前一个窗口（如 50ms）发出，以投递时间补偿延迟，保证“听到的”落在时间轴上
  2. `time` 广播的是调度时钟当前曲内时间，前端/外部消费者以其插值追帧，误差不累积

**人类演奏（与 AI 同总线共存）**
- 计算机键盘映射两排琴键（沿用前身项目的键位思路）、支持八度切换
- 可选接入 MIDI 键盘（winmm midiIn；二期）
- 人类事件与 AI 事件进同一总线 → 前端同样渲染动画，后端同样发声
- **彩蛋场景**：AI 弹伴奏 + 人类弹主旋律的"四手联弹"，架构上天然支持

**skill 集成形态（给 AI 平台的说明书即 SKILL.md）**
- 一句话播放：`pianostage play <曲名|路径> --headless`
- 精细控制：spawn `serve` 进程 + 读写 stdin JSONL
- 谱面保存：用户曲库目录（沿用前身项目"包外用户库"设计，避免更新被清空）

### 5.4 分发友好：AI 一键安装

**产物**：GitHub Releases，GoReleaser 自动出 `windows-amd64.zip`（内含单 exe + LICENSE + SKILL.md 模板）

**skill 侧一键安装流程（写成 skill 安装步骤，AI 可自助执行）**
1. 查询 GitHub API `releases/latest` 取版本与资产 URL
2. 下载 zip → 校验 sha256（随 release 发布 checksums.txt）
3. 解压到 `%LOCALAPPDATA%\PianoStage\`，PATH 或绝对路径记录到 skill 配置
4. `pianostage doctor`：自检（exe 版本 / MIDI 设备枚举 / 试音一个中央 C）
5. 全程 ~10MB 下载、无运行时依赖、无管理员权限

**版本协商**：`pianostage version --json`；协议版本 `ap: "1"` 写在 hello 帧里，skill 据此兼容新旧 exe。

### 5.5 生态友好：Go 轮子清单

| 需求 | 轮子 | 备注 |
|---|---|---|
| GUI/渲染/输入 | **Ebiten** v2 | 成熟活跃，文档全 |
| MIDI-out | **winmm syscall**（自写，~50 行） | 不引第三方，前身已验证 |
| MIDI 文件解析 | `gomidi/midi` 或自写 | SMF 0/1 解析不复杂，前身有 Python 参考实现可直译 |
| PCM 音频（仅二期 fluidsynth 路线可能用） | oto | Ebiten 自带音频引擎亦可 |
| 发布 | GoReleaser + GitHub Actions | 单 exe 产物流水线 |
| 已知短板 | 纯 Go SF2 合成生态弱 | 规避见 ADR-3/5.2，不影响 MVP |

---

## 6. 里程碑

| 里程碑 | 交付物 | 验收标准 |
|---|---|---|
| **M0 协议与骨架** | ① AP/1 协议规范（命令集 JSON schema、错误码、时钟模型）② 仓库骨架 `cmd/pianostage` + `internal/{core,backend,frontend,protocol}` ③ 调度器设计定稿（主时钟/前瞻窗口/voice 预算开关）④ 测试策略（解析器表驱动测试直译前身用例；协议一致性测试） | 协议文档定稿，CI 绿 |
| **M1 无声之舞台之前：先有声** | headless 播放：`play x.mid` → winmm 出声 | 命运交响曲播放，分声部音色正确，无削波杂音，VirtualMIDISynth 自动识别 |
| **M2 前端 MVP** | Ebiten 88 键 + 按下动画 + 同步 | 播放时按键动画与声音肉眼同步；窗口无焦点也能正常演 |
| **M3 协议完整化** | serve 进程 + JSONL 全命令 + 曲库/简谱解析移植 | AI（skill）通过纯协议完成：查曲库→播放→暂停→切曲 |
| **M4 人类演奏 + 下落音符** | 键盘现场弹奏、Synthesia 式分色下落 | 人机四手联弹演示成立 |
| **M5 发布流水线** | GoReleaser + skill 一键安装文档 | 新机器上 skill 全自动装完并弹出第一首曲子 |

每个里程碑结束都是可用状态（M1 即可替代前身项目的全部音频能力）。

---

## 7. 风险与缓解

| 风险 | 概率 | 缓解 |
|---|---|---|
| 音画不同步 | 中 | 音频主时钟 + 前端插值追帧；协议广播 time；M2 专项验收 |
| GS 32 复音在超密段落挤音 | 中 | 文档引导装 VirtualMIDISynth；`--backend` 预留升级位 |
| Ebiten 窗口抢焦点打扰用户 | 低 | 读者模式启动（不抢焦点，仅置顶可选）；headless 模式根本无窗口 |
| 协议设计过度/不足 | 中 | M0 先定最小命令集，play 直通命令保底（AI 可以完全绕过协议） |
| 前身项目曲库迁移 | 低 | 用户库仍是普通目录（.mid/.txt），直接复用 |

---

## 8. 开放问题（立项前需拍板）

1. **项目名**：PianoStage 为占位名，待定（候选：PianoStage / AutoKeys / Ivory…）
2. ~~ADR-1 语言 Go 是否确认~~（✅ 已确认）
3. **简谱/音名谱面解析是否沿用前身语法**（建议沿用：用户已有曲库兼容，文法已验证。如无异议按沿用执行）
4. **MIDI 键盘现场输入**（建议 M4，不在 MVP；winmm midiIn 接口需预留）
5. **前身 auto-piano skill 的去留**（建议：新项目 M1 达标后文档交叉引用，M4 后正式替代）
6. ~~开源许可证~~（✅ MIT，已建 LICENSE）
7. **前端乐器着色方案（M4 前拍板，2025-11-25 讨论记录，暂缓实施）**:
   - 背景: M2 现状是所有乐器都“翻译”成琴键按压（无色差），管弦乐场景丢失
     “谁在演奏”维度；曾提出过渡方案 A（按乐器族给按键着色），决定暂不实施
   - 标准部分: 乐器族划分有硬标准（GM Level 1 的 16 族 + 声道 10 打击乐）
   - 设计部分: 颜色本身无官方/事实标准（Synthesia 等均为按音轨可配置随机配色），
     拟定的五色方案（钢琴=琥珀/弦乐=蓝/木管=绿/铜管=金/打击=红）是设计决定，
     思路: 16 族坍缩为 5 视觉组（管弦乐常用编制恰好五类）+ 联觉约定
     （铜管→金、打击→红冲击感、弦乐→冷色）
   - 工程约束: 色板必须收敛为单一 const 表 + 一处映射函数，后续暴露为
     可配置项（--palette 或配置文件），避免硬编码进渲染层
   - 关联: M4 的 Synthesia 式分色下落音轨是完整方案，方案 A 是其过渡形态；
     实施时需先拍板: 色板值 / 族坍缩粒度 / 是否可配置
   - **2025-11-25 M4 用户反馈（升级为独立设计课题，v1.0 前必须解决）**:
     用户认为“用琴键表达非钢琴乐器”本质上不合理，未来要推翻。候选方向:
     a) 乐器族泳道视图（每族一条独立泳道，脱离键盘映射）
     b) 横向 piano roll 时间轴（但会稀释“钢琴舞台”的项目身份）
     c) 混合: 键盘为锚 + 上方族泳道为补充视图，可切换
     需要专门讨论后再定，色板/下落块等实现保持可复用

---

## 附：前身项目可直接移植的资产

- MIDI SMF 0/1 解析（含 tempo map 两遍法换算、格式 1 控制轨、SMPTE 时基）→ 直译 Go
- 简谱/音名序列文法（含八度/升降号/声明行）→ 直译 Go
- 用户曲库目录约定（包外、同名覆盖优先级）→ 原样沿用
- winmm 设备枚举逻辑 → 直译 syscall
- 全部踩坑记录（本文 1.1）→ 验收标准来源
