//go:build windows

package client

import (
	"fmt"
	"os/exec"
)

func rebootOS() error {
	cmd := exec.Command("shutdown", "/r", "/t", "0")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("reboot: %w", err)
	}
	return nil
}
