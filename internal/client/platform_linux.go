//go:build linux

package client

import (
	"fmt"
	"os"
	"os/exec"
)

func rebootOS() error {
	// Prefer systemctl so systemd has a chance to stop services cleanly.
	if path, err := exec.LookPath("systemctl"); err == nil {
		cmd := exec.Command(path, "reboot")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	// Fallback to reboot(8).
	if path, err := exec.LookPath("reboot"); err == nil {
		cmd := exec.Command(path)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("could not find systemctl or reboot in PATH")
}
