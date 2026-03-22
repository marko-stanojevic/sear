//go:build darwin || windows

package identity

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func runAndTrim(cmd string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cmd, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
