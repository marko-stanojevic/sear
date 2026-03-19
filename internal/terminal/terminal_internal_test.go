package terminal

import (
	"bytes"
	"testing"
)

func TestCRLFWriter_TranslatesLFtoCRLF(t *testing.T) {
	var buf bytes.Buffer
	w := &crlfWriter{w: &buf}

	input := []byte("line1\nline2\n")
	n, err := w.Write(input)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write returned n=%d; want %d", n, len(input))
	}

	got := buf.String()
	want := "line1\r\nline2\r\n"
	if got != want {
		t.Errorf("output = %q; want %q", got, want)
	}
}

func TestCRLFWriter_NormalizesExistingCRLF(t *testing.T) {
	var buf bytes.Buffer
	w := &crlfWriter{w: &buf}

	// Input already has \r\n — must not be doubled.
	_, _ = w.Write([]byte("a\r\nb\n"))
	got := buf.String()
	want := "a\r\nb\r\n"
	if got != want {
		t.Errorf("output = %q; want %q", got, want)
	}
}

func TestSetup_DoesNotPanic(t *testing.T) {
	// Setup replaces the global slog default; just ensure it does not panic.
	Setup(false)
	Setup(true)
}
