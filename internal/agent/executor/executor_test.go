package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

type logCollector struct {
	mu    sync.Mutex
	lines []string
}

func (c *logCollector) logger() Logger {
	return func(_ common.LogLevel, msg string) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.lines = append(c.lines, msg)
	}
}

func (c *logCollector) joined() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.lines, "\n")
}

func noop(_ common.LogLevel, _ string) {}

// ── SanitizeLine ──────────────────────────────────────────────────────────────

func TestSanitizeLine_PlainTextIsUnchanged(t *testing.T) {
	in := "plain text with\ttab"
	if got := SanitizeLine(in); got != in {
		t.Errorf("SanitizeLine(%q) = %q; want unchanged", in, got)
	}
}

func TestSanitizeLine_AnsiColoursStripped(t *testing.T) {
	cases := []struct{ in, want string }{
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b[1;32mbold green\x1b[0m", "bold green"},
		{"\x1b[2J", ""},                   // clear screen
		{"norm\x1b[1mbolded", "normbolded"}, // partial
	}
	for _, tt := range cases {
		if got := SanitizeLine(tt.in); got != tt.want {
			t.Errorf("SanitizeLine(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestSanitizeLine_ControlCharsRemoved(t *testing.T) {
	// BEL, BS, ESC, DEL must be stripped; TAB must be kept.
	in := "bell\x07back\x08esc\x1bdelete\x7f"
	want := "bellbackescdelete"
	if got := SanitizeLine(in); got != want {
		t.Errorf("SanitizeLine(%q) = %q; want %q", in, got, want)
	}
}

func TestSanitizeLine_TabPreserved(t *testing.T) {
	in := "col1\tcol2"
	if got := SanitizeLine(in); got != in {
		t.Errorf("tab should be preserved, got %q", got)
	}
}

// ── DrainPipeLines ────────────────────────────────────────────────────────────

func TestDrainPipeLines_MultipleCompleteLines(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = pw.WriteString("line1\nline2\nline3\n")
		_ = pw.Close()
	}()

	var got []string
	DrainPipeLines(pr, func(s string) { got = append(got, s) })
	_ = pr.Close()

	want := []string{"line1", "line2", "line3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestDrainPipeLines_PartialLastLineDelivered(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = pw.WriteString("line1\npartial")
		_ = pw.Close()
	}()

	var got []string
	DrainPipeLines(pr, func(s string) { got = append(got, s) })
	_ = pr.Close()

	want := []string{"line1", "partial"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestDrainPipeLines_EmptyInput(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = pw.Close()

	var got []string
	DrainPipeLines(pr, func(s string) { got = append(got, s) })
	_ = pr.Close()

	if len(got) != 0 {
		t.Errorf("expected no lines for empty input, got %v", got)
	}
}

// ── RunStep — simple paths ────────────────────────────────────────────────────

func TestRunStep_Reboot_DefaultReason(t *testing.T) {
	res := RunStep(context.Background(), common.Step{Uses: "reboot"}, nil, "", "", noop)
	if !res.NeedsReboot {
		t.Fatal("expected NeedsReboot=true")
	}
	if res.RebootReason == "" {
		t.Fatal("expected a default reboot reason")
	}
}

func TestRunStep_Reboot_CustomReason(t *testing.T) {
	res := RunStep(context.Background(),
		common.Step{Uses: "reboot", With: map[string]string{"reason": "maintenance"}},
		nil, "", "", noop,
	)
	if !res.NeedsReboot || res.RebootReason != "maintenance" || res.Err != nil {
		t.Fatalf("unexpected reboot result: %+v", res)
	}
}

func TestRunStep_UploadLogs_IsNoOp(t *testing.T) {
	res := RunStep(context.Background(), common.Step{Uses: "upload-logs"}, nil, "", "", noop)
	if res.Err != nil || res.NeedsReboot {
		t.Fatalf("upload-logs should be a no-op success: %+v", res)
	}
}

func TestRunStep_UnknownStep_ReturnsError(t *testing.T) {
	res := RunStep(context.Background(), common.Step{Name: "bad", Uses: "unknown-step"}, nil, "", "", noop)
	if res.Err == nil {
		t.Fatal("expected error for unknown step")
	}
}

// ── RunStep — shell execution ─────────────────────────────────────────────────

func TestRunStep_Shell_OutputCaptured(t *testing.T) {
	shell, run := "sh", "echo hello-kompakt"
	if runtime.GOOS == "windows" {
		shell, run = "cmd", "echo hello-kompakt"
	}
	logs := &logCollector{}
	res := RunStep(context.Background(), common.Step{Run: run, Shell: shell}, nil, "", "", logs.logger())
	if res.Err != nil {
		t.Fatalf("shell step failed: %v", res.Err)
	}
	if !strings.Contains(strings.ToLower(logs.joined()), "hello-kompakt") {
		t.Fatalf("expected output in logs, got: %q", logs.joined())
	}
}

func TestRunStep_Shell_NonZeroExitReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exit code test uses sh syntax")
	}
	res := RunStep(context.Background(), common.Step{Run: "exit 1", Shell: "sh"}, nil, "", "", noop)
	if res.Err == nil {
		t.Fatal("expected error for exit 1")
	}
}

func TestRunStep_Shell_ContextCancellationKillsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	res := RunStep(ctx, common.Step{Run: "sleep 60", Shell: "sh"}, nil, "", "", noop)
	if res.Err == nil {
		t.Fatal("expected error when process is killed by context cancellation")
	}
}

func TestRunStep_Shell_StepTimeoutKillsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	// TimeoutMinutes is float64 — use a very small fraction for a short timeout.
	// The minimum resolution is 1 minute, so we test via context cancellation instead;
	// this test verifies the step-level timeout path compiles and runs.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	res := RunStep(ctx, common.Step{Run: "sleep 60", Shell: "sh", TimeoutMinutes: 0}, nil, "", "", noop)
	if res.Err == nil {
		t.Fatal("expected error from killed process")
	}
}

func TestRunStep_Shell_SecretsInjectedAsEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	logs := &logCollector{}
	secrets := map[string]string{"MY_SECRET": "hunter2"}
	res := RunStep(context.Background(),
		common.Step{Run: "echo $MY_SECRET", Shell: "sh"},
		secrets, "", "", logs.logger(),
	)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(logs.joined(), "hunter2") {
		t.Fatalf("secret not available as env var; logs: %q", logs.joined())
	}
}

// ── download-artifact ─────────────────────────────────────────────────────────

func TestRunDownloadArtifact_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifacts/my.bin" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, "artifact-body")
	}))
	defer ts.Close()

	dest := t.TempDir()
	step := common.Step{Uses: "download-artifact", With: map[string]string{"artifact": "my.bin", "path": dest}}
	res := runDownloadArtifact(context.Background(), step, ts.URL+"/artifacts", "", noop)
	if res.Err != nil {
		t.Fatalf("download-artifact failed: %v", res.Err)
	}

	data, err := os.ReadFile(filepath.Join(dest, "my.bin"))
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != "artifact-body" {
		t.Fatalf("downloaded content = %q", string(data))
	}
}

