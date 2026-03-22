package common

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPrintBannerMessage(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	title := "TITLE"
	content := "This is a test message."
	PrintBannerMessage(title, content)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("failed to read from pipe: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, title) || !strings.Contains(output, content) {
		t.Errorf("output missing title or content: %q", output)
	}
	if !strings.Contains(output, "┌") || !strings.Contains(output, "┐") || !strings.Contains(output, "└") {
		t.Errorf("output missing box drawing: %q", output)
	}
}
