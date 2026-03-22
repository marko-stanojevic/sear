// DetectShells returns a list of available shell interpreters on the system.
package identity

import (
	"os/exec"
)

func DetectShells() []string {
	candidates := []struct{ name, exe string }{
		{"bash", "bash"},
		{"sh", "sh"},
		{"pwsh", "pwsh"},
		{"powershell", "powershell.exe"},
		{"cmd", "cmd.exe"},
	}
	var found []string
	for _, c := range candidates {
		if _, err := exec.LookPath(c.exe); err == nil {
			found = append(found, c.name)
		}
	}
	return found
}
