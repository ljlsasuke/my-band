// Package tests 集成测试 (PLAN.md 测试策略): 黑盒驱动完整 CLI。
// 单元测试留在各包内 (Go 约定), 这里只做端到端验证: 构建真实 exe →
// spawn serve 进程 → 走 AP/1 协议全流程。
package tests

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ljlsasuke/pianostage/internal/protocol"
)

var exePath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "pianostage-itest")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exePath = filepath.Join(tmp, "pianostage-itest.exe")
	out, err := exec.Command("go", "build", "-o", exePath, "github.com/ljlsasuke/pianostage/cmd/pianostage").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "构建失败: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// serveClient 黑盒客户端: spawn serve 进程, 收发 JSONL。
type serveClient struct {
	t     *testing.T
	cmd   *exec.Cmd
	stdin io.WriteCloser
	sc    *bufio.Scanner
}

func startServe(t *testing.T, args ...string) *serveClient {
	t.Helper()
	cmd := exec.Command(exePath, append([]string{"serve"}, args...)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr // 服务端诊断信息直通测试输出
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &serveClient{t: t, cmd: cmd, stdin: stdin, sc: bufio.NewScanner(stdout)}
	t.Cleanup(func() {
		c.stdin.Close()
		done := make(chan struct{})
		go func() { c.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			c.cmd.Process.Kill()
		}
	})
	return c
}

func (c *serveClient) send(line string) {
	c.t.Helper()
	if _, err := fmt.Fprintln(c.stdin, line); err != nil {
		c.t.Fatalf("写入命令失败: %v", err)
	}
}

// recv 读取一帧, 跳过广播帧 (time/note), 匹配期望的 type/op。
func (c *serveClient) recv(wantType, wantOp string, timeout time.Duration) (protocol.Message, error) {
	type result struct {
		m   protocol.Message
		err error
	}
	ch := make(chan result, 1)
	go func() {
		for c.sc.Scan() {
			var m protocol.Message
			if err := json.Unmarshal(c.sc.Bytes(), &m); err != nil {
				ch <- result{err: fmt.Errorf("非法 JSON 帧: %q", c.sc.Bytes())}
				return
			}
			if m.Type == "time" || m.Type == "note" {
				continue
			}
			if wantType != "" && m.Type != wantType {
				continue
			}
			if wantOp != "" && m.Op != wantOp {
				continue
			}
			ch <- result{m: m}
			return
		}
		ch <- result{err: errors.New("stdout 流提前结束")}
	}()
	select {
	case r := <-ch:
		return r.m, r.err
	case <-time.After(timeout):
		return protocol.Message{}, fmt.Errorf("超时 (%v) 等待 type=%q op=%q", timeout, wantType, wantOp)
	}
}

func (c *serveClient) expectOK(op string) protocol.Message {
	c.t.Helper()
	m, err := c.recv("ok", op, 5*time.Second)
	if err != nil {
		c.t.Fatalf("期待 ok(op=%s): %v", op, err)
	}
	return m
}

func (c *serveClient) expectErr(op, code string) protocol.Message {
	c.t.Helper()
	m, err := c.recv("err", op, 5*time.Second)
	if err != nil {
		c.t.Fatalf("期待 err(op=%s): %v", op, err)
	}
	if m.Code != code {
		c.t.Fatalf("期待错误码 %s, 实得 %s (%s)", code, m.Code, m.Detail)
	}
	return m
}

func TestServeHelloAndPing(t *testing.T) {
	c := startServe(t)
	hello, err := c.recv("hello", "", 5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if hello.Ap != protocol.Version {
		t.Errorf("hello.ap = %d, want %d", hello.Ap, protocol.Version)
	}
	c.send(`{"op":"ping"}`)
	c.expectOK("ping")
	c.send(`{"op":"quit"}`)
	c.expectOK("quit")
}

func TestServeFullFlow(t *testing.T) {
	c := startServe(t)
	c.recv("hello", "", 5*time.Second)

	// 写一个简谱到曲库 (load 裸名走曲库解析)
	lib, err := func() (string, error) { // 复用 library 的目录逻辑
		return filepath.Join(os.Getenv("APPDATA"), "pianostage", "songs"), nil
	}()
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(lib, 0o755)
	piece := filepath.Join(lib, "zz-itest.txt")
	os.WriteFile(piece, []byte("format: jianpu\n1 2 3 4 5 6 7 1"), 0o644)
	defer os.Remove(piece)

	c.send(`{"op":"load","source":"zz-itest"}`)
	load := c.expectOK("load")
	data := load.Data.(map[string]any)
	if data["notes"].(float64) != 8 {
		t.Errorf("简谱 8 音, 实得 %v", data["notes"])
	}

	c.send(`{"op":"play"}`)
	c.expectOK("play")
	time.Sleep(400 * time.Millisecond) // 让曲内时间推进

	c.send(`{"op":"query","query":"time"}`)
	m := c.expectOK("query")
	if t0 := m.Data.(map[string]any)["t"].(float64); t0 < 0.2 {
		t.Errorf("播放中 songtime 应 >0.2, 实得 %v", t0)
	}

	c.send(`{"op":"pause"}`)
	c.expectOK("pause")
	c.send(`{"op":"query","query":"status"}`)
	st := c.expectOK("query").Data.(map[string]any)
	if st["paused"] != true {
		t.Errorf("pause 后 status.paused 应为 true: %v", st)
	}
	tFrozen := st["songtime"].(float64)
	time.Sleep(300 * time.Millisecond)
	c.send(`{"op":"query","query":"time"}`)
	if t2 := c.expectOK("query").Data.(map[string]any)["t"].(float64); t2 < tFrozen-0.05 || t2 > tFrozen+0.05 {
		t.Errorf("暂停期间曲内时间应冻结: %v vs %v", t2, tFrozen)
	}

	c.send(`{"op":"tempo","tempo":2.0}`)
	c.expectOK("tempo")
	c.send(`{"op":"stop"}`)
	c.expectOK("stop")
	c.send(`{"op":"quit"}`)
	c.expectOK("quit")
}

func TestServeErrors(t *testing.T) {
	c := startServe(t)
	c.recv("hello", "", 5*time.Second)

	c.send(`{"op":}`) // 非法 JSON
	m, err := c.recv("err", "", 5*time.Second)
	if err == nil && m.Code != protocol.ErrProto {
		t.Errorf("非 JSON 应 E_PROTO, 实得 %s", m.Code)
	}

	c.send(`{"op":"wat"}`)
	c.expectErr("wat", protocol.ErrOp)

	c.send(`{"op":"play"}`)
	c.expectErr("play", protocol.ErrState) // 未 load

	c.send(`{"op":"load","source":"no-such-thing-xyz"}`)
	c.expectErr("load", protocol.ErrLoad)

	c.send(`{"op":"query","query":"wat"}`)
	c.expectErr("query", protocol.ErrRange)

	c.send(`{"op":"quit"}`)
	c.expectOK("quit")
}

func TestServeStdinCloseExits(t *testing.T) {
	c := startServe(t)
	c.recv("hello", "", 5*time.Second)
	c.stdin.Close() // 客户端断开 → 优雅退出
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stdin 关闭后进程未退出")
	}
}
