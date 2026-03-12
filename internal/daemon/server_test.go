package daemon

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
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
	}))

	handler.ServeHTTP(recorder, req)

	if !recorder.hijacked {
		t.Fatal("expected wrapped response writer to preserve hijacker")
	}
	if recorder.Code != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d; want %d", recorder.Code, http.StatusSwitchingProtocols)
	}
}
