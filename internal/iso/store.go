// Package iso implements the server-side ISO build pipeline.
package iso

import (
	"log/slog"
	"os"
	"sync"
	"time"
)

// IsoBuildRecord is the persistable representation of a Build.
type IsoBuildRecord struct {
	ID         string
	CustomName string
	SecretName string
	ServerURL  string
	Status     BuildStatus
	StartedAt  time.Time
	FinishedAt *time.Time
	ErrMsg     string
	ISOPath    string
	Logs       []string
}

// BuildPersistence is implemented by any store that can persist ISO build metadata.
type BuildPersistence interface {
	SaveIsoBuild(r IsoBuildRecord) error
	ListIsoBuilds() ([]IsoBuildRecord, error)
	DeleteIsoBuild(id string) error
}

// BuildStatus describes the lifecycle state of an ISO build.
type BuildStatus string

const (
	BuildStatusPending   BuildStatus = "pending"
	BuildStatusRunning   BuildStatus = "running"
	BuildStatusCompleted BuildStatus = "completed"
	BuildStatusFailed    BuildStatus = "failed"
)

// Build holds the live state of one ISO build job.
type Build struct {
	ID         string
	CustomName string
	SecretName string
	ServerURL  string
	StartedAt  time.Time

	mu         sync.RWMutex
	status     BuildStatus
	logs       []string
	isoPath    string
	errMsg     string
	finishedAt *time.Time
}

// BuildSnapshot is a point-in-time copy of Build's state, safe to return as JSON.
type BuildSnapshot struct {
	ID         string      `json:"id"`
	CustomName string      `json:"custom_name,omitempty"`
	SecretName string      `json:"secret_name"`
	ServerURL  string      `json:"server_url"`
	Status     BuildStatus `json:"status"`
	Logs       []string    `json:"logs"`
	LogCount   int         `json:"log_count"`
	HasISO     bool        `json:"has_iso"`
	Error      string      `json:"error,omitempty"`
	StartedAt  time.Time   `json:"started_at"`
	FinishedAt *time.Time  `json:"finished_at,omitempty"`
}

// Snapshot returns a consistent read-only copy of the build state.
func (b *Build) Snapshot() BuildSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	logs := make([]string, len(b.logs))
	copy(logs, b.logs)
	return BuildSnapshot{
		ID:         b.ID,
		CustomName: b.CustomName,
		SecretName: b.SecretName,
		ServerURL:  b.ServerURL,
		Status:     b.status,
		Logs:       logs,
		LogCount:   len(b.logs),
		HasISO:     b.isoPath != "",
		Error:      b.errMsg,
		StartedAt:  b.StartedAt,
		FinishedAt: b.finishedAt,
	}
}

// GetISOPath returns the filesystem path of the completed ISO, or "" if not ready.
func (b *Build) GetISOPath() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.isoPath
}

// AppendLog appends a single output line to the build log.
func (b *Build) AppendLog(line string) {
	b.mu.Lock()
	b.logs = append(b.logs, line)
	b.mu.Unlock()
}

// toRecordLocked snapshots b into an IsoBuildRecord. The caller must hold b.mu.
func (b *Build) toRecordLocked() IsoBuildRecord {
	logs := make([]string, len(b.logs))
	copy(logs, b.logs)
	return IsoBuildRecord{
		ID:         b.ID,
		CustomName: b.CustomName,
		SecretName: b.SecretName,
		ServerURL:  b.ServerURL,
		Status:     b.status,
		StartedAt:  b.StartedAt,
		FinishedAt: b.finishedAt,
		ErrMsg:     b.errMsg,
		ISOPath:    b.isoPath,
		Logs:       logs,
	}
}

// toRecord snapshots b's state while holding its own read lock.
func (b *Build) toRecord() IsoBuildRecord {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.toRecordLocked()
}

func (b *Build) setRunning(s *BuildStore) {
	b.mu.Lock()
	b.status = BuildStatusRunning
	rec := b.toRecordLocked()
	b.mu.Unlock()
	s.persistRecord(rec)
}

