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
// 裸调用 (无参数/双击 exe) = 自由弹奏, 不再是打印用法 (PLAN.md 演奏
// 模式矩阵: "打开应用 = 自由弹奏, 无菜单门槛")。
func Run(args []string) int {
	// GUI 子系统构建 (-H windowsgui): 终端启动时挂回父控制台,
	// 双击/管道场景无副作用 (见 console_windows.go)
	setupConsole()
	if len(args) == 0 {
		if err := cmdFreePlay(); err != nil {
			// 无控制台 (双击) 时 stderr 不可见, 弹原生错误框兜底
			alertBox("My Band", err.Error())
			fmt.Fprintln(os.Stderr, "错误:", err)
			return 1
		}
		return 0
	}
	var err error
	switch args[0] {
	case "help", "-h", "--help":
		Usage(os.Stdout)
		return 0
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

// Usage 打印帮助。参数风格统一用 -- (flag 包对 -/-- 一视同仁)。
func Usage(w *os.File) {
	fmt.Fprint(w, `My Band — 可被 AI 驱动、也可被人类演奏的琴曲舞台

用法:
  my-band                      自由弹奏: 打开钢琴界面, 用键盘/MIDI 键盘演奏
  my-band help                 显示本帮助
  my-band version [--json]     打印版本 (skill 做版本协商用)
  my-band doctor               环境自检: 设备枚举/自动选择/复音数
  my-band test-note [--device auto|序号|名称]  试弹一个中央 C
  my-band dump <文件>          调试: 逐行输出解析后的事件 (JSONL)
  my-band play <文件>          播放谱面 (.mid/.txt 简谱, 默认显示演奏窗口,
                               播完窗口保留转自由弹奏, 关窗退出)
     [--headless] [--device auto|序号|名称] [--max-sec N] [--look-ahead-ms N]
     [--key C] [--input auto|computer|midi]
  my-band serve                AP/1 协议服务 (JSONL over stdio, 常驻)
     [--headless] [--device auto|序号|名称] [--input auto|computer|midi]

各命令均支持 -h/--help 查看参数说明。
`)
}
