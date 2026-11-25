# PianoStage

可被 AI 驱动、也可被人类演奏的琴曲舞台：前端负责"演"（键盘动画），后端负责"响"
（可插拔声源），事件总线连接两者。

> 立项背景、架构设计与里程碑见 [PLAN.md](PLAN.md)。
> 前身项目: [ljlsasuke/auto-piano](https://github.com/ljlsasuke/auto-piano)。

## 状态

## 状态

- ✅ M0 协议与骨架
- ✅ M1 先有声 (headless MIDI 播放, 验收: 命运交响曲分声部正常)
- ✅ M2 前端 MVP (Ebiten 88 键 + 按下动画 + 音画同步, 验收: 用户实测同步正确)
- ✅ M3 协议完整化 (serve + JSONL 全命令 + 曲库/简谱解析 + 黑盒集成测试)
- **M4 人类演奏 + 下落音符 ← 当前**
- M4 人类演奏 + 下落音符
- M5 发布流水线
- M4 人类演奏 + 下落音符
- M5 发布流水线

## 构建

```
go build ./cmd/pianostage
go test ./...
```

## 许可证

MIT
