# AP/1 协议规范（PianoStage 自动化接口）

> 状态：M0 定稿草案。实现于 `internal/protocol`，版本协商经 `hello` 帧。
> 目标读者：skill 作者 / 第三方 AI 集成。设计动机见 [PLAN.md](../PLAN.md) §5.3。

## 1. 总览

- **传输**：JSON Lines，每行一个 JSON 对象，UTF-8。主通道为 **stdio**（spawn `pianostage serve` 后读写其 stdin/stdout）；未来可扩展 TCP localhost。
- **方向**：客户端（AI/skill）→ 程序为 **Command**；程序 → 客户端为 **Message**（回执或事件）。
- **接入模式**：
  - `serve --headless`：只有后端，只有声音（AI 只接后端）；
  - `serve`：前端 + 后端，音乐 + 动画（AI 同时接入两级）；
  - `play <文件>`：单命令直通，**完全绕过协议**，skill 最常用路径。

## 2. 版本协商

进程启动后第一帧必为：

```json
{"type":"hello","ap":1}
```

客户端若不支持 `ap` 版本，应立即退出进程。

## 3. Command（客户端 → 程序）

| op | 语义 | 关键字段 |
|---|---|---|
| `ping` | 存活探测 | — |
| `load` | 加载谱面（文件路径、曲库曲名；.mid/.midi/.txt，文本格式看声明行） | `source` |
| `list` | 列出用户曲库曲名 | — |
| `play` | 开始播放 | — |
| `pause` / `stop` | 暂停 / 停止（停止含 flush） | — |
| `seek` | 跳转到曲内时间 | `position`（秒） |
| `tempo` | 播放速率倍率（1.0 正常，0.5 半速；加载时重锚时钟） | `tempo` |
| `note` | 单音（现场/即时事件，走总线不进调度队列） | `on` `ch` `note` `vel` |
| `program` | 换音色 | `ch` `prog` |
| `cc` | 控制器（64=延音踏板） | `ch` `ctrl` `ctrlval` |
| `backend` | 选择后端/设备 | `backend` `device`（"auto"/序号/名称） |
| `query` | 查询 | `query`: `devices` / `status` / `time` |
| `quit` | 优雅退出（回执 ok 后进程结束） | — |

**后端惰性打开**：serve 启动不强制要求 MIDI 设备（协议流程与发声解耦）；
发声类操作（play 的调度分发 / note / program / cc / backend）在设备缺失时
返回 `E_DEVICE`，其余操作不受影响。

字段类型与取值范围见 `internal/protocol/protocol.go` 上的注释（`ch`: 0~15，`note`/`vel`/`prog`: 0~127）。

## 4. Message（程序 → 客户端）

| type | 语义 | 关键字段 |
|---|---|---|
| `hello` | 就绪帧，版本协商 | `ap` |
| `ok` | 命令成功 | `op`，`data`（query 结果 / load 的时长与音符数） |
| `err` | 命令失败 | `op` `code` `detail` |
| `state` | 状态变化广播 | `data`（playing/paused/backend/device…） |
| `time` | 主时钟广播（曲内秒） | `t`（约 20Hz） |
| `note` | 事件回显（供外部前端/可视化挂载） | `data`（ch/note/vel/kind），`t` |

每条命令恰好回执一次（`ok` 或 `err`）。客户端关闭 stdin 同样会触发进程
优雅退出（等价于 quit，但不回执）。

## 5.1 曲库

- 用户库目录：`%APPDATA%/pianostage/songs`（包外，程序更新不影响）
- `load` 的 `source` 支持裸曲名（可省略扩展名 .mid/.midi/.txt）或完整路径
- `list` 返回库内全部曲名

## 5. 错误码

| code | 含义 |
|---|---|
| `E_PROTO` | 非法 JSON / 字段形态错误 |
| `E_OP` | 未知 op |
| `E_RANGE` | 字段越界 |
| `E_LOAD` | 谱面加载失败（不存在/解析失败） |
| `E_BACKEND` | 后端不存在或未支持 |
| `E_DEVICE` | 设备不存在/打开失败 |
| `E_STATE` | 状态机非法迁移（如未 load 就 play） |
| `E_INTERNAL` | 内部错误 |

## 6. 时钟模型（重要）

- `midiOutShortMsg` 即发即忘，合成器不回报播放位置 → **没有可读的"音频时钟"**。
- 主时钟 = 程序内调度器的单调时钟 + 曲内时间轴；`time` 帧的 `t` 即其当前值。
- 事件以**前瞻调度**提前 ~50ms 发出（补偿 winmm 5~20ms 投递延迟）。
- 消费方（前端/可视化）应以最近收到的 `t` 为锚点、按墙钟插值渲染，不做绝对对齐。

## 7. 并发与现场演奏

- 现场演奏（人类键盘 / MIDI 键盘）与 AI 即时 `note` 命令产生的事件**不进调度队列**，直接进总线（T=0 立即生效）。
- 因此 AI 定时播放与人类即时演奏可并存（四手联弹），前端对二者一视同仁地渲染动画。
