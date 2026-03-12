package daemon

import (
	"bytes"
	"bufio"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	}), false)

	handler.ServeHTTP(recorder, req)

	if !recorder.hijacked {
		t.Fatal("expected wrapped response writer to preserve hijacker")
	}
	if recorder.Code != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d; want %d", recorder.Code, http.StatusSwitchingProtocols)
	}
}

func TestLoggingSkipsSuccessfulWebSocketUpgradeWhenNotDebug(t *testing.T) {
	var buf bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	})

	recorder := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	handler := logging(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}), false)

	handler.ServeHTTP(recorder, req)

	if strings.TrimSpace(buf.String()) != "" {
		t.Fatalf("expected no log output for successful websocket upgrade, got %q", buf.String())
	}
}
