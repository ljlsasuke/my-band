// PianoStage 入口: 只做一件事 —— 转发到 internal/cli。
// 命令实现一律写在 internal/cli 包, main.go 保持极薄。
package main

import (
	"os"

	"github.com/ljlsasuke/pianostage/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
