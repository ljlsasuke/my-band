# PianoStage

可被 AI 驱动、也可被人类演奏的琴曲舞台：前端负责"演"（键盘动画），后端负责"响"
（可插拔声源），事件总线连接两者。

> 立项背景、架构设计与里程碑见 [PLAN.md](PLAN.md)。
> 前身项目: [ljlsasuke/auto-piano](https://github.com/ljlsasuke/auto-piano)。

## 状态

M0（协议与骨架）进行中。里程碑：

- M0 协议与骨架 ← **当前**
- M1 无声之舞台之前: 先有声（headless MIDI 播放）
- M2 前端 MVP（Ebiten 88 键 + 按下动画）
- M3 协议完整化（serve + JSONL 全命令 + 曲库/简谱解析）
- M4 人类演奏 + 下落音符
- M5 发布流水线

## 构建

```
go build ./cmd/pianostage
go test ./...
```

## 许可证

MIT
