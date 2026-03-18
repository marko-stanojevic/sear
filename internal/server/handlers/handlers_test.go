package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/server/handlers"
	"github.com/marko-stanojevic/kompakt/internal/server/service"
	"github.com/marko-stanojevic/kompakt/internal/server/store"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

func newTestEnv(t *testing.T) *handlers.Handler {
	t.Helper()
	st, err := store.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	hub := handlers.NewHub()
	return &handlers.Handler{
		Store:            st,
		AgentJWTSecret:        []byte("test-secret-key-32-bytes-padding!"),
		RootPassword:     "admin123",
		TokenExpiryHours: 24,
		ArtifactsDir:     t.TempDir(),
		ServerURL:        "http://localhost:8080",
		Hub:              hub,
		Service:          &service.Manager{Store: st, Hub: hub, ServerURL: "http://localhost:8080"},
		RegistrationSecrets: map[string]string{
			"prod": "reg-secret-1",
		},
	}
}

func postJSON(t *testing.T, handler http.HandlerFunc, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	return requestWithClientID(t, http.MethodPost, handler, path, body, token, "")
}

func requestWithClientID(t *testing.T, method string, handler http.HandlerFunc, path string, body any, token, clientID string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if clientID != "" {
		req.Header.Set("X-Client-ID", clientID)
	}
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func getRequest(t *testing.T, handler http.HandlerFunc, path string, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func decode[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rr.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rr.Body.String())
	}
	return v
}

