package cli

import (
	"flag"
	"fmt"
	"time"

	"github.com/ljlsasuke/my-band/internal/backend"
)

func cmdTestNote(args []string) error {
	fs := flag.NewFlagSet("test-note", flag.ExitOnError)
	device := fs.String("device", "auto", "设备: auto | 序号 | 名称")
	fs.Parse(args)

	b := &backend.MidiOut{}
	if err := b.Open(*device); err != nil {
		return err
	}
	defer b.Close()

	if err := b.Program(0, 0); err != nil { // 大钢琴
		return err
	}
	fmt.Println("中央 C ...")
	if err := b.NoteOn(0, 60, 90); err != nil {
		return err
	}
	time.Sleep(600 * time.Millisecond)
	if err := b.NoteOff(0, 60); err != nil {
		return err
	}
	fmt.Println("OK: 链路正常")
	return nil
}
