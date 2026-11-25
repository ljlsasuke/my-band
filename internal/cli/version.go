package cli

import (
	"encoding/json"
	"flag"
	"fmt"

	"github.com/ljlsasuke/my-band/internal/protocol"
)

func cmdVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "输出 JSON (协议协商用)")
	fs.Parse(args)
	if *asJSON {
		b, _ := json.Marshal(map[string]string{"version": Version, "ap": fmt.Sprint(protocol.Version)})
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("my-band %s (AP/%d)\n", Version, protocol.Version)
	return nil
}