// registerAgent is a test helper that registers an agent and returns its token.
func registerAgent(t *testing.T, env *handlers.Handler, machineID, hostname string) (string, string) {
	t.Helper()
	rr := postJSON(t, env.HandleAgentRegister, "/api/v1/register", common.RegistrationRequest{
		Platform:           common.PlatformLinux,
		Hostname:           hostname,
		Metadata:           map[string]string{"machine_id": machineID},
		RegistrationSecret: "reg-secret-1",
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("register: status=%d body=%s", rr.Code, rr.Body.String())
	}
	resp := decode[common.RegistrationResponse](t, rr)
	return resp.ClientID, resp.Token
}

// ── Registration ─────────────────────────────────────────────────────────────

func TestHandleRegister_Success(t *testing.T) {
	env := newTestEnv(t)
	rr := postJSON(t, env.HandleAgentRegister, "/api/v1/register", common.RegistrationRequest{
		Platform:           common.PlatformLinux,
		Hostname:           "edge-01",
		RegistrationSecret: "reg-secret-1",
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rr.Code, rr.Body.String())
	}
	resp := decode[common.RegistrationResponse](t, rr)
	if resp.ClientID == "" {
		t.Error("ClientID is empty")
	}
	if resp.Token == "" {
		t.Error("Token is empty")
	}
}

func TestHandleRegister_InvalidSecret(t *testing.T) {
	env := newTestEnv(t)
	rr := postJSON(t, env.HandleAgentRegister, "/api/v1/register", common.RegistrationRequest{
		Platform:           common.PlatformLinux,
		Hostname:           "edge-02",
		RegistrationSecret: "wrong-secret",
	}, "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rr.Code)
	}
}

func TestHandleRegister_MethodNotAllowed(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/register", nil)
	rr := httptest.NewRecorder()

	env.HandleAgentRegister(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d; want 405", rr.Code)
	}
}

func TestHandleRegister_InvalidJSON(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewBufferString("{"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	env.HandleAgentRegister(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestHandleRegister_MissingFields(t *testing.T) {
	env := newTestEnv(t)
	rr := postJSON(t, env.HandleAgentRegister, "/api/v1/register", common.RegistrationRequest{
		RegistrationSecret: "reg-secret-1",
	}, "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rr.Code)
	}
}

func TestHandleRegister_Idempotent(t *testing.T) {
	env := newTestEnv(t)
	req := common.RegistrationRequest{
		Platform:           common.PlatformLinux,
		Hostname:           "edge-03",
		Metadata:           map[string]string{"machine_id": "machine-003"},
		RegistrationSecret: "reg-secret-1",
	}
	rr1 := postJSON(t, env.HandleAgentRegister, "/api/v1/register", req, "")
	rr2 := postJSON(t, env.HandleAgentRegister, "/api/v1/register", req, "")

	r1 := decode[common.RegistrationResponse](t, rr1)
	r2 := decode[common.RegistrationResponse](t, rr2)

	if r1.ClientID != r2.ClientID {
		t.Errorf("second registration should reuse client ID: %q != %q", r1.ClientID, r2.ClientID)
	}
	if len(r1.ClientID) != 26 {
		t.Errorf("client_id %q is not a 26-char ULID", r1.ClientID)
	}
}

func TestHandleRegister_ClientIDIsULID(t *testing.T) {
	env := newTestEnv(t)
	rr := postJSON(t, env.HandleAgentRegister, "/api/v1/register", common.RegistrationRequest{
		Platform: common.PlatformLinux,
		Hostname: "edge-03b",
		Metadata: map[string]string{
			"machine_id": "HPE ProLiant/Gen10 SN 1234",
		},
		RegistrationSecret: "reg-secret-1",
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rr.Code, rr.Body.String())
	}
	resp := decode[common.RegistrationResponse](t, rr)
	if len(resp.ClientID) != 26 {
		t.Errorf("client_id %q is not a 26-char ULID", resp.ClientID)
	}
}

func TestHandleRegister_CapturesClientIP(t *testing.T) {
	env := newTestEnv(t)

	body, err := json.Marshal(common.RegistrationRequest{
		Platform:           common.PlatformLinux,
		Hostname:           "edge-04",
		RegistrationSecret: "reg-secret-1",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.1")
	rr := httptest.NewRecorder()

	env.HandleAgentRegister(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rr.Code, rr.Body.String())
	}

	resp := decode[common.RegistrationResponse](t, rr)
	client, ok := env.Store.GetClient(resp.ClientID)
	if !ok {
		t.Fatalf("client %q not found", resp.ClientID)
	}
	if client.IPAddress != "203.0.113.10" {
		t.Errorf("ip_address = %q; want %q", client.IPAddress, "203.0.113.10")
	}
}

func TestHandleRegister_CapturesClientOS(t *testing.T) {
	env := newTestEnv(t)

	body, err := json.Marshal(common.RegistrationRequest{
		Platform:           common.PlatformLinux,
		Model:              "PowerEdge R650",
		Vendor:             "Dell Inc.",
		Hostname:           "edge-05",
		RegistrationSecret: "reg-secret-1",
		Metadata: map[string]string{
			"os": "Debian GNU/Linux 12 (bookworm)",
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	env.HandleAgentRegister(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rr.Code, rr.Body.String())
	}

	resp := decode[common.RegistrationResponse](t, rr)
	client, ok := env.Store.GetClient(resp.ClientID)
	if !ok {
		t.Fatalf("client %q not found", resp.ClientID)
	}
	if client.OS != "Debian GNU/Linux 12 (bookworm)" {
		t.Errorf("os = %q; want %q", client.OS, "Debian GNU/Linux 12 (bookworm)")
	}
	if client.Model != "PowerEdge R650" {
		t.Errorf("model = %q; want %q", client.Model, "PowerEdge R650")
	}
	if client.Vendor != "Dell Inc." {
		t.Errorf("vendor = %q; want %q", client.Vendor, "Dell Inc.")
	}
}

// ── Auth middleware ───────────────────────────────────────────────────────────

func TestRequireAgentAuth_Missing(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	rr := httptest.NewRecorder()
	env.RequireAgentAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rr.Code)
	}
}

func TestRequireAgentAuth_QueryToken(t *testing.T) {
	env := newTestEnv(t)
	clientID, token := registerAgent(t, env, "SN-QUERY-01", "query-client")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws?token="+token, nil)
	rr := httptest.NewRecorder()

	env.RequireAgentAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Client-ID"); got != clientID {
			t.Fatalf("X-Client-ID = %q; want %q", got, clientID)
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rr.Code)
	}
}

func TestHandleWS_UnauthorizedWithoutToken(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	rr := httptest.NewRecorder()

	env.HandleAgentWS(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rr.Code)
	}
}

func TestRequireRootAuth_WrongPassword(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.SetBasicAuth("root", "wrong")
	rr := httptest.NewRecorder()
	env.RequireRootAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rr.Code)
	}
}

func TestRequireRootAuth_Correct(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.SetBasicAuth("root", "admin123")
	rr := httptest.NewRecorder()
	env.RequireRootAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", rr.Code)
	}
}

