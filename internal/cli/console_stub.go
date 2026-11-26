//go:build !windows

package cli

// setupConsole 非 Windows 占位: 无 GUI 子系统/控制台问题。
func setupConsole() {}

// alertBox 非 Windows 占位: 错误照常走 stderr。
func alertBox(_, _ string) {}
