package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

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

func TestRunStepSimplePaths(t *testing.T) {
	logs := &logCollector{}

	res := RunStep(context.Background(), common.Step{Uses: "reboot", With: map[string]string{"reason": "maintenance"}}, nil, "", "", logs.logger())
	if !res.NeedsReboot || res.RebootReason != "maintenance" || res.Err != nil {
		t.Fatalf("unexpected reboot result: %+v", res)
	}

	res = RunStep(context.Background(), common.Step{Uses: "upload-logs"}, nil, "", "", logs.logger())
	if res.Err != nil || res.NeedsReboot {
		t.Fatalf("upload-logs should be no-op success: %+v", res)
	}

	res = RunStep(context.Background(), common.Step{Name: "bad", Uses: "unknown-step"}, nil, "", "", logs.logger())
	if res.Err == nil {
		t.Fatal("expected unknown step error")
	}
}

func TestRunStepShell(t *testing.T) {
	logs := &logCollector{}
	shell := "sh"
	run := "echo hello-kompakt"
	if runtime.GOOS == "windows" {
		shell = "cmd"
		run = "echo hello-kompakt"
	}

	res := RunStep(context.Background(), common.Step{Run: run, Shell: shell}, nil, "", "", logs.logger())
	if res.Err != nil {
		t.Fatalf("run shell failed: %v", res.Err)
	}
	joined := logs.joined()
	if !strings.Contains(strings.ToLower(joined), "hello-kompakt") {
		t.Fatalf("expected shell output in logs, got: %q", joined)
	}
}

func TestRunDownloadArtifact(t *testing.T) {
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
	logs := &logCollector{}
	res := runDownloadArtifact(context.Background(), step, ts.URL+"/artifacts", "", logs.logger())
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

func TestRunUploadArtifact(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.bin")
	if err := os.WriteFile(src, []byte("bin-data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s; want POST", r.Method)
		}
		if r.URL.Query().Get("name") != "remote.bin" {
			t.Fatalf("name query mismatch: %s", r.URL.RawQuery)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "bin-data" {
			t.Fatalf("uploaded body = %q", string(body))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	logs := &logCollector{}
	step := common.Step{Uses: "upload-artifact", With: map[string]string{"artifact": "remote.bin", "path": src}}
	res := runUploadArtifact(context.Background(), step, ts.URL, "", logs.logger())
	if res.Err != nil {
		t.Fatalf("upload-artifact failed: %v", res.Err)
	}
}
