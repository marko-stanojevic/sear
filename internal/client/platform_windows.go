//go:build windows

package client

import (
	"os"
	"os/exec"
)

func defaultStateFile() string {
	return `C:\ProgramData\sear\state.json`
}

func defaultWorkDir() string {
	return `C:\ProgramData\sear\work`
}

func rebootOS() error {
	cmd := exec.Command("shutdown", "/r", "/t", "5", "/c", "sear deployment reboot")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
