// Package library 用户谱面库 (沿用前身 auto-piano 的「包外用户库」设计,
// PLAN.md 附录): 库目录在应用数据目录下, 程序更新/重装不影响;
// 裸曲名查找时用户库优先。
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// varScoreSuffixes 支持的谱面扩展名。
var scoreSuffixes = []string{".mid", ".midi", ".txt"}

// UserDir 用户库目录 (可能尚不存在): %AppData% (Roaming) / pianostage / songs。
func UserDir() string {
	base, err := os.UserConfigDir() // Windows: %APPDATA%; *nix: ~/.config
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "pianostage", "songs")
}

// Ensure 确保库目录存在, 返回其路径。
func Ensure() (string, error) {
	d := UserDir()
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

// looksLikePath 显式路径: 绝对路径 / 带目录分隔符 / 带".*"通配之外的写法。
func looksLikePath(s string) bool {
	if filepath.IsAbs(s) || strings.HasPrefix(s, "~") {
		return true
	}
	return strings.ContainsRune(s, '/') || strings.ContainsRune(s, '\\')
}

// Resolve 曲名或路径 → 实际文件。
//   - 显式路径: 原样使用 (必须存在)
//   - 裸曲名: 可省略扩展名, 依次尝试精确名 + 各谱面扩展名 (沿用前身约定)
//
// 找不到时错误信息带库目录, 便于用户放文件。
func Resolve(name string) (string, error) {
	s := strings.TrimSpace(strings.Trim(name, `"`))
	if s == "" {
		return "", fmt.Errorf("曲名为空")
	}
	// 展开用户目录 (~)
	if strings.HasPrefix(s, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			s = filepath.Join(home, s[1:])
		}
	}

	if looksLikePath(s) {
		if _, err := os.Stat(s); err != nil {
			return "", fmt.Errorf("谱面不存在: %s", s)
		}
		return s, nil
	}

	// 裸曲名: 用户库内尝试
	dir := UserDir()
	candidates := []string{filepath.Join(dir, s)}
	for _, ext := range scoreSuffixes {
		candidates = append(candidates, filepath.Join(dir, s+ext))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("曲库中找不到 %q (库目录: %s, 支持 %v; 也可传完整路径)", s, dir, scoreSuffixes)
}

// List 列出用户库内全部谱面 (不带扩展名的曲名, 排序)。
func List() ([]string, error) {
	dir := UserDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		for _, s := range scoreSuffixes {
			if ext == s {
				names = append(names, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
				break
			}
		}
	}
	sort.Strings(names)
	return names, nil
}
