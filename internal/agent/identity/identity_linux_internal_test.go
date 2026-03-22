//go:build linux

package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	content := "NAME=Debian\nVERSION=12\nPRETTY_NAME=Debian GNU/Linux 12\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write os-release: %v", err)
	}

	vals := parseOSRelease(path)
	if vals["NAME"] != "Debian" {
		t.Fatalf("NAME = %q; want Debian", vals["NAME"])
	}
	if vals["VERSION"] != "12" {
		t.Fatalf("VERSION = %q; want 12", vals["VERSION"])
	}
	if vals["PRETTY_NAME"] != "Debian GNU/Linux 12" {
		t.Fatalf("PRETTY_NAME = %q; want Debian GNU/Linux 12", vals["PRETTY_NAME"])
	}
}
