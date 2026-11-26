package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"runtime/debug"

	"github.com/ljlsasuke/my-band/internal/protocol"
)

// DisplayVersion 返回用于展示的版本号, 三层兜底:
//  1. ldflags 注入值 (release.sh 发版场景) 优先;
//  2. Go 内嵌 buildinfo (go install @vX.Y.Z 产物自带真实 tag);
//  3. 源码随手构建 (devel) 拼 commit 短哈希, 如 devel-183de89;
//     全部不可得时返回 Version 原值 (通常为 "dev")。
func DisplayVersion() string {
	bi, _ := debug.ReadBuildInfo()
	return displayVersion(Version, bi)
}

func displayVersion(v string, bi *debug.BuildInfo) string {
	if v != "" && v != "dev" {
		return v
	}
	if bi != nil {
		if mv := bi.Main.Version; mv != "" && mv != "(devel)" {
			return mv
		}
		rev, tag, dirty := "", "", false
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.tag":
				tag = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if tag != "" {
			if dirty {
				return tag + "-dirty"
			}
			return tag
		}
		if len(rev) >= 7 {
			out := "devel-" + rev[:7]
			if dirty {
				out += "-dirty"
			}
			return out
		}
	}
	return v
}

func cmdVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "输出 JSON (协议协商用)")
	fs.Parse(args)
	if *asJSON {
		b, _ := json.Marshal(map[string]string{"version": DisplayVersion(), "ap": fmt.Sprint(protocol.Version)})
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("my-band %s (AP/%d)\n", DisplayVersion(), protocol.Version)
	return nil
}
