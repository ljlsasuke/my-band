package protocol

import (
	"encoding/json"
	"testing"
)

// 协议一致性: 命令与回执必须能无损往返 (skill 与 exe 的契约, 见 PLAN.md M0)。
func TestCommandRoundTrip(t *testing.T) {
	cmds := []Command{
		{Op: "ping"},
		{Op: "load", Source: "命运交响曲.mid"},
		{Op: "play"},
		{Op: "seek", Position: 12.5},
		{Op: "note", On: true, Ch: 0, Note: 60, Vel: 90},
		{Op: "program", Ch: 3, Prog: 73},
		{Op: "cc", Ch: 0, Ctrl: 64, CtrlVal: 127},
		{Op: "backend", Backend: "midiout", Device: "auto"},
		{Op: "query", Query: "devices"},
		{Op: "serve-only-field", Headless: true}, // Headless 仅 serve 用, 仍可序列化
	}
	for i, c := range cmds {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("marshal #%d: %v", i, err)
		}
		var back Command
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("unmarshal #%d: %v", i, err)
		}
		if back != c {
			t.Errorf("往返不一致 #%d:\n  want %+v\n  got  %+v", i, c, back)
		}
	}
}

func TestMessageFrames(t *testing.T) {
	frames := []Message{
		Hello(),
		OK("play"),
		Err("load", ErrLoad, "文件不存在: x.mid"),
		{Type: "state", Data: map[string]any{"playing": true}},
		TimeTick(3.14),
		{Type: "note", Data: map[string]any{"note": 60, "vel": 90}, T: 1.2},
	}
	for i, m := range frames {
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal #%d: %v", i, err)
		}
		var back Message
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("unmarshal #%d: %v", i, err)
		}
		if back.Type != m.Type || back.Op != m.Op || back.Code != m.Code || back.Ap != m.Ap {
			t.Errorf("帧 #%d 往返不一致: want %+v got %+v", i, m, back)
		}
	}
}

func TestHelloVersion(t *testing.T) {
	b, _ := json.Marshal(Hello())
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if ap, ok := raw["ap"].(float64); !ok || int(ap) != Version {
		t.Errorf("hello.ap = %v, want %d", raw["ap"], Version)
	}
	if raw["type"] != "hello" {
		t.Errorf("hello.type = %v, want hello", raw["type"])
	}
}
