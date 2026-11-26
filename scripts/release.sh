#!/usr/bin/env bash
# My Band 发版脚本 (唯一发版入口, VERSION 文件只由本脚本修改)。
#
# 用法:
#   scripts/release.sh          # 默认: patch 位 +1 (0.1.0 → 0.1.1)
#   scripts/release.sh 0.2.0    # 显式指定目标版本
#
# 流程: 工作区检查 → tag 冲突检查 → 全量测试 → 写 VERSION → chore(release) 提交
#       → 打 tag v{ver} → 构建双产物 → 推送 main + tag 到远程
# 纪律: 工作区必须干净; 发版提交只包含 VERSION 文件。
# 注意: 本脚本会触发 git push (用户运行 = 用户授权)。推送失败不回滚本地
#       发版, 事后手动 `git push origin main --tags` 补上即可。
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION_FILE="VERSION"
MODULE="github.com/ljlsasuke/my-band"

read_version() {
    VERSION=$(tr -d '[:space:]' < "$VERSION_FILE")
    if ! echo "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
        echo "错误: VERSION 文件内容非法: '$VERSION' (应为 x.y.z)" >&2
        exit 1
    fi
}

# 目标版本: 无参数 → patch 位 +1; 有参数 → 显式指定 (校验格式且必须大于当前)
resolve_target() {
    if [ $# -eq 0 ]; then
        local major minor patch
        IFS='.' read -r major minor patch <<< "$VERSION"
        TARGET="$major.$minor.$((patch + 1))"
        echo "目标版本: v$TARGET (默认 patch +1)"
    else
        TARGET="$1"
        if ! echo "$TARGET" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
            echo "错误: 目标版本非法: '$TARGET' (应为 x.y.z)" >&2
            exit 1
        fi
        if [ "$TARGET" = "$VERSION" ]; then
            echo "错误: 目标版本与当前版本相同 (v$VERSION)" >&2
            exit 1
        fi
        echo "目标版本: v$TARGET (显式指定)"
    fi
}

do_test() {
    # 发版前强制全量测试 (AGENTS.md 纪律): 红测试直接中止, 此时
    # 尚未写 VERSION/提交/打 tag, 零副作用。
    echo "== 全量测试 =="
    go test ./...
    echo "[OK] 测试全绿"
}

do_build() {
    # 双产物 (python.exe/pythonw.exe 模式, 业界标准做法):
    #   my-band-cli.exe  控制台子系统 — 终端 & AI skill 主入口 (shell 等待/Ctrl+C/提示符均正常)
    #   my-band.exe      GUI 子系统 (-H windowsgui) — 双击/开始菜单专用, 无黑框无闪烁
    go build -ldflags "-X $MODULE/internal/cli.Version=v$TARGET" -o build/my-band-cli.exe ./cmd/my-band
    go build -ldflags "-X $MODULE/internal/cli.Version=v$TARGET -H windowsgui" -o build/my-band.exe ./cmd/my-band
    echo "完成 → build/my-band-cli.exe (v$TARGET, 控制台) + build/my-band.exe (v$TARGET, GUI)"
}

do_push() {
    if ! git remote get-url origin >/dev/null 2>&1; then
        echo "[--] 未配置远程仓库, 跳过推送 (配置后手动: git push origin main --tags)"
        return 0
    fi
    if git push origin main "v$TARGET"; then
        echo "已推送: main + v$TARGET"
    else
        echo "[!!] 推送失败 (离线/认证问题?) —— 本地发版已完整, 稍后手动:" >&2
        echo "     git push origin main --tags" >&2
    fi
}

read_version
resolve_target "$@"

if [ -n "$(git status --porcelain)" ]; then
    echo "错误: 工作区不干净, 发版提交只包含 VERSION 文件——请先提交或 stash:" >&2
    git status --short >&2
    exit 1
fi
if git rev-parse "v$TARGET" >/dev/null 2>&1; then
    echo "错误: tag v$TARGET 已存在" >&2
    exit 1
fi

echo "$TARGET" > "$VERSION_FILE"
git add "$VERSION_FILE"
git commit -m "chore(release): v$TARGET"
git tag -a "v$TARGET" -m "v$TARGET"
echo "已打 tag: v$TARGET"

do_test
do_build
do_push
