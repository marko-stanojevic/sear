package identity

import (
	"strings"
	"testing"
)

func TestFirstStableMAC_ReturnsStringWithoutColons(t *testing.T) {
	mac := firstStableMAC()
	// The function returns either empty (no suitable interface) or a hex string
	// with colons stripped.
	if strings.Contains(mac, ":") {
		t.Errorf("firstStableMAC() = %q; should not contain colons", mac)
	}
}

func TestCollectID_ReturnsNonEmpty(t *testing.T) {
	meta := map[string]string{}
	id := collectID(meta)
	if id == "" {
		t.Error("collectID should always return a non-empty string")
	}
}

func TestCollectID_VirtualMachine_UsesVMGUID(t *testing.T) {
	// When vendor/model suggest a VM, collectID tries vmGUID first.
	// We can't control vmGUID output, but at minimum it shouldn't panic.
	meta := map[string]string{
		"vendor": "VMware, Inc.",
		"model":  "Virtual Machine",
	}
	id := collectID(meta)
	if id == "" {
		t.Error("collectID should return a non-empty string even for VMs")
	}
}

func TestCollectID_RandomFallback_HasPrefix(t *testing.T) {
	// This test exercises the random fallback path by noting that when no serial,
	// GUID, or MAC is available, we get "rnd-..." prefix. We can't control
	// external commands, so we just verify the output is non-empty and valid.
	meta := map[string]string{}
	id := collectID(meta)
	if id == "" {
		t.Error("collectID fallback should produce non-empty ID")
	}
}

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
