// Package store provides a JSON-file-backed persistence layer for the sear daemon.
// Design notes:
//   - All client/deployment/playbook/artifact/secret state lives in state.json.
//   - Logs are stored in per-deployment files under logsDir/{deploymentID}.json
//     so that a large deployment never inflates the main state snapshot.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/marko-stanojevic/sear/internal/common"
)

// PlaybookRecord wraps a Playbook with server-side metadata.
type PlaybookRecord struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Playbook    *common.Playbook `json:"playbook"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// Store holds all persistent state for the daemon.
type Store struct {
	mu          sync.RWMutex
	stateFile   string
	logsDir     string
	clients     map[string]*common.Client
	deployments map[string]*common.DeploymentState
	playbooks   map[string]*PlaybookRecord
	artifacts   map[string]*common.Artifact
	secrets     map[string]string // client secrets (name→value)

	// logMutexesMu protects logMutexes; logMutexes holds a per-deployment mutex so that
	// concurrent log reads/writes for the same deployment are serialised without
	// blocking unrelated deployments.
	logMutexesMu sync.Mutex
	logMutexes   map[string]*sync.Mutex
}

// snapshot is the structure serialised to state.json.
// Logs are intentionally excluded — they live in separate files.
type snapshot struct {
	Clients     map[string]*common.Client          `json:"clients"`
	Deployments map[string]*common.DeploymentState `json:"deployments"`
	Playbooks   map[string]*PlaybookRecord         `json:"playbooks"`
	Artifacts   map[string]*common.Artifact        `json:"artifacts"`
	Secrets     map[string]string                  `json:"secrets"`
}

// New creates (or reopens) a store rooted at dir.
// Logs are stored under logsDir; if empty, dir/logs is used.
func New(dir string, logsDir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating store dir: %w", err)
	}
	if logsDir == "" {
		logsDir = filepath.Join(dir, "logs")
	}
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating logs dir: %w", err)
	}
	s := &Store{
		stateFile:   filepath.Join(dir, "state.json"),
		logsDir:     logsDir,
		clients:     make(map[string]*common.Client),
		deployments: make(map[string]*common.DeploymentState),
		playbooks:   make(map[string]*PlaybookRecord),
		artifacts:   make(map[string]*common.Artifact),
		secrets:     make(map[string]string),
		logMutexes:  make(map[string]*sync.Mutex),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	if _, err := os.Stat(s.stateFile); os.IsNotExist(err) {
		if err := s.save(); err != nil {
			return nil, fmt.Errorf("initializing store: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("checking store state file: %w", err)
	}
	return s, nil
}

// ── Persistence ───────────────────────────────────────────────────────────────

func (s *Store) load() error {
	data, err := os.ReadFile(s.stateFile)
	if os.IsNotExist(err) {
		return nil // first run
	}
	if err != nil {
		return fmt.Errorf("reading store: %w", err)
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("parsing store: %w", err)
	}
	if snap.Clients != nil {
		s.clients = snap.Clients
	}
	if snap.Deployments != nil {
		s.deployments = snap.Deployments
	}
	if snap.Playbooks != nil {
		s.playbooks = snap.Playbooks
	}
	if snap.Artifacts != nil {
		s.artifacts = snap.Artifacts
	}
	if snap.Secrets != nil {
		s.secrets = snap.Secrets
	}
	for _, c := range s.clients {
		migrateLegacyClientFields(c)
	}
	return nil
}

func migrateLegacyClientFields(c *common.Client) {
	if c == nil || c.Metadata == nil {
		return
	}
	if strings.TrimSpace(c.OS) == "" {
		if osDesc := strings.TrimSpace(c.Metadata["os_description"]); osDesc != "" {
			c.OS = osDesc
		} else if osName := strings.TrimSpace(c.Metadata["os"]); osName != "" {
			c.OS = osName
		}
	}
	if strings.TrimSpace(c.Vendor) == "" {
		c.Vendor = strings.TrimSpace(c.Metadata["vendor"])
	}
	if strings.TrimSpace(c.Model) == "" {
		c.Model = strings.TrimSpace(c.Metadata["model"])
	}
}

func normalizePlatform(platform common.PlatformType, osName string, metadata map[string]string) common.PlatformType {
	current := strings.ToLower(strings.TrimSpace(string(platform)))
	switch current {
	case "", "auto":
		return platformFromOS(osName, metadata)
	case "linux":
		return common.PlatformLinux
	case "mac":
		return common.PlatformMac
	case "windows":
		return common.PlatformWindows
	default:
		return platformFromOS(osName, metadata)
	}
}

func platformFromOS(osName string, metadata map[string]string) common.PlatformType {
	normalizedOS := strings.ToLower(strings.TrimSpace(osName))
	if normalizedOS == "" && metadata != nil {
		normalizedOS = strings.ToLower(strings.TrimSpace(metadata["os"]))
		if normalizedOS == "" {
			normalizedOS = strings.ToLower(strings.TrimSpace(metadata["type"]))
		}
		if normalizedOS == "" {
			normalizedOS = strings.ToLower(strings.TrimSpace(metadata["os_type"]))
		}
	}

	switch normalizedOS {
	case "darwin":
		return common.PlatformMac
	case "windows":
		return common.PlatformWindows
	default:
		return common.PlatformLinux
	}
}

// save must be called with s.mu held (write lock).
func (s *Store) save() error {
	snap := snapshot{
		Clients:     s.clients,
		Deployments: s.deployments,
		Playbooks:   s.playbooks,
		Artifacts:   s.artifacts,
		Secrets:     s.secrets,
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling store: %w", err)
	}
	tmp := s.stateFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing store tmp: %w", err)
	}
	return os.Rename(tmp, s.stateFile)
}

// ── Clients ───────────────────────────────────────────────────────────────────

func (s *Store) SaveClient(c *common.Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.Platform = normalizePlatform(c.Platform, c.OS, c.Metadata)
	cc := cloneClient(c)
	s.clients[cc.ID] = cc
	return s.save()
}

func (s *Store) GetClient(id string) (*common.Client, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.clients[id]
	if !ok {
		return nil, false
	}
	return cloneClient(c), true
}

func (s *Store) ListClients() []*common.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*common.Client, 0, len(s.clients))
	for _, c := range s.clients {
		out = append(out, cloneClient(c))
	}
	sort.Slice(out, func(i, j int) bool {
		hi := strings.ToLower(strings.TrimSpace(out[i].Hostname))
		hj := strings.ToLower(strings.TrimSpace(out[j].Hostname))
		if hi == hj {
			return out[i].ID < out[j].ID
		}
		if hi == "" {
			return false
		}
		if hj == "" {
			return true
		}
		return hi < hj
	})
	return out
}

func (s *Store) DeleteClient(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, id)
	return s.save()
}

// ── Deployments ───────────────────────────────────────────────────────────────

func (s *Store) SaveDeployment(d *common.DeploymentState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deployments[d.ID] = cloneDeployment(d)
	return s.save()
}

func (s *Store) GetDeployment(id string) (*common.DeploymentState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.deployments[id]
	if !ok {
		return nil, false
	}
	return cloneDeployment(d), true
}

// GetActiveDeploymentForClient returns the most recent non-terminal deployment
// for a client (running or rebooting), or any deployment if none is active.
func (s *Store) GetActiveDeploymentForClient(clientID string) (*common.DeploymentState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *common.DeploymentState
	for _, d := range s.deployments {
		if d.ClientID != clientID {
			continue
		}
		if latest == nil || d.StartedAt.After(latest.StartedAt) {
			latest = d
		}
	}
	if latest == nil {
		return nil, false
	}
	return cloneDeployment(latest), true
}

func (s *Store) ListDeployments() []*common.DeploymentState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*common.DeploymentState, 0, len(s.deployments))
	for _, d := range s.deployments {
		out = append(out, cloneDeployment(d))
	}
	return out
}

// ── Playbooks ─────────────────────────────────────────────────────────────────

func (s *Store) SavePlaybook(p *PlaybookRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playbooks[p.ID] = clonePlaybookRecord(p)
	return s.save()
}

func (s *Store) GetPlaybook(id string) (*PlaybookRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.playbooks[id]
	if !ok {
		return nil, false
	}
	return clonePlaybookRecord(p), true
}

func (s *Store) ListPlaybooks() []*PlaybookRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*PlaybookRecord, 0, len(s.playbooks))
	for _, p := range s.playbooks {
		out = append(out, clonePlaybookRecord(p))
	}
	return out
}

func (s *Store) DeletePlaybook(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.playbooks, id)
	return s.save()
}

// ── Artifacts ─────────────────────────────────────────────────────────────────

func (s *Store) SaveArtifact(a *common.Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts[a.ID] = cloneArtifact(a)
	return s.save()
}

func (s *Store) GetArtifact(id string) (*common.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.artifacts[id]
	if !ok {
		return nil, false
	}
	return cloneArtifact(a), true
}

func (s *Store) GetArtifactByName(name string) (*common.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.artifacts {
		if a.Name == name {
			return cloneArtifact(a), true
		}
	}
	return nil, false
}

func (s *Store) ListArtifacts() []*common.Artifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*common.Artifact, 0, len(s.artifacts))
	for _, a := range s.artifacts {
		out = append(out, cloneArtifact(a))
	}
	return out
}

func cloneClient(in *common.Client) *common.Client {
	if in == nil {
		return nil
	}
	out := *in
	if in.Metadata != nil {
		out.Metadata = make(map[string]string, len(in.Metadata))
		for k, v := range in.Metadata {
			out.Metadata[k] = v
		}
	}
	return &out
}

func cloneDeployment(in *common.DeploymentState) *common.DeploymentState {
	if in == nil {
		return nil
	}
	out := *in
	if in.FinishedAt != nil {
		t := *in.FinishedAt
		out.FinishedAt = &t
	}
	return &out
}

func clonePlaybookRecord(in *PlaybookRecord) *PlaybookRecord {
	if in == nil {
		return nil
	}
	out := *in
	out.Playbook = clonePlaybook(in.Playbook)
	return &out
}

func clonePlaybook(in *common.Playbook) *common.Playbook {
	if in == nil {
		return nil
	}
	out := &common.Playbook{Name: in.Name}
	if in.Env != nil {
		out.Env = make(map[string]string, len(in.Env))
		for k, v := range in.Env {
			out.Env[k] = v
		}
	}
	if len(in.Jobs) > 0 {
		out.Jobs = make([]common.Job, len(in.Jobs))
		for i, job := range in.Jobs {
			out.Jobs[i].Name = job.Name
			if len(job.Steps) > 0 {
				out.Jobs[i].Steps = make([]common.Step, len(job.Steps))
				for j, st := range job.Steps {
					out.Jobs[i].Steps[j] = st
					if st.With != nil {
						m := make(map[string]string, len(st.With))
						for k, v := range st.With {
							m[k] = v
						}
						out.Jobs[i].Steps[j].With = m
					}
					if st.Env != nil {
						m := make(map[string]string, len(st.Env))
						for k, v := range st.Env {
							m[k] = v
						}
						out.Jobs[i].Steps[j].Env = m
					}
				}
			}
		}
	}
	return out
}

func cloneArtifact(in *common.Artifact) *common.Artifact {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (s *Store) DeleteArtifact(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.artifacts, id)
	return s.save()
}

// ── Secrets ───────────────────────────────────────────────────────────────────

func (s *Store) SetSecret(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[name] = value
	return s.save()
}

func (s *Store) GetSecret(name string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.secrets[name]
	return v, ok
}

func (s *Store) ListSecretNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.secrets))
	for k := range s.secrets {
		out = append(out, k)
	}
	return out
}

func (s *Store) DeleteSecret(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.secrets, name)
	return s.save()
}

// MergeSecrets bulk-imports secrets without overwriting existing entries.
// Used on startup to seed values from secrets.yml.
func (s *Store) MergeSecrets(m map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range m {
		s.secrets[k] = v
	}
	return s.save()
}

// AllSecrets returns a copy of all secrets for injection into playbooks.
func (s *Store) AllSecrets() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string]string, len(s.secrets))
	for k, v := range s.secrets {
		cp[k] = v
	}
	return cp
}

// ── Logs — per-deployment files ───────────────────────────────────────────────

func (s *Store) logPath(deploymentID string) string {
	return filepath.Join(s.logsDir, deploymentID+".json")
}

// logMutex returns (and lazily creates) the mutex for a specific deployment ID.
func (s *Store) logMutex(deploymentID string) *sync.Mutex {
	s.logMutexesMu.Lock()
	defer s.logMutexesMu.Unlock()
	if mu, ok := s.logMutexes[deploymentID]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	s.logMutexes[deploymentID] = mu
	return mu
}

// AppendLogs appends log entries to their respective per-deployment log files.
// Multiple deployments in a single batch are handled correctly.
func (s *Store) AppendLogs(entries []*common.LogEntry) error {
	// Group by deployment ID.
	byDep := make(map[string][]*common.LogEntry)
	for _, e := range entries {
		byDep[e.DeploymentID] = append(byDep[e.DeploymentID], e)
	}
	for deploymentID, batch := range byDep {
		if err := s.appendDeploymentLogs(deploymentID, batch); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) appendDeploymentLogs(deploymentID string, entries []*common.LogEntry) error {
	mu := s.logMutex(deploymentID)
	mu.Lock()
	defer mu.Unlock()

	path := s.logPath(deploymentID)
	existing := s.readDeploymentLogsLocked(path)
	existing = append(existing, entries...)
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readDeploymentLogsLocked reads log entries from path. The caller must hold the
// per-deployment log mutex.
func (s *Store) readDeploymentLogsLocked(path string) []*common.LogEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []*common.LogEntry
	_ = json.Unmarshal(data, &out)
	return out
}

// GetLogsForDeployment returns all log entries for a specific deployment.
func (s *Store) GetLogsForDeployment(deploymentID string) []*common.LogEntry {
	mu := s.logMutex(deploymentID)
	mu.Lock()
	defer mu.Unlock()
	return s.readDeploymentLogsLocked(s.logPath(deploymentID))
}

// GetLogsForClient returns all log entries for every deployment of a client.
func (s *Store) GetLogsForClient(clientID string) []*common.LogEntry {
	s.mu.RLock()
	deploymentIDs := make([]string, 0)
	for _, d := range s.deployments {
		if d.ClientID == clientID {
			deploymentIDs = append(deploymentIDs, d.ID)
		}
	}
	s.mu.RUnlock()

	var out []*common.LogEntry
	for _, deploymentID := range deploymentIDs {
		mu := s.logMutex(deploymentID)
		mu.Lock()
		out = append(out, s.readDeploymentLogsLocked(s.logPath(deploymentID))...)
		mu.Unlock()
	}
	return out
}