// ── Status ────────────────────────────────────────────────────────────────────

func TestHandleStatus(t *testing.T) {
	env := newTestEnv(t)
	registerAgent(t, env, "SN-050", "host-50")

	rr := getRequest(t, env.HandleStatus, "/status", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	resp := decode[handlers.StatusResponse](t, rr)
	if len(resp.Clients) != 1 {
		t.Errorf("clients = %d; want 1", len(resp.Clients))
	}
}

func TestHandleUIPages(t *testing.T) {
	env := newTestEnv(t)

	tests := []struct {
		name    string
		path    string
		handler http.HandlerFunc
	}{
		{name: "status ui", path: "/ui", handler: env.HandleAgentsUI},
		{name: "secrets ui", path: "/ui/secrets", handler: env.HandleSecretsUI},
		{name: "playbooks ui", path: "/ui/playbooks", handler: env.HandlePlaybooksUI},
		{name: "deployments ui", path: "/ui/deployments", handler: env.HandleDeploymentsUI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := getRequest(t, tt.handler, tt.path, "")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d; want 200", rr.Code)
			}
			if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
				t.Fatalf("content-type = %q; expected text/html", ct)
			}
		})
	}
}

// ── Root playbooks ───────────────────────────────────────────────────────────────────

func TestHandleRootPlaybooks_CRUD(t *testing.T) {
	env := newTestEnv(t)

	// Create.
	body := store.PlaybookRecord{
		Name: "my-playbook",
		Playbook: &common.Playbook{
			Name: "deploy",
			Jobs: []common.Job{
				{Name: "j1", Steps: []common.Step{{Name: "s1", Run: "ls"}}},
			},
		},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.HandleRootPlaybooks(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created store.PlaybookRecord
	_ = json.NewDecoder(rr.Body).Decode(&created)
	if created.ID == "" {
		t.Error("created ID is empty")
	}

	// List.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/playbooks", nil)
	rr2 := httptest.NewRecorder()
	env.HandleRootPlaybooks(rr2, req2)
	var list []*store.PlaybookRecord
	_ = json.NewDecoder(rr2.Body).Decode(&list)
	if len(list) != 1 {
		t.Errorf("list len = %d; want 1", len(list))
	}

	// Get by ID.
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/playbooks/"+created.ID, nil)
	req3.URL.Path = "/api/v1/playbooks/" + created.ID
	rr3 := httptest.NewRecorder()
	env.HandleRootPlaybooks(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("get: status=%d", rr3.Code)
	}

	// Delete.
	req4 := httptest.NewRequest(http.MethodDelete, "/api/v1/playbooks/"+created.ID, nil)
	req4.URL.Path = "/api/v1/playbooks/" + created.ID
	rr4 := httptest.NewRecorder()
	env.HandleRootPlaybooks(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Errorf("delete: status=%d", rr4.Code)
	}
}

func TestHandleRootPlaybooks_MethodNotAllowed(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/playbooks", nil)
	rr := httptest.NewRecorder()

	env.HandleRootPlaybooks(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d; want 405", rr.Code)
	}
}

func TestHandleRootPlaybooks_CreateInvalidPayload(t *testing.T) {
	env := newTestEnv(t)

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks", bytes.NewBufferString("{"))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d; want 400 (body=%s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("missing jobs", func(t *testing.T) {
		b, _ := json.Marshal(map[string]any{
			"name": "empty",
			"playbook": map[string]any{
				"name": "empty",
				"jobs": []any{},
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d; want 400 (body=%s)", rr.Code, rr.Body.String())
		}
	})
}

func TestHandleRootPlaybooks_Assign(t *testing.T) {
	env := newTestEnv(t)
	now := time.Now()

	if err := env.Store.SaveClient(&common.Client{
		ID:             "client-assign-1",
		Hostname:       "edge-assign",
		Platform:       common.PlatformLinux,
		Status:         common.ClientStatusRegistered,
		RegisteredAt:   now,
		LastActivityAt: now,
	}); err != nil {
		t.Fatalf("SaveClient: %v", err)
	}
	if err := env.Store.SavePlaybook(&store.PlaybookRecord{
		ID:   "pb-assign-1",
		Name: "assignable",
		Playbook: &common.Playbook{
			Name: "assignable",
			Jobs: []common.Job{{Name: "j1", Steps: []common.Step{{Name: "s1", Run: "echo ok"}}}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SavePlaybook: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		b, _ := json.Marshal(map[string]string{"client_id": "client-assign-1"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks/pb-assign-1/assign", bytes.NewReader(b))
		req.URL.Path = "/api/v1/playbooks/pb-assign-1/assign"
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}

		c, ok := env.Store.GetClient("client-assign-1")
		if !ok {
			t.Fatal("client not found")
		}
		if c.PlaybookID != "pb-assign-1" {
			t.Fatalf("PlaybookID = %q; want pb-assign-1", c.PlaybookID)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks/pb-assign-1/assign", bytes.NewBufferString("{"))
		req.URL.Path = "/api/v1/playbooks/pb-assign-1/assign"
		rr := httptest.NewRecorder()

		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d; want 400", rr.Code)
		}
	})

	t.Run("missing client", func(t *testing.T) {
		b, _ := json.Marshal(map[string]string{"client_id": "no-client"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks/pb-assign-1/assign", bytes.NewReader(b))
		req.URL.Path = "/api/v1/playbooks/pb-assign-1/assign"
		rr := httptest.NewRecorder()

		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d; want 404", rr.Code)
		}
	})

	t.Run("missing playbook", func(t *testing.T) {
		b, _ := json.Marshal(map[string]string{"client_id": "client-assign-1"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks/no-playbook/assign", bytes.NewReader(b))
		req.URL.Path = "/api/v1/playbooks/no-playbook/assign"
		rr := httptest.NewRecorder()

		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d; want 404", rr.Code)
		}
	})

	t.Run("service missing", func(t *testing.T) {
		envNoService := newTestEnv(t)
		envNoService.Service = nil
		b, _ := json.Marshal(map[string]string{"client_id": "client-assign-1"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks/pb-assign-1/assign", bytes.NewReader(b))
		req.URL.Path = "/api/v1/playbooks/pb-assign-1/assign"
		rr := httptest.NewRecorder()

		envNoService.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d; want 500", rr.Code)
		}
	})
}

func TestHandleRootPlaybooks_UpdateAndGetErrors(t *testing.T) {
	env := newTestEnv(t)
	now := time.Now()

	if err := env.Store.SavePlaybook(&store.PlaybookRecord{
		ID:   "pb-update-1",
		Name: "update-me",
		Playbook: &common.Playbook{
			Name: "update-me",
			Jobs: []common.Job{{Name: "j1", Steps: []common.Step{{Name: "s1", Run: "echo old"}}}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SavePlaybook: %v", err)
	}

	t.Run("get missing playbook", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/playbooks/missing", nil)
		req.URL.Path = "/api/v1/playbooks/missing"
		rr := httptest.NewRecorder()
		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d; want 404", rr.Code)
		}
	})

	t.Run("put missing id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/playbooks", bytes.NewBufferString(`{"name":"x"}`))
		req.URL.Path = "/api/v1/playbooks"
		rr := httptest.NewRecorder()
		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d; want 400", rr.Code)
		}
	})

	t.Run("put update with yaml payload", func(t *testing.T) {
		body := map[string]any{
			"name":          "updated-name",
			"playbook_yaml": "name: updated-playbook\njobs:\n  - name: j2\n    steps:\n      - name: s2\n        run: echo new\n",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/playbooks/pb-update-1", bytes.NewReader(b))
		req.URL.Path = "/api/v1/playbooks/pb-update-1"
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		env.HandleRootPlaybooks(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
		got, ok := env.Store.GetPlaybook("pb-update-1")
		if !ok {
			t.Fatal("updated playbook not found")
		}
		if got.Name != "updated-name" || got.Playbook == nil || got.Playbook.Name != "updated-playbook" {
			t.Fatalf("unexpected updated playbook: %+v", got)
		}
	})
}

func TestHandleRootClients_CRUDAndErrors(t *testing.T) {
	env := newTestEnv(t)
	now := time.Now()

	if err := env.Store.SaveClient(&common.Client{
		ID:             "client-root-1",
		Hostname:       "edge-root",
		Platform:       common.PlatformLinux,
		Status:         common.ClientStatusRegistered,
		RegisteredAt:   now,
		LastActivityAt: now,
	}); err != nil {
		t.Fatalf("SaveClient: %v", err)
	}

	t.Run("list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clients", nil)
		rr := httptest.NewRecorder()
		env.HandleRootClients(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
		var clients []*common.Client
		_ = json.NewDecoder(rr.Body).Decode(&clients)
		if len(clients) != 1 {
			t.Fatalf("clients len=%d; want 1", len(clients))
		}
	})

	t.Run("get one", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clients/client-root-1", nil)
		req.URL.Path = "/api/v1/clients/client-root-1"
		rr := httptest.NewRecorder()
		env.HandleRootClients(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
	})

	t.Run("get missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clients/missing", nil)
		req.URL.Path = "/api/v1/clients/missing"
		rr := httptest.NewRecorder()
		env.HandleRootClients(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d; want 404", rr.Code)
		}
	})

	t.Run("put missing id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/clients", bytes.NewBufferString(`{"status":"connected"}`))
		req.URL.Path = "/api/v1/clients"
		rr := httptest.NewRecorder()
		env.HandleRootClients(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d; want 400", rr.Code)
		}
	})

	t.Run("put invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/clients/client-root-1", bytes.NewBufferString("{"))
		req.URL.Path = "/api/v1/clients/client-root-1"
		rr := httptest.NewRecorder()
		env.HandleRootClients(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d; want 400", rr.Code)
		}
	})

	t.Run("put success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/clients/client-root-1", bytes.NewBufferString(`{"playbook_id":"pb-1","status":"deploying"}`))
		req.URL.Path = "/api/v1/clients/client-root-1"
		rr := httptest.NewRecorder()
		env.HandleRootClients(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
		c, ok := env.Store.GetClient("client-root-1")
		if !ok {
			t.Fatal("client not found")
		}
		if c.PlaybookID != "pb-1" || c.Status != common.ClientStatusDeploying {
			t.Fatalf("unexpected update: playbook=%q status=%q", c.PlaybookID, c.Status)
		}
	})

	t.Run("delete missing id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/clients", nil)
		req.URL.Path = "/api/v1/clients"
		rr := httptest.NewRecorder()
		env.HandleRootClients(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d; want 400", rr.Code)
		}
	})

	t.Run("delete success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/clients/client-root-1", nil)
		req.URL.Path = "/api/v1/clients/client-root-1"
		rr := httptest.NewRecorder()
		env.HandleRootClients(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
		if _, ok := env.Store.GetClient("client-root-1"); ok {
			t.Fatal("client should be deleted")
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/clients", nil)
		req.URL.Path = "/api/v1/clients"
		rr := httptest.NewRecorder()
		env.HandleRootClients(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d; want 405", rr.Code)
		}
	})
}

