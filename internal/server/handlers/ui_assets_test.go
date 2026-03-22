package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marko-stanojevic/kompakt/internal/server/handlers"
)

func TestServeUIAsset_JS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ui/assets/app.js", nil)
	rr := httptest.NewRecorder()
	handlers.ServeUIAsset(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q; want javascript", ct)
	}
}

func TestServeUIAsset_CSS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ui/assets/style.css", nil)
	rr := httptest.NewRecorder()
	handlers.ServeUIAsset(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "css") {
		t.Errorf("Content-Type = %q; want css", ct)
	}
}

func TestServeUIAsset_NotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ui/assets/does-not-exist.js", nil)
	rr := httptest.NewRecorder()
	handlers.ServeUIAsset(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", rr.Code)
	}
}

func TestServeUIAsset_SecurityHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ui/assets/app.js", nil)
	rr := httptest.NewRecorder()
	handlers.ServeUIAsset(rr, req)

	if rr.Header().Get("X-Frame-Options") == "" {
		t.Error("X-Frame-Options header should be set")
	}
	if rr.Header().Get("X-Content-Type-Options") == "" {
		t.Error("X-Content-Type-Options header should be set")
	}
	if rr.Header().Get("Cache-Control") == "" {
		t.Error("Cache-Control header should be set")
	}
}

func TestServeUIAsset_OtherFileType(t *testing.T) {
	// htmx.min.js also exists and should be served correctly.
	req := httptest.NewRequest(http.MethodGet, "/ui/assets/htmx.min.js", nil)
	rr := httptest.NewRecorder()
	handlers.ServeUIAsset(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", rr.Code)
	}
}
