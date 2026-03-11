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

// ---- helpers ---------------------------------------------------------------

func newTestEnv(t *testing.T) *handlers.Env {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return &handlers.Env{
		Store:        st,
		JWTSecret:    []byte("test-secret-key"),
		RootPassword: "admin123",
		TokenExpiryHours: 24,
		ArtifactsDir: t.TempDir(),
		ServerURL:    "http://localhost:8080",
		RegistrationSecrets: map[string]string{
			"prod": "reg-secret-1",
		},
	}
}

func postJSON(t *testing.T, handler http.HandlerFunc, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func getJSON(t *testing.T, handler http.HandlerFunc, path string, token string) *httptest.ResponseRecorder {
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

// ---- Registration ----------------------------------------------------------

func TestHandleRegister_Success(t *testing.T) {
	env := newTestEnv(t)
	rr := postJSON(t, env.HandleRegister, "/api/v1/register", common.RegistrationRequest{
		Platform:           common.PlatformBaremetal,
		PlatformID:         "SN-001",
		Hostname:           "edge-01",
		RegistrationSecret: "reg-secret-1",
	}, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body: %s)", rr.Code, rr.Body.String())
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

// ---- Auth middleware --------------------------------------------------------

func TestRequireClientAuth_Missing(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/connect", nil)
	rr := httptest.NewRecorder()
	env.RequireClientAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rr.Code)
	}
}

func TestRequireAdminAuth_WrongPassword(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.SetBasicAuth("admin", "wrong")
	rr := httptest.NewRecorder()
	env.RequireAdminAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rr.Code)
	}
}

func TestRequireAdminAuth_CorrectPassword(t *testing.T) {
	env := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.SetBasicAuth("admin", "admin123")
	rr := httptest.NewRecorder()
	env.RequireAdminAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", rr.Code)
	}
}

// ---- Connect + State -------------------------------------------------------

// registerClient is a helper that registers a client and returns its token.
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

func TestHandleConnect_NoPlaybook(t *testing.T) {
	env := newTestEnv(t)
	clientID, token := registerClient(t, env, "SN-010", "host-10")
	_ = clientID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/connect", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Client-ID", clientID) // middleware would set this
	rr := httptest.NewRecorder()
	env.HandleConnect(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}
	resp := decode[common.ConnectResponse](t, rr)
	if resp.Action != "wait" {
		t.Errorf("Action = %q; want wait", resp.Action)
	}
}

