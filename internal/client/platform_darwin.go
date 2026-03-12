//go:build darwin

package client

import (
	"fmt"
	"os"
	"os/exec"
)

func defaultStateFile() string {
	return "/var/lib/sear/state.json"
}

func defaultWorkDir() string {
	return "/var/lib/sear/work"
}

func rebootOS() error {
	if path, err := exec.LookPath("shutdown"); err == nil {
		cmd := exec.Command(path, "-r", "now")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("could not find shutdown in PATH")
}