// ── Secrets ───────────────────────────────────────────────────────────────────

func TestHandleSecrets_CRUD(t *testing.T) {
	env := newTestEnv(t)

	// Set.
	b, _ := json.Marshal(map[string]string{"value": "s3cr3t"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/secrets/MY_SECRET", bytes.NewReader(b))
	req.URL.Path = "/api/v1/secrets/MY_SECRET"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.HandleSecrets(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set: status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Get.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/MY_SECRET", nil)
	req2.URL.Path = "/api/v1/secrets/MY_SECRET"
	rr2 := httptest.NewRecorder()
	env.HandleSecrets(rr2, req2)
	var got map[string]string
	_ = json.NewDecoder(rr2.Body).Decode(&got)
	if got["value"] != "s3cr3t" {
		t.Errorf("value = %q; want s3cr3t", got["value"])
	}

	// List names (values must be redacted).
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/secrets", nil)
	req3.URL.Path = "/api/v1/secrets"
	rr3 := httptest.NewRecorder()
	env.HandleSecrets(rr3, req3)
	var names []string
	_ = json.NewDecoder(rr3.Body).Decode(&names)
	if len(names) != 1 || names[0] != "MY_SECRET" {
		t.Errorf("names = %v; want [MY_SECRET]", names)
	}

	// Delete.
	req4 := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/MY_SECRET", nil)
	req4.URL.Path = "/api/v1/secrets/MY_SECRET"
	rr4 := httptest.NewRecorder()
	env.HandleSecrets(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Errorf("delete: status=%d", rr4.Code)
	}
}

func TestHandleSecrets_ErrorPaths(t *testing.T) {
	env := newTestEnv(t)

	t.Run("get missing secret", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/DOES_NOT_EXIST", nil)
		req.URL.Path = "/api/v1/secrets/DOES_NOT_EXIST"
		rr := httptest.NewRecorder()

		env.HandleSecrets(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d; want 404", rr.Code)
		}
	})

	t.Run("put without name", func(t *testing.T) {
		b, _ := json.Marshal(map[string]string{"value": "x"})
		req := httptest.NewRequest(http.MethodPut, "/api/v1/secrets", bytes.NewReader(b))
		req.URL.Path = "/api/v1/secrets"
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		env.HandleSecrets(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d; want 400", rr.Code)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", nil)
		req.URL.Path = "/api/v1/secrets"
		rr := httptest.NewRecorder()

		env.HandleSecrets(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d; want 405", rr.Code)
		}
	})
}

func TestHandleRootDeployments_MethodNotAllowed(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deployments", nil)
	rr := httptest.NewRecorder()

	env.HandleRootDeployments(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d; want 405", rr.Code)
	}
}

func TestHandleRootDeployments_ListGetAndNotFound(t *testing.T) {
	env := newTestEnv(t)
	now := time.Now()
	_ = env.Store.SaveDeployment(&common.DeploymentState{
		ID:         "dep-list-1",
		ClientID:   "client-1",
		PlaybookID: "pb-1",
		Status:     common.DeploymentStatusRunning,
		StartedAt:  now,
		UpdatedAt:  now,
	})

	t.Run("list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments", nil)
		rr := httptest.NewRecorder()
		env.HandleRootDeployments(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
		var deps []*common.DeploymentState
		_ = json.NewDecoder(rr.Body).Decode(&deps)
		if len(deps) != 1 {
			t.Fatalf("len(deps)=%d; want 1", len(deps))
		}
	})

	t.Run("get by id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-list-1", nil)
		req.URL.Path = "/api/v1/deployments/dep-list-1"
		rr := httptest.NewRecorder()
		env.HandleRootDeployments(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
	})

	t.Run("get missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/missing", nil)
		req.URL.Path = "/api/v1/deployments/missing"
		rr := httptest.NewRecorder()
		env.HandleRootDeployments(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d; want 404", rr.Code)
		}
	})
}

