package cli

import (
	"runtime/debug"
	"testing"
)

func TestDisplayVersion(t *testing.T) {
	// ldflags 注入值优先 (release.sh 场景)
	if got := displayVersion("v9.9.9", nil); got != "v9.9.9" {
		t.Errorf("注入值被覆盖: got %q", got)
	}

	// go install @vX.Y.Z 产物: buildinfo 携带真实模块版本
	bi := &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}
	if got := displayVersion("dev", bi); got != "v1.2.3" {
		t.Errorf("buildinfo 模块版本未生效: got %q", got)
	}

	// 源码随手构建 (devel): commit 短哈希
	bi = &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
		},
	}
	if got := displayVersion("dev", bi); got != "devel-abcdef1" {
		t.Errorf("(devel)+revision = %q, want devel-abcdef1", got)
	}

	// 干净检出: tag 即版本
	bi = &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
			{Key: "vcs.tag", Value: "v0.1.1"},
		},
	}
	if got := displayVersion("dev", bi); got != "v0.1.1" {
		t.Errorf("(devel)+tag = %q, want v0.1.1", got)
	}

	// 工作区脏: tag 加 -dirty 后缀
	bi.Settings = append(bi.Settings, debug.BuildSetting{Key: "vcs.modified", Value: "true"})
	if got := displayVersion("dev", bi); got != "v0.1.1-dirty" {
		t.Errorf("脏树 = %q, want v0.1.1-dirty", got)
	}

	// 无任何版本信息 → 原值
	if got := displayVersion("dev", nil); got != "dev" {
		t.Errorf("无信息 = %q, want dev", got)
	}
}
