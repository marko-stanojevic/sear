// Package store provides a SQLite-backed persistence layer for the kompakt server.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // SQLite driver

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/iso"
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

// Store holds the SQLite database connection.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS agents (
    id               TEXT PRIMARY KEY,
    hostname         TEXT NOT NULL DEFAULT '',
    platform         TEXT NOT NULL DEFAULT '',
    os               TEXT NOT NULL DEFAULT '',
    model            TEXT NOT NULL DEFAULT '',
    vendor           TEXT NOT NULL DEFAULT '',
    ip_address       TEXT NOT NULL DEFAULT '',
    metadata_json    TEXT NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL DEFAULT '',
    playbook_id      TEXT NOT NULL DEFAULT '',
    registered_at    TEXT NOT NULL DEFAULT '',
    last_activity_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_agents_hostname ON agents (hostname COLLATE NOCASE);

CREATE TABLE IF NOT EXISTS deployments (
    id                TEXT PRIMARY KEY,
    agent_id          TEXT NOT NULL DEFAULT '',
    playbook_id       TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT '',
    resume_step_index INTEGER NOT NULL DEFAULT 0,
    started_at        TEXT NOT NULL DEFAULT '',
    updated_at        TEXT NOT NULL DEFAULT '',
    finished_at       TEXT,
    error_detail      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_deployments_agent_id      ON deployments (agent_id);
CREATE INDEX IF NOT EXISTS idx_deployments_agent_started ON deployments (agent_id, started_at DESC);

CREATE TABLE IF NOT EXISTS playbooks (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    playbook_json TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL DEFAULT '',
    updated_at    TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS artifacts (
    id                   TEXT PRIMARY KEY,
    name                 TEXT NOT NULL DEFAULT '',
    filename             TEXT NOT NULL DEFAULT '',
    size                 INTEGER NOT NULL DEFAULT 0,
    content_type         TEXT NOT NULL DEFAULT '',
    access_policy        TEXT NOT NULL DEFAULT '',
    allowed_agents_json  TEXT NOT NULL DEFAULT '[]',
    uploaded_at          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_artifacts_name ON artifacts (name);

CREATE TABLE IF NOT EXISTS secrets (
    name  TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS logs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    deployment_id TEXT NOT NULL DEFAULT '',
    job_name      TEXT NOT NULL DEFAULT '',
    step_index    INTEGER NOT NULL DEFAULT 0,
    level         TEXT NOT NULL DEFAULT '',
    message       TEXT NOT NULL DEFAULT '',
    timestamp     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_logs_deployment_id ON logs (deployment_id);

CREATE TABLE IF NOT EXISTS agent_tokens (
    id         TEXT PRIMARY KEY,
    agent_id   TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT,
    revoked_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_hash     ON agent_tokens (token_hash);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_agent_id ON agent_tokens (agent_id);

CREATE TABLE IF NOT EXISTS iso_builds (
    id          TEXT PRIMARY KEY,
    secret_name TEXT NOT NULL DEFAULT '',
    server_url  TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT '',
    started_at  TEXT NOT NULL DEFAULT '',
    finished_at TEXT,
    error_msg   TEXT NOT NULL DEFAULT '',
    iso_path    TEXT NOT NULL DEFAULT ''
);
`

// New opens (or creates) the SQLite store at dir/kompakt.db.
// The logsDir parameter is accepted for API compatibility but is not used.
func New(dir string, _ string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating store dir: %w", err)
	}
	dbPath := filepath.Join(dir, "kompakt.db")
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	// Enforce foreign key constraints and cascades. This must be executed after
	// every new connection is opened; the DSN parameter alone is not reliable
	// across all SQLite driver versions.
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	// Additive migrations: ignore errors when column already exists.
	for _, m := range []string{
		`ALTER TABLE agents     ADD COLUMN shells_json  TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE iso_builds ADD COLUMN logs_json    TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE iso_builds ADD COLUMN custom_name  TEXT NOT NULL DEFAULT ''`,
	} {
		_, _ = db.Exec(m)
	}
	// Data migrations: rename status values for consistency.
	_, _ = db.Exec(`UPDATE agents      SET status = 'completed' WHERE status = 'done'`)
	_, _ = db.Exec(`UPDATE deployments SET status = 'completed' WHERE status = 'done'`)
	return &Store{db: db}, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// ── Time helpers ──────────────────────────────────────────────────────────────

func encodeTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func decodeTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func encodeTimePtr(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: encodeTime(*t), Valid: true}
}

func decodeTimePtr(ns sql.NullString) *time.Time {
	if !ns.Valid {
		return nil
	}
	t, _ := time.Parse(time.RFC3339Nano, ns.String)
	return &t
}

// ── Agents ────────────────────────────────────────────────────────────────────

func (s *Store) SaveAgent(a *common.Agent) error {
	meta, err := json.Marshal(a.Metadata)
	if err != nil {
		return fmt.Errorf("marshalling agent metadata: %w", err)
	}
	shells, err := json.Marshal(a.Shells)
	if err != nil {
		return fmt.Errorf("marshalling agent shells: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO agents (id, hostname, platform, os, model, vendor, ip_address,
		                    metadata_json, shells_json, status, playbook_id, registered_at, last_activity_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		    hostname=excluded.hostname, platform=excluded.platform, os=excluded.os,
		    model=excluded.model, vendor=excluded.vendor, ip_address=excluded.ip_address,
		    metadata_json=excluded.metadata_json, shells_json=excluded.shells_json,
		    status=excluded.status, playbook_id=excluded.playbook_id,
		    last_activity_at=excluded.last_activity_at`,
		a.ID, a.Hostname, string(a.Platform), a.OS, a.Model, a.Vendor, a.IPAddress,
		string(meta), string(shells), string(a.Status), a.PlaybookID,
		encodeTime(a.RegisteredAt), encodeTime(a.LastActivityAt),
	)
	return err
}

func (s *Store) GetAgent(id string) (*common.Agent, bool) {
	row := s.db.QueryRow(`
		SELECT id, hostname, platform, os, model, vendor, ip_address,
		       metadata_json, shells_json, status, playbook_id, registered_at, last_activity_at
		FROM agents WHERE id = ?`, id)
	a, err := scanAgent(row)
	if err != nil {
		return nil, false
	}
	return a, true
}

func (s *Store) ListAgents() []*common.Agent {
	rows, err := s.db.Query(`
		SELECT id, hostname, platform, os, model, vendor, ip_address,
		       metadata_json, shells_json, status, playbook_id, registered_at, last_activity_at
		FROM agents
		ORDER BY
		    CASE WHEN TRIM(hostname) = '' THEN 1 ELSE 0 END ASC,
		    LOWER(TRIM(hostname)) ASC,
		    id ASC`)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var out []*common.Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err == nil {
			out = append(out, a)
		}
	}
	return out
}

func (s *Store) DeleteAgent(id string) error {
	_, err := s.db.Exec(`DELETE FROM agents WHERE id = ?`, id)
	return err
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanAgent(r scanner) (*common.Agent, error) {
	var (
		id, hostname, platform, os_, model, vendor, ipAddress string
		metaJSON, shellsJSON, status, playbookID              string
		registeredAt, lastActivityAt                          string
	)
	if err := r.Scan(&id, &hostname, &platform, &os_, &model, &vendor, &ipAddress,
		&metaJSON, &shellsJSON, &status, &playbookID, &registeredAt, &lastActivityAt); err != nil {
		return nil, err
	}
	var meta map[string]string
	_ = json.Unmarshal([]byte(metaJSON), &meta)
	var shells []string
	_ = json.Unmarshal([]byte(shellsJSON), &shells)
	return &common.Agent{
		ID:             id,
		Hostname:       hostname,
		Platform:       common.PlatformType(platform),
		OS:             os_,
		Model:          model,
		Vendor:         vendor,
		IPAddress:      ipAddress,
		Metadata:       meta,
		Shells:         shells,
		Status:         common.AgentStatus(status),
		PlaybookID:     playbookID,
		RegisteredAt:   decodeTime(registeredAt),
		LastActivityAt: decodeTime(lastActivityAt),
	}, nil
}

// ── Deployments ───────────────────────────────────────────────────────────────

func (s *Store) SaveDeployment(d *common.DeploymentState) error {
	_, err := s.db.Exec(`
		INSERT INTO deployments (id, agent_id, playbook_id, status, resume_step_index,
		                         started_at, updated_at, finished_at, error_detail)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		    agent_id=excluded.agent_id, playbook_id=excluded.playbook_id,
		    status=excluded.status, resume_step_index=excluded.resume_step_index,
		    updated_at=excluded.updated_at, finished_at=excluded.finished_at,
		    error_detail=excluded.error_detail`,
		d.ID, d.AgentID, d.PlaybookID, string(d.Status), d.ResumeStepIndex,
		encodeTime(d.StartedAt), encodeTime(d.UpdatedAt),
		encodeTimePtr(d.FinishedAt), d.ErrorDetail,
	)
	return err
}

func (s *Store) GetDeployment(id string) (*common.DeploymentState, bool) {
	row := s.db.QueryRow(`
		SELECT id, agent_id, playbook_id, status, resume_step_index,
		       started_at, updated_at, finished_at, error_detail
		FROM deployments WHERE id = ?`, id)
	d, err := scanDeployment(row)
	if err != nil {
		return nil, false
	}
	s.enrichDeployment(d)
	return d, true
}

func (s *Store) GetActiveDeploymentForAgent(agentID string) (*common.DeploymentState, bool) {
	row := s.db.QueryRow(`
		SELECT id, agent_id, playbook_id, status, resume_step_index,
		       started_at, updated_at, finished_at, error_detail
		FROM deployments
		WHERE agent_id = ?
		ORDER BY started_at DESC
		LIMIT 1`, agentID)
	d, err := scanDeployment(row)
	if err != nil {
		return nil, false
	}
	s.enrichDeployment(d)
	return d, true
}

func (s *Store) ListDeployments() []*common.DeploymentState {
	rows, err := s.db.Query(`
		SELECT id, agent_id, playbook_id, status, resume_step_index,
		       started_at, updated_at, finished_at, error_detail
		FROM deployments`)
	if err != nil {
		return nil
	}
	// Collect all rows before closing the cursor so the connection is free
	// for the enrichDeployment sub-queries.
	var out []*common.DeploymentState
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err == nil {
			out = append(out, d)
		}
	}
	_ = rows.Close()
	for _, d := range out {
		s.enrichDeployment(d)
	}
	return out
}

func (s *Store) DeleteDeployment(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM deployments WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM logs WHERE deployment_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func scanDeployment(r scanner) (*common.DeploymentState, error) {
	var (
		id, agentID, playbookID, status string
		resumeStepIndex                 int
		startedAt, updatedAt            string
		finishedAt                      sql.NullString
		errorDetail                     string
	)
	if err := r.Scan(&id, &agentID, &playbookID, &status, &resumeStepIndex,
		&startedAt, &updatedAt, &finishedAt, &errorDetail); err != nil {
		return nil, err
	}
	return &common.DeploymentState{
		ID:              id,
		AgentID:         agentID,
		PlaybookID:      playbookID,
		Status:          common.DeploymentStatus(status),
		ResumeStepIndex: resumeStepIndex,
		StartedAt:       decodeTime(startedAt),
		UpdatedAt:       decodeTime(updatedAt),
		FinishedAt:      decodeTimePtr(finishedAt),
		ErrorDetail:     errorDetail,
	}, nil
}

// enrichDeployment fills the derived Hostname and PlaybookName fields.
func (s *Store) enrichDeployment(d *common.DeploymentState) {
	if d == nil {
		return
	}
	var hostname string
	err := s.db.QueryRow(`SELECT hostname FROM agents WHERE id = ?`, d.AgentID).Scan(&hostname)
	if err != nil {
		d.Hostname = "Deleted Agent"
	} else {
		d.Hostname = hostname
	}
	var playbookName string
	err = s.db.QueryRow(`SELECT name FROM playbooks WHERE id = ?`, d.PlaybookID).Scan(&playbookName)
	if err != nil {
		d.PlaybookName = "Deleted Playbook"
	} else {
		d.PlaybookName = playbookName
	}
}

// ── Playbooks ─────────────────────────────────────────────────────────────────

func (s *Store) SavePlaybook(p *PlaybookRecord) error {
	pbJSON, err := json.Marshal(p.Playbook)
	if err != nil {
		return fmt.Errorf("marshalling playbook: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO playbooks (id, name, description, playbook_json, created_at, updated_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		    name=excluded.name, description=excluded.description,
		    playbook_json=excluded.playbook_json, updated_at=excluded.updated_at`,
		p.ID, p.Name, p.Description, string(pbJSON),
		encodeTime(p.CreatedAt), encodeTime(p.UpdatedAt),
	)
	return err
}

func (s *Store) GetPlaybook(id string) (*PlaybookRecord, bool) {
	row := s.db.QueryRow(`
		SELECT id, name, description, playbook_json, created_at, updated_at
		FROM playbooks WHERE id = ?`, id)
	p, err := scanPlaybook(row)
	if err != nil {
		return nil, false
	}
	return p, true
}

func (s *Store) ListPlaybooks() []*PlaybookRecord {
	rows, err := s.db.Query(`
		SELECT id, name, description, playbook_json, created_at, updated_at
		FROM playbooks`)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var out []*PlaybookRecord
	for rows.Next() {
		p, err := scanPlaybook(rows)
		if err == nil {
			out = append(out, p)
		}
	}
	return out
}

func (s *Store) DeletePlaybook(id string) error {
	_, err := s.db.Exec(`DELETE FROM playbooks WHERE id = ?`, id)
	return err
}

func scanPlaybook(r scanner) (*PlaybookRecord, error) {
	var id, name, description, pbJSON, createdAt, updatedAt string
	if err := r.Scan(&id, &name, &description, &pbJSON, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var pb common.Playbook
	_ = json.Unmarshal([]byte(pbJSON), &pb)
	return &PlaybookRecord{
		ID:          id,
		Name:        name,
		Description: description,
		Playbook:    &pb,
		CreatedAt:   decodeTime(createdAt),
		UpdatedAt:   decodeTime(updatedAt),
	}, nil
}

// ── Artifacts ─────────────────────────────────────────────────────────────────

func (s *Store) SaveArtifact(a *common.Artifact) error {
	agentsJSON, err := json.Marshal(a.AllowedAgents)
	if err != nil {
		return fmt.Errorf("marshalling allowed agents: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO artifacts (id, name, filename, size, content_type,
		                       access_policy, allowed_agents_json, uploaded_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		    name=excluded.name, filename=excluded.filename, size=excluded.size,
		    content_type=excluded.content_type, access_policy=excluded.access_policy,
		    allowed_agents_json=excluded.allowed_agents_json`,
		a.ID, a.Name, a.FileName, a.Size, a.ContentType,
		string(a.AccessPolicy), string(agentsJSON), encodeTime(a.UploadedAt),
	)
	return err
}

func (s *Store) GetArtifact(id string) (*common.Artifact, bool) {
	row := s.db.QueryRow(`
		SELECT id, name, filename, size, content_type, access_policy, allowed_agents_json, uploaded_at
		FROM artifacts WHERE id = ?`, id)
	a, err := scanArtifact(row)
	if err != nil {
		return nil, false
	}
	return a, true
}

func (s *Store) GetArtifactByName(name string) (*common.Artifact, bool) {
	row := s.db.QueryRow(`
		SELECT id, name, filename, size, content_type, access_policy, allowed_agents_json, uploaded_at
		FROM artifacts WHERE name = ?`, name)
	a, err := scanArtifact(row)
	if err != nil {
		return nil, false
	}
	return a, true
}

func (s *Store) ListArtifacts() []*common.Artifact {
	rows, err := s.db.Query(`
		SELECT id, name, filename, size, content_type, access_policy, allowed_agents_json, uploaded_at
		FROM artifacts`)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var out []*common.Artifact
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err == nil {
			out = append(out, a)
		}
	}
	return out
}

func (s *Store) DeleteArtifact(id string) error {
	_, err := s.db.Exec(`DELETE FROM artifacts WHERE id = ?`, id)
	return err
}

func scanArtifact(r scanner) (*common.Artifact, error) {
	var (
		id, name, filename, contentType, accessPolicy string
		size                                          int64
		agentsJSON, uploadedAt                        string
	)
	if err := r.Scan(&id, &name, &filename, &size, &contentType,
		&accessPolicy, &agentsJSON, &uploadedAt); err != nil {
		return nil, err
	}
	var agents []string
	_ = json.Unmarshal([]byte(agentsJSON), &agents)
	return &common.Artifact{
		ID:            id,
		Name:          name,
		FileName:      filename,
		Size:          size,
		ContentType:   contentType,
		AccessPolicy:  common.AccessPolicy(accessPolicy),
		AllowedAgents: agents,
		UploadedAt:    decodeTime(uploadedAt),
	}, nil
}

// ── Secrets ───────────────────────────────────────────────────────────────────

func (s *Store) SetSecret(name, value string) error {
	_, err := s.db.Exec(`INSERT INTO secrets (name, value) VALUES (?,?)
		ON CONFLICT(name) DO UPDATE SET value=excluded.value`, name, value)
	return err
}

func (s *Store) GetSecret(name string) (string, bool) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM secrets WHERE name = ?`, name).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

func (s *Store) ListSecretNames() []string {
	rows, err := s.db.Query(`SELECT name FROM secrets ORDER BY name`)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			out = append(out, name)
		}
	}
	return out
}

func (s *Store) DeleteSecret(name string) error {
	_, err := s.db.Exec(`DELETE FROM secrets WHERE name = ?`, name)
	return err
}

// MergeSecrets bulk-imports secrets, overwriting existing entries.
// Used on startup to seed values from secrets.yml.
func (s *Store) MergeSecrets(m map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for k, v := range m {
		if _, err := tx.Exec(`INSERT INTO secrets (name, value) VALUES (?,?)
			ON CONFLICT(name) DO UPDATE SET value=excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AllSecrets returns a copy of all secrets for injection into playbooks.
func (s *Store) AllSecrets() map[string]string {
	rows, err := s.db.Query(`SELECT name, value FROM secrets`)
	if err != nil {
		return map[string]string{}
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]string)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err == nil {
			out[name] = value
		}
	}
	return out
}

// ── Logs ──────────────────────────────────────────────────────────────────────

func (s *Store) AppendLogs(entries []*common.LogEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`INSERT INTO logs (deployment_id, job_name, step_index, level, message, timestamp)
		VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, e := range entries {
		if _, err := stmt.Exec(e.DeploymentID, e.JobName, e.StepIndex,
			string(e.Level), e.Message, encodeTime(e.Timestamp)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetLogsForDeployment(deploymentID string) []*common.LogEntry {
	rows, err := s.db.Query(`
		SELECT deployment_id, job_name, step_index, level, message, timestamp
		FROM logs WHERE deployment_id = ? ORDER BY id ASC`, deploymentID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	return scanLogs(rows)
}

func (s *Store) GetLogsForAgent(agentID string) []*common.LogEntry {
	rows, err := s.db.Query(`
		SELECT l.deployment_id, l.job_name, l.step_index, l.level, l.message, l.timestamp
		FROM logs l
		JOIN deployments d ON l.deployment_id = d.id
		WHERE d.agent_id = ?
		ORDER BY l.id ASC`, agentID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	return scanLogs(rows)
}

func scanLogs(rows *sql.Rows) []*common.LogEntry {
	var out []*common.LogEntry
	for rows.Next() {
		var deploymentID, jobName, level, message, timestamp string
		var stepIndex int
		if err := rows.Scan(&deploymentID, &jobName, &stepIndex, &level, &message, &timestamp); err == nil {
			out = append(out, &common.LogEntry{
				DeploymentID: deploymentID,
				JobName:      jobName,
				StepIndex:    stepIndex,
				Level:        common.LogLevel(level),
				Message:      message,
				Timestamp:    decodeTime(timestamp),
			})
		}
	}
	return out
}

// ── Agent tokens ──────────────────────────────────────────────────────────────

func (s *Store) CreateAgentToken(t *common.AgentToken) error {
	_, err := s.db.Exec(
		`INSERT INTO agent_tokens (id, agent_id, token_hash, created_at, expires_at, revoked_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.ID, t.AgentID, t.TokenHash,
		encodeTime(t.CreatedAt),
		encodeTimePtr(t.ExpiresAt),
		encodeTimePtr(t.RevokedAt),
	)
	return err
}

func (s *Store) GetAgentTokenByHash(hash string) (*common.AgentToken, error) {
	row := s.db.QueryRow(
		`SELECT id, agent_id, token_hash, created_at, expires_at, revoked_at
		 FROM agent_tokens WHERE token_hash = ?`, hash)
	var t common.AgentToken
	var createdAt string
	var expiresAt, revokedAt sql.NullString
	if err := row.Scan(&t.ID, &t.AgentID, &t.TokenHash, &createdAt, &expiresAt, &revokedAt); err != nil {
		return nil, err
	}
	t.CreatedAt = decodeTime(createdAt)
	t.ExpiresAt = decodeTimePtr(expiresAt)
	t.RevokedAt = decodeTimePtr(revokedAt)
	return &t, nil
}

func (s *Store) RevokeAgentToken(id string) error {
	_, err := s.db.Exec(
		`UPDATE agent_tokens SET revoked_at = ? WHERE id = ?`,
		encodeTime(time.Now()), id)
	return err
}

func (s *Store) RevokeAllAgentTokens(agentID string) error {
	_, err := s.db.Exec(
		`UPDATE agent_tokens SET revoked_at = ? WHERE agent_id = ? AND revoked_at IS NULL`,
		encodeTime(time.Now()), agentID)
	return err
}

// ── ISO Builds ────────────────────────────────────────────────────────────────

func (s *Store) SaveIsoBuild(r iso.IsoBuildRecord) error {
	logsJSON, err := json.Marshal(r.Logs)
	if err != nil {
		return fmt.Errorf("marshalling iso build logs: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO iso_builds (id, custom_name, secret_name, server_url, status, started_at, finished_at, error_msg, iso_path, logs_json)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		    status=excluded.status, finished_at=excluded.finished_at,
		    error_msg=excluded.error_msg, iso_path=excluded.iso_path, logs_json=excluded.logs_json`,
		r.ID, r.CustomName, r.SecretName, r.ServerURL, string(r.Status),
		encodeTime(r.StartedAt), encodeTimePtr(r.FinishedAt),
		r.ErrMsg, r.ISOPath, string(logsJSON),
	)
	return err
}

func (s *Store) DeleteIsoBuild(id string) error {
	_, err := s.db.Exec(`DELETE FROM iso_builds WHERE id = ?`, id)
	return err
}

func (s *Store) ListIsoBuilds() ([]iso.IsoBuildRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, custom_name, secret_name, server_url, status, started_at, finished_at, error_msg, iso_path, logs_json
		FROM iso_builds ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []iso.IsoBuildRecord
	for rows.Next() {
		var r iso.IsoBuildRecord
		var status, startedAt, logsJSON string
		var finishedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.CustomName, &r.SecretName, &r.ServerURL, &status,
			&startedAt, &finishedAt, &r.ErrMsg, &r.ISOPath, &logsJSON); err != nil {
			return nil, err
		}
		r.Status = iso.BuildStatus(status)
		r.StartedAt = decodeTime(startedAt)
		r.FinishedAt = decodeTimePtr(finishedAt)
		_ = json.Unmarshal([]byte(logsJSON), &r.Logs)
		out = append(out, r)
	}
	return out, nil
}