// ── Artifacts ────────────────────────────────────────────────────────────────

func TestHandleArtifacts_UploadDownloadMetaDelete(t *testing.T) {
	env := newTestEnv(t)

	uploadReq := httptest.NewRequest(http.MethodPost, "/artifacts?name=myapp.bin", bytes.NewBufferString("hello artifact"))
	uploadReq.SetBasicAuth("root", env.RootPassword)
	uploadRR := httptest.NewRecorder()
	env.HandleArtifacts(uploadRR, uploadReq)
	if uploadRR.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", uploadRR.Code, uploadRR.Body.String())
	}

	created := decode[common.Artifact](t, uploadRR)
	if created.ID == "" {
		t.Fatal("artifact ID should not be empty")
	}
	if created.ContentType != "application/octet-stream" {
		t.Fatalf("default content type = %q; want application/octet-stream", created.ContentType)
	}

	metaReq := httptest.NewRequest(http.MethodGet, "/artifacts/"+created.ID+"/meta", nil)
	metaReq.SetBasicAuth("root", env.RootPassword)
	metaRR := httptest.NewRecorder()
	env.HandleArtifacts(metaRR, metaReq)
	if metaRR.Code != http.StatusOK {
		t.Fatalf("meta status=%d", metaRR.Code)
	}
	meta := decode[common.Artifact](t, metaRR)
	if meta.ID != created.ID {
		t.Fatalf("meta ID=%q; want %q", meta.ID, created.ID)
	}

	// Download by artifact name validates fallback lookup (GetArtifactByName).
	downloadReq := httptest.NewRequest(http.MethodGet, "/artifacts/myapp.bin", nil)
	downloadReq.SetBasicAuth("root", env.RootPassword)
	downloadRR := httptest.NewRecorder()
	env.HandleArtifacts(downloadRR, downloadReq)
	if downloadRR.Code != http.StatusOK {
		t.Fatalf("download status=%d body=%s", downloadRR.Code, downloadRR.Body.String())
	}
	if downloadRR.Body.String() != "hello artifact" {
		t.Fatalf("download body=%q; want %q", downloadRR.Body.String(), "hello artifact")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/artifacts/"+created.ID, nil)
	deleteReq.SetBasicAuth("root", env.RootPassword)
	deleteRR := httptest.NewRecorder()
	env.HandleArtifacts(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("delete status=%d", deleteRR.Code)
	}
}

