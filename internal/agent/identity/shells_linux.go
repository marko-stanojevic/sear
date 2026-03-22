//go:build linux

package identity

import "os/exec"

func DetectShells() []string {
	candidates := []struct{ name, exe string }{
		{"bash", "bash"},
		{"sh", "sh"},
		{"pwsh", "pwsh"}, // PowerShell Core
	}
	var found []string
	for _, c := range candidates {
		if _, err := exec.LookPath(c.exe); err == nil {
			found = append(found, c.name)
		}
	}
	return found
}
