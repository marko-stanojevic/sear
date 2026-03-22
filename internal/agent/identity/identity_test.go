package identity

import (
	"runtime"
	"testing"
)

func TestCollect_ReturnsValidPlatformInfo(t *testing.T) {
	info := Collect()

	if info.Platform == "" {
		t.Error("Collect() Platform should not be empty")
	}
	if info.Hostname == "" {
		t.Error("Collect() Hostname should not be empty")
	}
	if info.Metadata == nil {
		t.Error("Collect() Metadata should not be nil")
	}
	if info.Metadata["os"] == "" {
		t.Error("Collect() Metadata[os] should not be empty")
	}
	if info.Metadata["type"] != runtime.GOOS {
		t.Errorf("Collect() Metadata[type] = %q; want %q", info.Metadata["type"], runtime.GOOS)
	}
	if info.Metadata["arch"] != runtime.GOARCH {
		t.Errorf("Collect() Metadata[arch] = %q; want %q", info.Metadata["arch"], runtime.GOARCH)
	}
	if info.Metadata["machine_id"] == "" {
		t.Error("Collect() Metadata[machine_id] should not be empty")
	}
}

func TestDetectShells_ReturnsSlice(t *testing.T) {
	shells := DetectShells()
	// Just verify it doesn't panic and returns something sensible (even empty is ok).
	_ = shells
}

func TestDetectPlatform(t *testing.T) {
	want := "linux"
	switch runtime.GOOS {
	case "darwin":
		want = "mac"
	case "windows":
		want = "windows"
	}

	if got := detectPlatform(); got != want {
		t.Fatalf("detectPlatform() = %q; want %q", got, want)
	}
}