func TestHandleArtifacts_ErrorPaths(t *testing.T) {
	env := newTestEnv(t)

	t.Run("upload missing name", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/artifacts", bytes.NewBufferString("x"))
		req.SetBasicAuth("root", env.RootPassword)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d; want 400", rr.Code)
		}
	})

	t.Run("get missing artifact", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/artifacts/missing", nil)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d; want 404", rr.Code)
		}
	})

	t.Run("meta missing artifact", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/artifacts/missing/meta", nil)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d; want 404", rr.Code)
		}
	})

	t.Run("download missing file on disk", func(t *testing.T) {
		art := &common.Artifact{ID: "art-missing-file", Name: "ghost", Filename: "ghost.bin"}
		if err := env.Store.SaveArtifact(art); err != nil {
			t.Fatalf("SaveArtifact: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/artifacts/art-missing-file", nil)
		req.SetBasicAuth("root", env.RootPassword)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d; want 500", rr.Code)
		}
	})

	t.Run("delete without id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/artifacts", nil)
		req.SetBasicAuth("root", env.RootPassword)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d; want 400", rr.Code)
		}
	})

	t.Run("delete missing artifact", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/artifacts/not-found", nil)
		req.SetBasicAuth("root", env.RootPassword)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d; want 404", rr.Code)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/artifacts", nil)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d; want 405", rr.Code)
		}
	})
}

