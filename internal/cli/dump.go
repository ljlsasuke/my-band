package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ljlsasuke/my-band/internal/library"
	"github.com/ljlsasuke/my-band/internal/score"
)

type dumpEvent struct {
	T       float64 `json:"t"`
	Kind    string  `json:"kind"`
	Ch      int     `json:"ch"`
	Note    int     `json:"note,omitempty"`
	Vel     int     `json:"vel,omitempty"`
	Prog    int     `json:"prog,omitempty"`
	Ctrl    int     `json:"ctrl,omitempty"`
	CtrlVal int     `json:"ctrlval,omitempty"`
}

// cmdDump 调试用: 输出解析后的完整事件流, 供交叉验证 (如与前身 Python
// 解析器逐音符 diff) 与人工检查。
func cmdDump(args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Println("用法: my-band dump <曲名|文件>\n\n调试: 逐行输出解析后的事件 (JSONL)。")
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("用法: my-band dump <曲名|文件>")
	}
	path, err := library.Resolve(args[0])
	if err != nil {
		return err
	}
	events, err := score.Load(path, "C", score.DefaultBeat, score.DefaultVelocity)
	if err != nil {
		return err
	}
	w := json.NewEncoder(os.Stdout)
	for _, e := range events {
		if err := w.Encode(dumpEvent{
			T: e.T, Kind: e.Kind.String(), Ch: e.Ch, Note: e.Note,
			Vel: e.Vel, Prog: e.Prog, Ctrl: e.Ctrl, CtrlVal: e.CtrlVal,
		}); err != nil {
			return err
		}
	}
	return nil
}
