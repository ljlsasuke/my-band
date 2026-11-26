//go:build windows

package cli

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
	user32            = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW   = user32.NewProc("MessageBoxW")
)

const (
	attachParentProcess = ^uintptr(0) // (DWORD)-1: 挂到父进程的控制台
	mbIconError         = 0x00000010
	mbSetForeground     = 0x00010000
)

// consoleUsable 标记 stderr 是否可达 (终端/管道)。
// 双击启动 (GUI 子系统, 无控制台可挂) 时为 false → 错误改走 MessageBox。
var consoleUsable bool

// setupConsole 修复 GUI 子系统 (-H windowsgui) 构建在终端里"打印不出来"的问题。
//
// -H windowsgui 使双击 exe 不再创建控制台黑框 (闪烁根治), 代价是进程
// 默认不挂任何控制台, stdout/stderr 写入丢失。对策 (7z 等工具同款):
//
//   - 终端启动 (cmd/PowerShell): AttachConsole 挂回父 shell 的控制台,
//     无效的 std 句柄重开到 CONIN$/CONOUT$ —— 子命令输出照旧;
//   - 管道/重定向 (AI skill spawn, `serve | ...`): std 句柄是继承来的
//     管道, 本就有效, 原样保留 (绝不重开覆盖);
//   - 双击/开始菜单 (父进程 explorer 无控制台): attach 失败, 无副作用。
func setupConsole() {
	consoleUsable = reopenStdio()
}

func reopenStdio() bool {
	if !stdHandleValid(os.Stdout) && !attachParentConsole() {
		return false // 双击场景: 无控制台, 什么都不做
	}
	if !stdHandleValid(os.Stdin) {
		if h, err := windows.CreateFile(windows.StringToUTF16Ptr("CONIN$"),
			windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
			nil, windows.OPEN_EXISTING, 0, 0); err == nil {
			os.Stdin = os.NewFile(uintptr(h), "CONIN$")
		}
	}
	if !stdHandleValid(os.Stdout) {
		// stdout/stderr 同指 CONOUT$ (与控制台程序默认布局一致)
		if h, err := windows.CreateFile(windows.StringToUTF16Ptr("CONOUT$"),
			windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
			nil, windows.OPEN_EXISTING, 0, 0); err == nil {
			os.Stdout = os.NewFile(uintptr(h), "CONOUT$")
			os.Stderr = os.NewFile(uintptr(h), "CONOUT$")
		}
	}
	return stdHandleValid(os.Stdout)
}

// attachParentConsole 挂到父进程的控制台 (GUI 子系统从终端启动时用)。
func attachParentConsole() bool {
	r, _, _ := procAttachConsole.Call(attachParentProcess)
	return r != 0
}

// stdHandleValid Windows 句柄 0 与 InvalidHandle 均无效。
func stdHandleValid(f *os.File) bool {
	if f == nil {
		return false
	}
	h := windows.Handle(f.Fd())
	return h != 0 && h != windows.InvalidHandle
}

// alertBox 双击场景的错误兜底: 无控制台, stderr 用户看不到,
// 改弹原生错误对话框。终端/管道场景 (consoleUsable) 不弹 —— 错误已在输出流里。
func alertBox(title, text string) {
	if consoleUsable {
		return
	}
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	procMessageBoxW.Call(0,
		uintptr(unsafe.Pointer(m)),
		uintptr(unsafe.Pointer(t)),
		mbIconError|mbSetForeground)
}