func (b *Build) setCompleted(isoPath string, s *BuildStore) {
	now := time.Now()
	b.mu.Lock()
	b.status = BuildStatusCompleted
	b.isoPath = isoPath
	b.finishedAt = &now
	rec := b.toRecordLocked()
	b.mu.Unlock()
	s.persistRecord(rec)
}

func (b *Build) setFailed(errMsg string, s *BuildStore) {
	now := time.Now()
	b.mu.Lock()
	b.status = BuildStatusFailed
	b.errMsg = errMsg
	b.finishedAt = &now
	rec := b.toRecordLocked()
	b.mu.Unlock()
	s.persistRecord(rec)
}

// ── BuildStore ────────────────────────────────────────────────────────────────

// BuildStore is an in-memory store for ISO build sessions backed by optional persistence.
type BuildStore struct {
	mu     sync.RWMutex
	builds map[string]*Build
	db     BuildPersistence // nil = no persistence
}

// NewBuildStore creates a BuildStore. If db is non-nil, existing builds are loaded from
// it and any pending/running builds are marked as failed (they cannot be resumed).
func NewBuildStore(db BuildPersistence) *BuildStore {
	s := &BuildStore{builds: make(map[string]*Build), db: db}
	if db == nil {
		return s
	}
	records, err := db.ListIsoBuilds()
	if err != nil {
		slog.Error("iso: failed to load builds from database", "error", err)
		return s
	}
	for _, r := range records {
		b := &Build{
			ID:         r.ID,
			CustomName: r.CustomName,
			SecretName: r.SecretName,
			ServerURL:  r.ServerURL,
			StartedAt:  r.StartedAt,
			status:     r.Status,
			isoPath:    r.ISOPath,
			errMsg:     r.ErrMsg,
			finishedAt: r.FinishedAt,
			logs:       r.Logs,
		}
		// Builds that were in-flight when the server stopped cannot be resumed.
		if b.status == BuildStatusPending || b.status == BuildStatusRunning {
			now := time.Now()
			b.mu.Lock()
			b.status = BuildStatusFailed
			b.errMsg = "interrupted by server restart"
			b.finishedAt = &now
			rec := b.toRecordLocked()
			b.mu.Unlock()
			_ = db.SaveIsoBuild(rec)
		}
		s.builds[r.ID] = b
	}
	return s
}

// Create registers a new build job, persists it, and returns it.
func (s *BuildStore) Create(id, secretName, serverURL, customName string) *Build {
	b := &Build{
		ID:         id,
		CustomName: customName,
		SecretName: secretName,
		ServerURL:  serverURL,
		status:     BuildStatusPending,
		StartedAt:  time.Now(),
	}
	s.mu.Lock()
	s.builds[id] = b
	s.mu.Unlock()
	s.persist(b)
	return b
}

// Get retrieves a build by ID.
func (s *BuildStore) Get(id string) (*Build, bool) {
	s.mu.RLock()
	b, ok := s.builds[id]
	s.mu.RUnlock()
	return b, ok
}

// List returns all known builds (unordered).
func (s *BuildStore) List() []*Build {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*Build, 0, len(s.builds))
	for _, b := range s.builds {
		list = append(list, b)
	}
	return list
}

// Delete removes a build from memory and the database, and deletes its ISO file.
// Returns false if the build was not found.
func (s *BuildStore) Delete(id string) bool {
	s.mu.Lock()
	b, ok := s.builds[id]
	if !ok {
		s.mu.Unlock()
		return false
	}
	delete(s.builds, id)
	s.mu.Unlock()

	if isoPath := b.GetISOPath(); isoPath != "" {
		_ = os.Remove(isoPath)
	}
	if s.db != nil {
		_ = s.db.DeleteIsoBuild(id)
	}
	return true
}

// persist snapshots b and writes it to the database.
func (s *BuildStore) persist(b *Build) {
	s.persistRecord(b.toRecord())
}

// persistRecord writes rec to the database (no-op when db is nil).
func (s *BuildStore) persistRecord(rec IsoBuildRecord) {
	if s.db == nil {
		return
	}
	_ = s.db.SaveIsoBuild(rec)
}
