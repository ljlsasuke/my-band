package cli

import (
	"fmt"

	"github.com/ljlsasuke/pianostage/internal/backend"
	"github.com/ljlsasuke/pianostage/internal/protocol"
)

func cmdDoctor(args []string) error {
	fmt.Printf("== PianoStage doctor ==\n")
	fmt.Printf("[OK] pianostage %s (AP/%d)\n", Version, protocol.Version)

	b := &backend.MidiOut{}
	devs, err := b.Devices()
	if err != nil {
		return fmt.Errorf("MIDI 设备枚举失败: %w", err)
	}
	fmt.Printf("[--] MIDI 输出设备 %d 个:\n", len(devs))
	for _, d := range devs {
		fmt.Printf("     %d: %s\n", d.ID, d.Name)
	}
	if len(devs) == 0 {
		return fmt.Errorf("无任何 MIDI 输出设备 (可安装 VirtualMIDISynth)")
	}

	pick, err := backend.PickAuto(devs)
	if err != nil {
		return err
	}
	fmt.Printf("[OK] auto 设备选择: %s\n", pick.Name)
	if pick.Name != devs[0].Name {
		fmt.Printf("[--] 提示: 当前音源为系统兜底 (32 复音)。安装 VirtualMIDISynth + SoundFont 可显著提升音质\n")
	}
	fmt.Println("== 自检结束 ==")
	return nil
}
