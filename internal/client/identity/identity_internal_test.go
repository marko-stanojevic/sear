package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsLikelyVM(t *testing.T) {
	if !isLikelyVM(map[string]string{"vendor": "VMware, Inc.", "model": "Virtual Machine"}) {
		t.Fatal("expected VM markers to be detected")
	}
	if isLikelyVM(map[string]string{"vendor": "Dell Inc.", "model": "PowerEdge R650"}) {
		t.Fatal("expected physical machine markers to not be detected as VM")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	got := firstNonEmpty("", "NAME=value", "ignored")
	if got != "value" {
		t.Fatalf("firstNonEmpty parsed value = %q; want value", got)
	}

	got = firstNonEmpty("\n\n", "  \"quoted\"  ")
	if got != "quoted" {
		t.Fatalf("firstNonEmpty quoted value = %q; want quoted", got)
	}

	got = firstNonEmpty("", "")
	if got != "" {
		t.Fatalf("firstNonEmpty empty = %q; want empty", got)
	}
}

func TestCleanHardwareValue(t *testing.T) {
	if got := cleanHardwareValue(" unknown "); got != "" {
		t.Fatalf("cleanHardwareValue unknown = %q; want empty", got)
	}
	if got := cleanHardwareValue("not specified"); got != "" {
		t.Fatalf("cleanHardwareValue not specified = %q; want empty", got)
	}
	if got := cleanHardwareValue("Dell"); got != "Dell" {
		t.Fatalf("cleanHardwareValue Dell = %q; want Dell", got)
	}
}

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
