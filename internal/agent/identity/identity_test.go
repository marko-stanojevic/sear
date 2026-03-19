package identity

import (
	"runtime"
	"testing"
)

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
