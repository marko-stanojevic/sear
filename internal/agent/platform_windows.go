//go:build windows

package agent

import (
	"os"
	"os/exec"
)

func rebootOS() error {
	cmd := exec.Command("shutdown", "/r", "/t", "5", "/c", "kompakt deployment reboot")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