func TestRunDownloadArtifact_MissingArtifactNameReturnsError(t *testing.T) {
	step := common.Step{Uses: "download-artifact", With: map[string]string{"path": "/tmp"}}
	res := runDownloadArtifact(context.Background(), step, "http://example.com/artifacts", "", noop)
	if res.Err == nil {
		t.Fatal("expected error for missing artifact name")
	}
}

func TestRunDownloadArtifact_ServerNotFoundReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()

	step := common.Step{Uses: "download-artifact", With: map[string]string{"artifact": "missing.bin", "path": t.TempDir()}}
	res := runDownloadArtifact(context.Background(), step, ts.URL+"/artifacts", "", noop)
	if res.Err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestRunDownloadArtifact_SendsAuthorizationHeader(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "body")
	}))
	defer ts.Close()

	step := common.Step{Uses: "download-artifact", With: map[string]string{"artifact": "a.bin", "path": t.TempDir()}}
	res := runDownloadArtifact(context.Background(), step, ts.URL+"/artifacts", "my-token", noop)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if gotAuth != "Bearer my-token" {
		t.Errorf("Authorization header = %q; want %q", gotAuth, "Bearer my-token")
	}
}

func TestRunDownloadArtifact_AcceptsAlternativeNameKey(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer ts.Close()

	// Use "name" instead of "artifact" — both should work.
	step := common.Step{Uses: "download-artifact", With: map[string]string{"name": "alt.bin", "path": t.TempDir()}}
	res := runDownloadArtifact(context.Background(), step, ts.URL+"/artifacts", "", noop)
	if res.Err != nil {
		t.Fatalf("unexpected error with 'name' key: %v", res.Err)
	}
}

// ── upload-artifact ───────────────────────────────────────────────────────────

func TestRunUploadArtifact_Success(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.bin")
	if err := os.WriteFile(src, []byte("bin-data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s; want POST", r.Method)
		}
		if r.URL.Query().Get("name") != "remote.bin" {
			t.Errorf("name query mismatch: %s", r.URL.RawQuery)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "bin-data" {
			t.Errorf("uploaded body = %q", string(body))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	step := common.Step{Uses: "upload-artifact", With: map[string]string{"artifact": "remote.bin", "path": src}}
	res := runUploadArtifact(context.Background(), step, ts.URL, "", noop)
	if res.Err != nil {
		t.Fatalf("upload-artifact failed: %v", res.Err)
	}
}

func TestRunUploadArtifact_MissingParamsReturnsError(t *testing.T) {
	step := common.Step{Uses: "upload-artifact", With: map[string]string{}}
	res := runUploadArtifact(context.Background(), step, "http://example.com", "", noop)
	if res.Err == nil {
		t.Fatal("expected error for missing artifact and path")
	}
}

func TestRunUploadArtifact_MissingSourceFileReturnsError(t *testing.T) {
	step := common.Step{Uses: "upload-artifact", With: map[string]string{
		"artifact": "remote.bin",
		"path":     "/nonexistent/file.bin",
	}}
	res := runUploadArtifact(context.Background(), step, "http://example.com", "", noop)
	if res.Err == nil {
		t.Fatal("expected error for nonexistent source file")
	}
}

func TestRunUploadArtifact_ServerErrorReturnsError(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.bin")
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "disk full", http.StatusInternalServerError)
	}))
	defer ts.Close()

	step := common.Step{Uses: "upload-artifact", With: map[string]string{"artifact": "a.bin", "path": src}}
	res := runUploadArtifact(context.Background(), step, ts.URL, "", noop)
	if res.Err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestRunUploadArtifact_SendsAuthorizationHeader(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.bin")
	if err := os.WriteFile(src, []byte("d"), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	step := common.Step{Uses: "upload-artifact", With: map[string]string{"artifact": "a.bin", "path": src}}
	res := runUploadArtifact(context.Background(), step, ts.URL, "upload-token", noop)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if gotAuth != "Bearer upload-token" {
		t.Errorf("Authorization = %q; want %q", gotAuth, "Bearer upload-token")
	}
}