func TestHandleArtifacts_AccessPolicy(t *testing.T) {
	env := newTestEnv(t)
	clientID, token := registerAgent(t, env, "client-1", "host-1")
	_, otherToken := registerAgent(t, env, "client-2", "host-2")

	// 1. Upload Public Artifact
	pubReq := httptest.NewRequest(http.MethodPost, "/artifacts?name=pub.bin&access_policy=public", bytes.NewBufferString("public content"))
	pubReq.SetBasicAuth("root", env.RootPassword)
	pubRR := httptest.NewRecorder()
	env.HandleArtifacts(pubRR, pubReq)
	pubArt := decode[common.Artifact](t, pubRR)

	// 2. Upload Authenticated Artifact
	authReq := httptest.NewRequest(http.MethodPost, "/artifacts?name=auth.bin&access_policy=authenticated", bytes.NewBufferString("auth content"))
	authReq.SetBasicAuth("root", env.RootPassword)
	authRR := httptest.NewRecorder()
	env.HandleArtifacts(authRR, authReq)
	authArt := decode[common.Artifact](t, authRR)

	// 3. Upload Restricted Artifact for client-1 (use the actual ULID assigned at registration)
	restReq := httptest.NewRequest(http.MethodPost, "/artifacts?name=rest.bin&access_policy=restricted&allowed_clients="+clientID, bytes.NewBufferString("rest content"))
	restReq.SetBasicAuth("root", env.RootPassword)
	restRR := httptest.NewRecorder()
	env.HandleArtifacts(restRR, restReq)
	restArt := decode[common.Artifact](t, restRR)

	t.Run("public download no auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+pubArt.ID, nil)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d; want 200", rr.Code)
		}
	})

	t.Run("auth download no auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+authArt.ID, nil)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d; want 401", rr.Code)
		}
	})

	t.Run("auth download with auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+authArt.ID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d; want 200", rr.Code)
		}
	})

	t.Run("restricted download other client", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+restArt.ID, nil)
		req.Header.Set("Authorization", "Bearer "+otherToken)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status=%d; want 403", rr.Code)
		}
	})

	t.Run("restricted download allowed client", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+restArt.ID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d; want 200", rr.Code)
		}
	})

	t.Run("restricted download root", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+restArt.ID, nil)
		req.SetBasicAuth("root", env.RootPassword)
		rr := httptest.NewRecorder()
		env.HandleArtifacts(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d; want 200", rr.Code)
		}
	})
}

