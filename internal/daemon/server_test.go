package daemon

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/marko-stanojevic/kompakt/internal/daemon/handlers"
	"github.com/marko-stanojevic/kompakt/internal/daemon/service"
	"github.com/marko-stanojevic/kompakt/internal/daemon/store"
)

type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (r *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	r.hijacked = true
	return nil, nil, nil
}

func TestLoggingPreservesHijacker(t *testing.T) {
	recorder := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)

	handler := logging(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if _, _, err := hijacker.Hijack(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))

	handler.ServeHTTP(recorder, req)

	if !recorder.hijacked {
		t.Fatal("expected wrapped response writer to preserve hijacker")
	}
	if recorder.Code != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d; want %d", recorder.Code, http.StatusSwitchingProtocols)
	}
}

func newServerTestEnv(t *testing.T) *handlers.Handler {
	t.Helper()
	st, err := store.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	hub := handlers.NewHub()
	return &handlers.Handler{
		Store:               st,
		JWTSecret:           []byte("test-jwt-secret-32-bytes-padding!"),
		RootPassword:        "admin123",
		TokenExpiryHours:    24,
		ArtifactsDir:        t.TempDir(),
		ServerURL:           "http://localhost:8080",
		RegistrationSecrets: map[string]string{"default": "reg-secret"},
		Hub:                 hub,
		Service:             &service.Manager{Store: st, Hub: hub, ServerURL: "http://localhost:8080"},
	}
}

func TestNewServer_Healthz(t *testing.T) {
	env := newServerTestEnv(t)
	h := NewServer(env)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Fatalf("body = %q; want ok", rr.Body.String())
	}
}

func TestDualAuth_BasicAndBearer(t *testing.T) {
	env := newServerTestEnv(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := dualAuth(env, next)

	// Basic auth path.
	basicReq := httptest.NewRequest(http.MethodGet, "/artifacts", nil)
	basicReq.SetBasicAuth("root", "admin123")
	basicRR := httptest.NewRecorder()
	h.ServeHTTP(basicRR, basicReq)
	if basicRR.Code != http.StatusNoContent {
		t.Fatalf("basic auth status = %d; want 204", basicRR.Code)
	}

	// Bearer auth path.
	claims := jwt.RegisteredClaims{
		Subject:   "client-1",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := tok.SignedString(env.JWTSecret)
	if err != nil {
		t.Fatalf("signed token: %v", err)
	}
	bearerReq := httptest.NewRequest(http.MethodGet, "/artifacts", nil)
	bearerReq.Header.Set("Authorization", "Bearer "+raw)
	bearerRR := httptest.NewRecorder()
	h.ServeHTTP(bearerRR, bearerReq)
	if bearerRR.Code != http.StatusNoContent {
		t.Fatalf("bearer auth status = %d; want 204", bearerRR.Code)
	}
}

type flushPushRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
	pushed  bool
}

func (r *flushPushRecorder) Flush() {
	r.flushed = true
}

func (r *flushPushRecorder) Push(_ string, _ *http.PushOptions) error {
	r.pushed = true
	return nil
}

func TestLoggingResponseWriter_FlushAndPush(t *testing.T) {
	base := &flushPushRecorder{ResponseRecorder: httptest.NewRecorder()}
	lrw := &loggingResponseWriter{ResponseWriter: base, status: http.StatusOK}

	lrw.Flush()
	if !base.flushed {
		t.Fatal("expected Flush to be forwarded")
	}

	if err := lrw.Push("/asset.js", nil); err != nil {
		t.Fatalf("Push returned error: %v", err)
	}
	if !base.pushed {
		t.Fatal("expected Push to be forwarded")
	}
}
