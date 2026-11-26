//go:build windows

package cli

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

const mutexName = "Local\\MyBand.SingleInstance"

// acquireSingleInstance 单实例保护: 已有实例在运行时返回 alreadyRunning=true。
// 成功时返回释放函数 (进程退出自动释放, 一般无需手动调)。
func acquireSingleInstance() (release func(), alreadyRunning bool, err error) {
	// 测试旁路: 集成测试需要并行 spawn 多个实例
	if os.Getenv("MY_BAND_ALLOW_MULTI") == "1" {
		return func() {}, false, nil
	}
	ptr, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return nil, false, err
	}
	handle, err := windows.CreateMutex(nil, false, ptr)
	if err != nil {
		// ERROR_ALREADY_EXISTS (183): 已有实例
		if err == windows.ERROR_ALREADY_EXISTS {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("创建互斥锁失败: %w", err)
	}
	return func() { windows.CloseHandle(handle) }, false, nil
}