func TestHandleConnect_WithPlaybook(t *testing.T) {
	env := newTestEnv(t)
	clientID, token := registerClient(t, env, "SN-020", "host-20")

	// Create a playbook.
	pb := &store.PlaybookRecord{
		ID:   "pb-test",
		Name: "test",
		Playbook: &common.Playbook{
			Name: "test",
			Jobs: map[string]common.Job{
				"job1": {Steps: []common.Step{{Name: "s1", Run: "echo hi"}}},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = env.Store.SavePlaybook(pb)

	// Assign playbook to client.
	c, _ := env.Store.GetClient(clientID)
	c.PlaybookID = "pb-test"
	_ = env.Store.SaveClient(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/connect", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Client-ID", clientID)
	rr := httptest.NewRecorder()
	env.HandleConnect(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}
	resp := decode[common.ConnectResponse](t, rr)
	if resp.Action != "deploy" {
		t.Errorf("Action = %q; want deploy", resp.Action)
	}
	if resp.Playbook == nil {
		t.Error("Playbook is nil")
	}
	if resp.DeploymentID == "" {
		t.Error("DeploymentID is empty")
	}
}

func TestHandleStateUpdate(t *testing.T) {
	env := newTestEnv(t)
	clientID, token := registerClient(t, env, "SN-030", "host-30")

	// Create a deployment manually.
	dep := &common.DeploymentState{
		ID:        "dep-test",
		ClientID:  clientID,
		Status:    common.DeploymentStatusRunning,
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = env.Store.SaveDeployment(dep)

	rr := postJSON(t, env.HandleStateUpdate, "/api/v1/state", common.StateUpdateRequest{
		DeploymentID:     "dep-test",
		Status:           common.DeploymentStatusDone,
		CurrentJobName:   "setup",
		CurrentStepIndex: 3,
	}, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}

	updated, _ := env.Store.GetDeployment("dep-test")
	if updated.Status != common.DeploymentStatusDone {
		t.Errorf("status = %q; want done", updated.Status)
	}
	if updated.FinishedAt == nil {
		t.Error("FinishedAt should be set")
	}
}

// ---- Log upload ------------------------------------------------------------

func TestHandleLogUpload(t *testing.T) {
	env := newTestEnv(t)
	_, token := registerClient(t, env, "SN-040", "host-40")

	batch := common.LogBatch{
		Entries: []common.LogEntry{
			{DeploymentID: "dep-1", Level: common.LogLevelInfo, Message: "hello", Timestamp: time.Now()},
			{DeploymentID: "dep-1", Level: common.LogLevelError, Message: "oops", Timestamp: time.Now()},
		},
	}
	rr := postJSON(t, env.HandleLogUpload, "/api/v1/logs", batch, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}

	logs := env.Store.GetLogsForDeployment("dep-1")
	if len(logs) != 2 {
		t.Errorf("log count = %d; want 2", len(logs))
	}
}

// ---- Status ----------------------------------------------------------------

func TestHandleStatus(t *testing.T) {
	env := newTestEnv(t)
	registerClient(t, env, "SN-050", "host-50")

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rr := httptest.NewRecorder()
	env.HandleStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	resp := decode[handlers.StatusResponse](t, rr)
	if len(resp.Clients) != 1 {
		t.Errorf("clients = %d; want 1", len(resp.Clients))
	}
}

// ---- Admin playbooks -------------------------------------------------------

func TestHandleAdminPlaybooks_CRUD(t *testing.T) {
	env := newTestEnv(t)

	// Create.
	body := store.PlaybookRecord{
		Name: "my-playbook",
		Playbook: &common.Playbook{
			Name: "deploy",
			Jobs: map[string]common.Job{
				"j1": {Steps: []common.Step{{Name: "s1", Run: "ls"}}},
			},
		},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/admin/playbooks", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.HandleAdminPlaybooks(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created store.PlaybookRecord
	_ = json.NewDecoder(rr.Body).Decode(&created)
	if created.ID == "" {
		t.Error("created ID is empty")
	}

	// List.
	req2 := httptest.NewRequest(http.MethodGet, "/admin/playbooks", nil)
	rr2 := httptest.NewRecorder()
	env.HandleAdminPlaybooks(rr2, req2)
	var list []*store.PlaybookRecord
	_ = json.NewDecoder(rr2.Body).Decode(&list)
	if len(list) != 1 {
		t.Errorf("list len = %d; want 1", len(list))
	}

	// Get by ID.
	req3 := httptest.NewRequest(http.MethodGet, "/admin/playbooks/"+created.ID, nil)
	req3.URL.Path = "/admin/playbooks/" + created.ID
	rr3 := httptest.NewRecorder()
	env.HandleAdminPlaybooks(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("get: status=%d", rr3.Code)
	}

	// Delete.
	req4 := httptest.NewRequest(http.MethodDelete, "/admin/playbooks/"+created.ID, nil)
	req4.URL.Path = "/admin/playbooks/" + created.ID
	rr4 := httptest.NewRecorder()
	env.HandleAdminPlaybooks(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Errorf("delete: status=%d", rr4.Code)
	}
}

// ---- Secrets ---------------------------------------------------------------

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
	if rr2.Code != http.StatusOK {
		t.Fatalf("get: status=%d", rr2.Code)
	}
	var got map[string]string
	_ = json.NewDecoder(rr2.Body).Decode(&got)
	if got["value"] != "s3cr3t" {
		t.Errorf("value = %q; want s3cr3t", got["value"])
	}

	// List names.
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
