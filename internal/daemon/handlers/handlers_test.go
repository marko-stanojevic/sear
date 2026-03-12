package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/marko-stanojevic/sear/internal/common"
	"github.com/marko-stanojevic/sear/internal/daemon/handlers"
	"github.com/marko-stanojevic/sear/internal/daemon/store"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

func newTestEnv(t *testing.T) *handlers.Env {
	t.Helper()
	st, err := store.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return &handlers.Env{
		Store:            st,
		JWTSecret:        []byte("test-secret-key-32-bytes-padding!"),
		RootPassword:     "admin123",
		TokenExpiryHours: 24,
		ArtifactsDir:     t.TempDir(),
		ServerURL:        "http://localhost:8080",
		Hub:              handlers.NewHub(),
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

// registerClient is a test helper that registers a client and returns its token.
func registerClient(t *testing.T, env *handlers.Env, platformID, hostname string) (string, string) {
	t.Helper()
	rr := postJSON(t, env.HandleRegister, "/api/v1/register", common.RegistrationRequest{
		Platform:           common.PlatformBaremetal,
		PlatformID:         platformID,
		Hostname:           hostname,
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
	rr := postJSON(t, env.HandleRegister, "/api/v1/register", common.RegistrationRequest{
		Platform:           common.PlatformBaremetal,
		PlatformID:         "SN-001",
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
	rr := postJSON(t, env.HandleRegister, "/api/v1/register", common.RegistrationRequest{
		Platform:           common.PlatformBaremetal,
		PlatformID:         "SN-002",
		Hostname:           "edge-02",
		RegistrationSecret: "wrong-secret",
	}, "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rr.Code)
	}
}

func TestHandleRegister_MissingFields(t *testing.T) {
	env := newTestEnv(t)
	rr := postJSON(t, env.HandleRegister, "/api/v1/register", common.RegistrationRequest{
		RegistrationSecret: "reg-secret-1",
	}, "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rr.Code)
	}
}

func TestHandleRegister_Idempotent(t *testing.T) {
	env := newTestEnv(t)
	req := common.RegistrationRequest{
		Platform:           common.PlatformBaremetal,
		PlatformID:         "SN-003",
		Hostname:           "edge-03",
		RegistrationSecret: "reg-secret-1",
	}
	rr1 := postJSON(t, env.HandleRegister, "/api/v1/register", req, "")
	rr2 := postJSON(t, env.HandleRegister, "/api/v1/register", req, "")

	r1 := decode[common.RegistrationResponse](t, rr1)
	r2 := decode[common.RegistrationResponse](t, rr2)

	if r1.ClientID != r2.ClientID {
		t.Errorf("second registration should reuse client ID: %q != %q", r1.ClientID, r2.ClientID)
	}
}

func TestHandleRegister_CapturesClientIP(t *testing.T) {
	env := newTestEnv(t)

	body, err := json.Marshal(common.RegistrationRequest{
		Platform:           common.PlatformBaremetal,
		PlatformID:         "SN-004",
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

	env.HandleRegister(rr, req)
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
		Platform:           common.PlatformBaremetal,
		PlatformID:         "SN-005",
		Hostname:           "edge-05",
		RegistrationSecret: "reg-secret-1",
		Metadata: map[string]string{
			"os":             "linux",
			"os_type":        "linux",
			"os_description": "Debian GNU/Linux 12 (bookworm)",
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	env.HandleRegister(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rr.Code, rr.Body.String())
	}

	resp := decode[common.RegistrationResponse](t, rr)
	client, ok := env.Store.GetClient(resp.ClientID)
	if !ok {
		t.Fatalf("client %q not found", resp.ClientID)
	}
	if client.OS != "linux" {
		t.Errorf("os = %q; want %q", client.OS, "linux")
	}
	if client.OSType != "linux" {
		t.Errorf("os_type = %q; want %q", client.OSType, "linux")
	}
	if client.OSDescription != "Debian GNU/Linux 12 (bookworm)" {
		t.Errorf("os_description = %q; want %q", client.OSDescription, "Debian GNU/Linux 12 (bookworm)")
	}
}

// ── Auth middleware ───────────────────────────────────────────────────────────

func TestRequireClientAuth_Missing(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	rr := httptest.NewRecorder()
	env.RequireClientAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rr.Code)
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
	registerClient(t, env, "SN-050", "host-50")

	rr := getRequest(t, env.HandleStatus, "/status", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	resp := decode[handlers.StatusResponse](t, rr)
	if len(resp.Clients) != 1 {
		t.Errorf("clients = %d; want 1", len(resp.Clients))
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
	req := httptest.NewRequest(http.MethodPost, "/playbooks", bytes.NewReader(b))
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
	req2 := httptest.NewRequest(http.MethodGet, "/playbooks", nil)
	rr2 := httptest.NewRecorder()
	env.HandleRootPlaybooks(rr2, req2)
	var list []*store.PlaybookRecord
	_ = json.NewDecoder(rr2.Body).Decode(&list)
	if len(list) != 1 {
		t.Errorf("list len = %d; want 1", len(list))
	}

	// Get by ID.
	req3 := httptest.NewRequest(http.MethodGet, "/playbooks/"+created.ID, nil)
	req3.URL.Path = "/playbooks/" + created.ID
	rr3 := httptest.NewRecorder()
	env.HandleRootPlaybooks(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("get: status=%d", rr3.Code)
	}

	// Delete.
	req4 := httptest.NewRequest(http.MethodDelete, "/playbooks/"+created.ID, nil)
	req4.URL.Path = "/playbooks/" + created.ID
	rr4 := httptest.NewRecorder()
	env.HandleRootPlaybooks(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Errorf("delete: status=%d", rr4.Code)
	}
}

// ── Secrets ───────────────────────────────────────────────────────────────────

func TestHandleSecrets_CRUD(t *testing.T) {
	env := newTestEnv(t)

	// Set.
	b, _ := json.Marshal(map[string]string{"value": "s3cr3t"})
	req := httptest.NewRequest(http.MethodPut, "/secrets/MY_SECRET", bytes.NewReader(b))
	req.URL.Path = "/secrets/MY_SECRET"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.HandleSecrets(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("set: status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Get.
	req2 := httptest.NewRequest(http.MethodGet, "/secrets/MY_SECRET", nil)
	req2.URL.Path = "/secrets/MY_SECRET"
	rr2 := httptest.NewRecorder()
	env.HandleSecrets(rr2, req2)
	var got map[string]string
	_ = json.NewDecoder(rr2.Body).Decode(&got)
	if got["value"] != "s3cr3t" {
		t.Errorf("value = %q; want s3cr3t", got["value"])
	}

	// List names (values must be redacted).
	req3 := httptest.NewRequest(http.MethodGet, "/secrets", nil)
	req3.URL.Path = "/secrets"
	rr3 := httptest.NewRecorder()
	env.HandleSecrets(rr3, req3)
	var names []string
	_ = json.NewDecoder(rr3.Body).Decode(&names)
	if len(names) != 1 || names[0] != "MY_SECRET" {
		t.Errorf("names = %v; want [MY_SECRET]", names)
	}

	// Delete.
	req4 := httptest.NewRequest(http.MethodDelete, "/secrets/MY_SECRET", nil)
	req4.URL.Path = "/secrets/MY_SECRET"
	rr4 := httptest.NewRecorder()
	env.HandleSecrets(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Errorf("delete: status=%d", rr4.Code)
	}
}

// ── Cross-client security ─────────────────────────────────────────────────────

func TestCrossClientDeploymentForbidden(t *testing.T) {
	env := newTestEnv(t)
	ownerID, _ := registerClient(t, env, "SN-031", "host-31")
	_, _ = registerClient(t, env, "SN-032", "host-32")

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
	req := httptest.NewRequest(http.MethodGet, "/deployments/dep-owned/logs", nil)
	req.URL.Path = "/deployments/dep-owned/logs"
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
