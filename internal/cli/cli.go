// Package cli 承载 My Band 全部命令实现。
//
// cmd/my-band/main.go 保持极薄 (仅调 Run), 原因:
//  1. 命令逻辑与入口解耦, 集成测试 (tests/) 可直接调用 Run 驱动完整 CLI;
//  2. 命令依赖的内部包 (score/backend/core) 不会泄漏进 main;
//  3. 增删命令只改本包, main.go 永远不用动。
package cli

import (
	"fmt"
	"os"
)

// Version 二进制版本。默认 "dev" (源码直接构建), 正式发版由 scripts/release.sh
// 通过 ldflags 注入 (Go 惯例: 版本属构建产物而非源码)。
var Version = "dev"

// Run 执行命令, 返回进程退出码。
func Run(args []string) int {
	if len(args) == 0 {
		Usage(os.Stderr)
		return 2
	}
	var err error
	switch args[0] {
	case "version":
		err = cmdVersion(args[1:])
	case "doctor":
		err = cmdDoctor(args[1:])
	case "test-note":
		err = cmdTestNote(args[1:])
	case "dump":
		err = cmdDump(args[1:])
	case "play":
		err = cmdPlay(args[1:])
	case "serve":
		err = cmdServe(args[1:])
	default:
		Usage(os.Stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 1
	}
	return 0
}

// Usage 打印帮助。
func Usage(w *os.File) {
	fmt.Fprint(w, `My Band — 可被 AI 驱动、也可被人类演奏的琴曲舞台

用法:
  my-band version [-json]      打印版本 (skill 做版本协商用)
  my-band doctor               环境自检: 设备枚举/自动选择/复音数
  my-band test-note [-device auto|序号|名称]  试弹一个中央 C
  my-band dump <文件>           调试: 逐行输出解析后的事件 (JSONL)
  my-band play <文件.mid>      播放谱面 (headless, 走系统 GM 音源)
     [--device auto|序号|名称] [--max-sec N] [--look-ahead-ms N] [--show]
     --show: 显示演奏窗口 (Ebiten 前端, 按键动画)
  my-band serve [--show] [--device auto]   AP/1 协议服务 (JSONL over stdio)
`)
}
