#!/usr/bin/env bash
# My Band 日常构建: 跑全量测试 + 编译双产物到 build/。
#
# 与 scripts/release.sh 的区别: 不 bump 版本/不提交/不打 tag/不推送。
# 版本号不注入 ldflags, 二进制里显示 buildinfo 推导的开发版 (如 v0.1.3-...-dirty)。
# 用法: scripts/build.sh
set -euo pipefail
cd "$(dirname "$0")/.."

MODULE="github.com/ljlsasuke/my-band"

# 测试全绿再打包, 避免产出已知坏的 exe
echo "== 全量测试 =="
go test ./...
echo "[OK] 测试全绿"
# 双产物 (python.exe/pythonw.exe 模式):
#   my-band-cli.exe  控制台子系统 — 终端 & AI skill 主入口
#   my-band.exe      GUI 子系统 — 双击/开始菜单专用, 无黑框
go build -o build/my-band-cli.exe ./cmd/my-band
go build -ldflags "-H windowsgui" -o build/my-band.exe ./cmd/my-band
echo "完成 → build/my-band-cli.exe (控制台) + build/my-band.exe (GUI)"
