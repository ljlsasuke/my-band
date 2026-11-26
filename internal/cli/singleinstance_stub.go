//go:build !windows

package cli

import "os"

// acquireSingleInstance 非 Windows 占位。
func acquireSingleInstance() (release func(), alreadyRunning bool, err error) {
	if os.Getenv("MY_BAND_ALLOW_MULTI") == "1" {
		return func() {}, false, nil
	}
	return func() {}, false, nil
}
