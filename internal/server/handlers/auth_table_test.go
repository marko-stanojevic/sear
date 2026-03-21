package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func expiredToken(t *testing.T, secret []byte, subject string) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Subject:   subject,
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}
	return s
}

func wrongSigToken(t *testing.T, subject string) string {
	t.Helper()
	claims := jwt.RegisteredClaims{Subject: subject}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte("completely-different-secret"))
	if err != nil {
		t.Fatalf("sign wrong-sig token: %v", err)
	}
	return s
}

// ── RequireAgentAuth ─────────────────────────────────────────────────────────

func TestRequireAgentAuth_TokenVariants(t *testing.T) {
	env := newTestEnv(t)
	secret := []byte("test-secret-key-32-bytes-padding!")

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name      string
		buildReq  func() *http.Request
		wantCode  int
	}{
		{
			name: "no token at all",
			buildReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "malformed bearer — not a JWT",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
				r.Header.Set("Authorization", "Bearer not.a.valid.jwt")
				return r
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "wrong signature",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
				r.Header.Set("Authorization", "Bearer "+wrongSigToken(t, "some-client"))
				return r
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "expired token",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
				r.Header.Set("Authorization", "Bearer "+expiredToken(t, secret, "some-client"))
				return r
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "empty bearer value",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
				r.Header.Set("Authorization", "Bearer ")
				return r
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "empty query token",
			buildReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/v1/ws?token=", nil)
			},
			wantCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			env.RequireAgentAuth(okHandler).ServeHTTP(rr, tt.buildReq())
			if rr.Code != tt.wantCode {
				t.Errorf("status = %d; want %d", rr.Code, tt.wantCode)
			}
		})
	}
}

// ── RequireRootAuth ───────────────────────────────────────────────────────────

func TestRequireRootAuth_Variants(t *testing.T) {
	env := newTestEnv(t)
	secret := []byte("test-secret-key-32-bytes-padding!")

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Obtain a valid UI token via the login endpoint.
	loginRR := postJSON(t, env.HandleUILogin, "/api/v1/ui/login", map[string]string{"password": "admin123"}, "")
	if loginRR.Code != http.StatusOK {
		t.Fatalf("HandleUILogin setup: status=%d body=%s", loginRR.Code, loginRR.Body.String())
	}
	uiToken := decode[map[string]string](t, loginRR)["token"]

	// Obtain a valid agent token by registering a client.
	_, agentToken := registerAgent(t, env, "SN-ROOT-VARIANTS", "root-variants-client")

	const wantWWWAuth = `Basic realm="kompakt-root"`

	tests := []struct {
		name        string
		buildReq    func() *http.Request
		wantCode    int
		wantWWWAuth bool // whether WWW-Authenticate should be present
	}{
		{
			name: "correct basic auth",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/status", nil)
				r.SetBasicAuth("root", "admin123")
				return r
			},
			wantCode: http.StatusOK,
		},
		{
			name: "wrong password",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/status", nil)
				r.SetBasicAuth("root", "wrongpassword")
				return r
			},
			wantCode:    http.StatusUnauthorized,
			wantWWWAuth: true,
		},
		{
			name: "wrong username",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/status", nil)
				r.SetBasicAuth("admin", "admin123")
				return r
			},
			wantCode:    http.StatusUnauthorized,
			wantWWWAuth: true,
		},
		{
			name: "no auth",
			buildReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/status", nil)
			},
			wantCode:    http.StatusUnauthorized,
			wantWWWAuth: true,
		},
		{
			name: "valid UI JWT accepted",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/status", nil)
				r.Header.Set("Authorization", "Bearer "+uiToken)
				return r
			},
			wantCode: http.StatusOK,
		},
		{
			name: "agent JWT rejected — no ui audience",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/status", nil)
				r.Header.Set("Authorization", "Bearer "+agentToken)
				return r
			},
			wantCode: http.StatusUnauthorized,
			// Bearer was presented — no WWW-Authenticate
		},
		{
			name: "expired token rejected — no WWW-Authenticate",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/status", nil)
				r.Header.Set("Authorization", "Bearer "+expiredToken(t, secret, "root"))
				return r
			},
			wantCode: http.StatusUnauthorized,
			// Bearer was presented — no WWW-Authenticate
		},
		{
			name: "wrong signature rejected",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/status", nil)
				r.Header.Set("Authorization", "Bearer "+wrongSigToken(t, "root"))
				return r
			},
			wantCode: http.StatusUnauthorized,
			// Bearer was presented — no WWW-Authenticate
		},
		// ── HTMX requests: never send WWW-Authenticate ────────────────────────
		{
			name: "HTMX request — no auth — no WWW-Authenticate",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/status", nil)
				r.Header.Set("HX-Request", "true")
				return r
			},
			wantCode: http.StatusUnauthorized,
			// HTMX — no WWW-Authenticate even though no token
		},
		{
			name: "HTMX request — wrong basic auth — no WWW-Authenticate",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/status", nil)
				r.Header.Set("HX-Request", "true")
				r.SetBasicAuth("root", "wrongpassword")
				return r
			},
			wantCode: http.StatusUnauthorized,
			// HTMX — no WWW-Authenticate even with failed Basic auth
		},
		{
			name: "HTMX request — expired Bearer — no WWW-Authenticate",
			buildReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/status", nil)
				r.Header.Set("HX-Request", "true")
				r.Header.Set("Authorization", "Bearer "+expiredToken(t, secret, "root"))
				return r
			},
			wantCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			env.RequireRootAuth(okHandler).ServeHTTP(rr, tt.buildReq())
			if rr.Code != tt.wantCode {
				t.Errorf("status = %d; want %d", rr.Code, tt.wantCode)
			}
			got := rr.Header().Get("WWW-Authenticate")
			if tt.wantWWWAuth && got != wantWWWAuth {
				t.Errorf("WWW-Authenticate = %q; want %q", got, wantWWWAuth)
			}
			if !tt.wantWWWAuth && got != "" {
				t.Errorf("WWW-Authenticate = %q; want empty (should not trigger browser dialog)", got)
			}
		})
	}
}

// ── HandleUILogin ─────────────────────────────────────────────────────────────

func TestHandleUILogin(t *testing.T) {
	t.Run("correct password returns token", func(t *testing.T) {
		env := newTestEnv(t)
		rr := postJSON(t, env.HandleUILogin, "/api/v1/ui/login", map[string]string{"password": "admin123"}, "")
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200 (body: %s)", rr.Code, rr.Body.String())
		}
		resp := decode[map[string]string](t, rr)
		if resp["token"] == "" {
			t.Error("expected non-empty token in response")
		}
	})

	t.Run("wrong password rejected", func(t *testing.T) {
		env := newTestEnv(t)
		rr := postJSON(t, env.HandleUILogin, "/api/v1/ui/login", map[string]string{"password": "wrong"}, "")
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("status = %d; want 401", rr.Code)
		}
	})

	t.Run("GET not allowed", func(t *testing.T) {
		env := newTestEnv(t)
		rr := getRequest(t, env.HandleUILogin, "/api/v1/ui/login", "")
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d; want 405", rr.Code)
		}
	})

	t.Run("invalid JSON body rejected", func(t *testing.T) {
		env := newTestEnv(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ui/login", bytes.NewBufferString("{"))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		env.HandleUILogin(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", rr.Code)
		}
	})

	t.Run("empty password rejected", func(t *testing.T) {
		env := newTestEnv(t)
		rr := postJSON(t, env.HandleUILogin, "/api/v1/ui/login", map[string]string{"password": ""}, "")
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("status = %d; want 401", rr.Code)
		}
	})
}
