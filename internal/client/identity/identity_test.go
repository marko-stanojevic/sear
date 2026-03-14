package identity

import (
	"runtime"
	"testing"
)

func TestNormalizePlatformHint(t *testing.T) {
	tests := []struct {
		name string
		hint string
		want string
	}{
		{name: "auto empty", hint: "", want: detectPlatform()},
		{name: "auto explicit", hint: "auto", want: detectPlatform()},
		{name: "linux", hint: "linux", want: "linux"},
		{name: "mac", hint: "mac", want: "mac"},
		{name: "windows", hint: "windows", want: "windows"},
		{name: "legacy alias falls back to auto", hint: "darwin", want: detectPlatform()},
		{name: "unknown falls back to auto", hint: "custom", want: detectPlatform()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePlatformHint(tt.hint); got != tt.want {
				t.Fatalf("normalizePlatformHint(%q) = %q; want %q", tt.hint, got, tt.want)
			}
		})
	}
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