func TestHandleArtifacts_ModifyPolicy(t *testing.T) {
	env := newTestEnv(t)
	_, token := registerAgent(t, env, "client-1", "host-1")

	// 1. Upload Authenticated Artifact
	req := httptest.NewRequest(http.MethodPost, "/artifacts?name=mod.bin&access_policy=authenticated", bytes.NewBufferString("content"))
	req.SetBasicAuth("root", env.RootPassword)
	rr := httptest.NewRecorder()
	env.HandleArtifacts(rr, req)
	art := decode[common.Artifact](t, rr)

	// 2. Verify it's authenticated (cannot download without auth)
	dlReq := httptest.NewRequest(http.MethodGet, "/artifacts/"+art.ID, nil)
	dlRR := httptest.NewRecorder()
	env.HandleArtifacts(dlRR, dlReq)
	if dlRR.Code != http.StatusUnauthorized {
		t.Fatalf("initial dl: status=%d; want 401", dlRR.Code)
	}

	// 3. Modify to Public
	modReq := httptest.NewRequest(http.MethodPatch, "/artifacts/"+art.ID+"?access_policy=public", nil)
	modReq.SetBasicAuth("root", env.RootPassword)
	modRR := httptest.NewRecorder()
	env.HandleArtifacts(modRR, modReq)
	if modRR.Code != http.StatusOK {
		t.Fatalf("patch status=%d; body=%s", modRR.Code, modRR.Body.String())
	}

	// 4. Verify it's now public
	pubRR := httptest.NewRecorder()
	env.HandleArtifacts(pubRR, dlReq) // reused dlReq
	if pubRR.Code != http.StatusOK {
		t.Fatalf("after patch dl: status=%d; want 200", pubRR.Code)
	}

	// 5. Modify to Restricted for client-2
	modReq2 := httptest.NewRequest(http.MethodPatch, "/artifacts/"+art.ID+"?access_policy=restricted&allowed_clients=client-2", nil)
	modReq2.SetBasicAuth("root", env.RootPassword)
	modRR2 := httptest.NewRecorder()
	env.HandleArtifacts(modRR2, modReq2)
	if modRR2.Code != http.StatusOK {
		t.Fatalf("patch2 status=%d", modRR2.Code)
	}

	// 6. Verify client-1 (from token) cannot download it
	c1Req := httptest.NewRequest(http.MethodGet, "/artifacts/"+art.ID, nil)
	c1Req.Header.Set("Authorization", "Bearer "+token)
	c1RR := httptest.NewRecorder()
	env.HandleArtifacts(c1RR, c1Req)
	if c1RR.Code != http.StatusForbidden {
		t.Fatalf("restricted c1: status=%d; want 403", c1RR.Code)
	}
}

// ── Cross-client security ─────────────────────────────────────────────────────

func TestCrossClientDeploymentForbidden(t *testing.T) {
	env := newTestEnv(t)
	ownerID, _ := registerAgent(t, env, "SN-031", "host-31")
	_, _ = registerAgent(t, env, "SN-032", "host-32")

	// Create a deployment owned by ownerID.
	dep := &common.DeploymentState{
		ID:        "dep-owned",
		ClientID:  ownerID,
		Status:    common.DeploymentStatusRunning,
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = env.Store.SaveDeployment(dep)

	// Root reading logs for someone else's deployment should work.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-owned/logs", nil)
	req.URL.Path = "/api/v1/deployments/dep-owned/logs"
	rr := httptest.NewRecorder()
	env.HandleRootDeployments(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("root logs: status=%d; want 200", rr.Code)
	}
}

// ── Playbook job ordering ─────────────────────────────────────────────────────

func TestPlaybookJobOrderPreserved(t *testing.T) {
	// Jobs must execute in the order they are declared, not alphabetically.
	pb := &common.Playbook{
		Name: "ordered",
		Jobs: []common.Job{
			{Name: "zzz-last", Steps: []common.Step{{Name: "s", Run: "echo last"}}},
			{Name: "aaa-first", Steps: []common.Step{{Name: "s", Run: "echo first"}}},
		},
	}
	flat := common.FlattenPlaybook(pb)
	if flat[0].JobName != "zzz-last" {
		t.Errorf("first job = %q; want zzz-last (order must be preserved)", flat[0].JobName)
	}
	if flat[1].JobName != "aaa-first" {
		t.Errorf("second job = %q; want aaa-first", flat[1].JobName)
	}
}
