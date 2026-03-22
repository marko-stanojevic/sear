package identity

import (
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
