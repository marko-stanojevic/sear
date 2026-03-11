// Package store provides a simple JSON-file-backed persistence layer for the
// sear daemon.  All public methods are safe for concurrent use.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/marko-stanojevic/sear/internal/common"
)

// Store holds all persistent state for the daemon.
type Store struct {
	mu          sync.RWMutex
	dir         string
	clients     map[string]*common.Client
	deployments map[string]*common.DeploymentState
	playbooks   map[string]*PlaybookRecord
	artifacts   map[string]*common.Artifact
	secrets     map[string]string // client secrets (name→value)
	logs        []*common.LogEntry
}

// PlaybookRecord wraps a Playbook with server-side metadata.
type PlaybookRecord struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Playbook    *common.Playbook `json:"playbook"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// snapshot is serialised to disk.
type snapshot struct {
	Clients     map[string]*common.Client         `json:"clients"`
	Deployments map[string]*common.DeploymentState `json:"deployments"`
	Playbooks   map[string]*PlaybookRecord          `json:"playbooks"`
	Artifacts   map[string]*common.Artifact         `json:"artifacts"`
	Secrets     map[string]string                   `json:"secrets"`
	Logs        []*common.LogEntry                  `json:"logs"`
}

// New creates (or reopens) a store rooted at dir.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating store dir: %w", err)
	}
	s := &Store{
		dir:         dir,
		clients:     make(map[string]*common.Client),
		deployments: make(map[string]*common.DeploymentState),
		playbooks:   make(map[string]*PlaybookRecord),
		artifacts:   make(map[string]*common.Artifact),
		secrets:     make(map[string]string),
		logs:        nil,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// ---- persistence -----------------------------------------------------------

func (s *Store) snapshotPath() string { return filepath.Join(s.dir, "state.json") }

func (s *Store) load() error {
	path := s.snapshotPath()
	data, err := os.ReadFile(path)
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
	if snap.Logs != nil {
		s.logs = snap.Logs
	}
	return nil
}

// save must be called with s.mu held (write lock).
func (s *Store) save() error {
	snap := snapshot{
		Clients:     s.clients,
		Deployments: s.deployments,
		Playbooks:   s.playbooks,
		Artifacts:   s.artifacts,
		Secrets:     s.secrets,
		Logs:        s.logs,
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling store: %w", err)
	}
	tmp := s.snapshotPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing store tmp: %w", err)
	}
	return os.Rename(tmp, s.snapshotPath())
}

// ---- Clients ---------------------------------------------------------------

func (s *Store) SaveClient(c *common.Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[c.ID] = c
	return s.save()
}

func (s *Store) GetClient(id string) (*common.Client, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.clients[id]
	return c, ok
}

func (s *Store) ListClients() []*common.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*common.Client, 0, len(s.clients))
	for _, c := range s.clients {
		out = append(out, c)
	}
	return out
}

func (s *Store) DeleteClient(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, id)
	return s.save()
}

// ---- Deployments -----------------------------------------------------------

func (s *Store) SaveDeployment(d *common.DeploymentState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deployments[d.ID] = d
	return s.save()
}

func (s *Store) GetDeployment(id string) (*common.DeploymentState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.deployments[id]
	return d, ok
}

// GetDeploymentForClient returns the most recent deployment for a client.
func (s *Store) GetDeploymentForClient(clientID string) (*common.DeploymentState, bool) {
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
	return latest, latest != nil
}

func (s *Store) ListDeployments() []*common.DeploymentState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*common.DeploymentState, 0, len(s.deployments))
	for _, d := range s.deployments {
		out = append(out, d)
	}
	return out
}

// ---- Playbooks -------------------------------------------------------------

func (s *Store) SavePlaybook(p *PlaybookRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playbooks[p.ID] = p
	return s.save()
}

func (s *Store) GetPlaybook(id string) (*PlaybookRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.playbooks[id]
	return p, ok
}

func (s *Store) ListPlaybooks() []*PlaybookRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*PlaybookRecord, 0, len(s.playbooks))
	for _, p := range s.playbooks {
		out = append(out, p)
	}
	return out
}

func (s *Store) DeletePlaybook(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.playbooks, id)
	return s.save()
}

// ---- Artifacts -------------------------------------------------------------

func (s *Store) SaveArtifact(a *common.Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts[a.ID] = a
	return s.save()
}

func (s *Store) GetArtifact(id string) (*common.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.artifacts[id]
	return a, ok
}

func (s *Store) GetArtifactByName(name string) (*common.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.artifacts {
		if a.Name == name {
			return a, true
		}
	}
	return nil, false
}

func (s *Store) ListArtifacts() []*common.Artifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*common.Artifact, 0, len(s.artifacts))
	for _, a := range s.artifacts {
		out = append(out, a)
	}
	return out
}

func (s *Store) DeleteArtifact(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.artifacts, id)
	return s.save()
}

// ---- Secrets ---------------------------------------------------------------

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

// MergeSecrets bulk-imports secrets (e.g. from secrets.yml).
func (s *Store) MergeSecrets(m map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range m {
		s.secrets[k] = v
	}
	return s.save()
}

// AllSecrets returns a copy of all secrets; used when injecting into playbooks.
func (s *Store) AllSecrets() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string]string, len(s.secrets))
	for k, v := range s.secrets {
		cp[k] = v
	}
	return cp
}

// ---- Logs ------------------------------------------------------------------

func (s *Store) AppendLogs(entries []*common.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, entries...)
	return s.save()
}

func (s *Store) GetLogsForDeployment(deploymentID string) []*common.LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*common.LogEntry
	for _, l := range s.logs {
		if l.DeploymentID == deploymentID {
			out = append(out, l)
		}
	}
	return out
}

func (s *Store) GetLogsForClient(clientID string) []*common.LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Collect deployment IDs for the client.
	depIDs := make(map[string]struct{})
	for _, d := range s.deployments {
		if d.ClientID == clientID {
			depIDs[d.ID] = struct{}{}
		}
	}
	var out []*common.LogEntry
	for _, l := range s.logs {
		if _, ok := depIDs[l.DeploymentID]; ok {
			out = append(out, l)
		}
	}
	return out
}
