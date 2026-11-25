package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAndList(t *testing.T) {
	// 用临时 HOME 隔离 (UserConfigDir 在 Windows 下不可重定向,
	// 因此这里只测相对逻辑: 直接对 UserDir 写入测试文件)
	dir, err := Ensure()
	if err != nil {
		t.Skipf("无法创建用户库目录 (环境限制): %v", err)
	}
	testFile := filepath.Join(dir, "zz-test-piece.mid")
	os.WriteFile(testFile, []byte("x"), 0644)
	defer os.Remove(testFile)

	// 裸名省略扩展名
	got, err := Resolve("zz-test-piece")
	if err != nil || got != testFile {
		t.Errorf("Resolve(裸名) = %q (err %v), want %q", got, err, testFile)
	}
	// 带扩展名
	got, err = Resolve("zz-test-piece.mid")
	if err != nil || got != testFile {
		t.Errorf("Resolve(带扩展名) = %q (err %v)", got, err)
	}
	// 显式路径
	got, err = Resolve(testFile)
	if err != nil || got != testFile {
		t.Errorf("Resolve(路径) = %q (err %v)", got, err)
	}
	// 不存在 → 报错且带库目录
	_, err = Resolve("no-such-piece")
	if err == nil || !contains(err.Error(), UserDir()) {
		t.Errorf("报错应含库目录提示: %v", err)
	}

	// List 应含测试曲
	names, err := List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range names {
		if n == "zz-test-piece" {
			found = true
		}
	}
	if !found {
		t.Errorf("List() 应含测试曲: %v", names)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
