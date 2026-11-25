package library

import (
	"os"
	"path/filepath"
	"testing"
)

// MY_BAND_HOME 重定向到临时目录, 测试完全隔离真实用户库。
func TestResolveAndList(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("MY_BAND_HOME", tmp)

	testFile := filepath.Join(tmp, "zz-test-piece.mid")
	os.WriteFile(testFile, []byte("x"), 0o644)

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
	// ~ 展开到真实用户主目录 (MY_BAND_HOME 只重定向曲库, 不重定向 HOME):
	// 文件不在真实主目录下, 应报"谱面不存在"
	_, err = Resolve("~/zz-test-piece.mid")
	if err == nil {
		t.Error("~ 展开指向真实主目录, 不存在时应报错")
	}
	// 不存在 → 报错且带库目录提示
	_, err = Resolve("no-such-piece")
	if err == nil || !contains(err.Error(), tmp) {
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

func TestUserDirEnvOverride(t *testing.T) {
	t.Setenv("MY_BAND_HOME", filepath.Join("custom", "dir"))
	if got := UserDir(); got != filepath.Join("custom", "dir") {
		t.Errorf("MY_BAND_HOME 未生效: %v", got)
	}
	os.Unsetenv("MY_BAND_HOME")
	t.Setenv("MY_BAND_HOME", "")
	if got := UserDir(); filepath.Base(got) != "songs" {
		t.Errorf("空值应回落默认目录, 实得 %v", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
