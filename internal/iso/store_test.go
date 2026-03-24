package iso

import (
	"sync"
	"testing"
	"time"
)

// ── in-memory BuildPersistence for tests ──────────────────────────────────────

type memDB struct {
	mu      sync.Mutex
	records map[string]IsoBuildRecord
}

func newMemDB() *memDB { return &memDB{records: make(map[string]IsoBuildRecord)} }

func (m *memDB) SaveIsoBuild(r IsoBuildRecord) error {
	m.mu.Lock()
	m.records[r.ID] = r
	m.mu.Unlock()
	return nil
}

func (m *memDB) ListIsoBuilds() ([]IsoBuildRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]IsoBuildRecord, 0, len(m.records))
	for _, r := range m.records {
		out = append(out, r)
	}
	return out, nil
}

func (m *memDB) DeleteIsoBuild(id string) error {
	m.mu.Lock()
	delete(m.records, id)
	m.mu.Unlock()
	return nil
}

func (m *memDB) get(id string) (IsoBuildRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	return r, ok
}

// ── state transition tests ────────────────────────────────────────────────────

func TestBuildStateTransitions(t *testing.T) {
	tests := []struct {
		name       string
		transition func(s *BuildStore, b *Build)
		wantStatus BuildStatus
		wantHasISO bool
		wantErrMsg string
	}{
		{
			name:       "create yields pending",
			transition: func(_ *BuildStore, _ *Build) {},
			wantStatus: BuildStatusPending,
		},
		{
			name: "pending to running",
			transition: func(s *BuildStore, b *Build) {
				b.setRunning(s)
			},
			wantStatus: BuildStatusRunning,
		},
		{
			name: "running to completed",
			transition: func(s *BuildStore, b *Build) {
				b.setRunning(s)
				b.setCompleted("/tmp/out.iso", s)
			},
			wantStatus: BuildStatusCompleted,
			wantHasISO: true,
		},
		{
			name: "running to failed",
			transition: func(s *BuildStore, b *Build) {
				b.setRunning(s)
				b.setFailed("docker not found", s)
			},
			wantStatus: BuildStatusFailed,
			wantErrMsg: "docker not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newMemDB()
			s := NewBuildStore(db)
			b := s.Create("id-1", "default", "https://example.com", "")

			tc.transition(s, b)

			snap := b.Snapshot()
			if snap.Status != tc.wantStatus {
				t.Errorf("status = %q; want %q", snap.Status, tc.wantStatus)
			}
			if snap.HasISO != tc.wantHasISO {
				t.Errorf("HasISO = %v; want %v", snap.HasISO, tc.wantHasISO)
			}
			if snap.Error != tc.wantErrMsg {
				t.Errorf("Error = %q; want %q", snap.Error, tc.wantErrMsg)
			}
			if tc.wantStatus == BuildStatusCompleted || tc.wantStatus == BuildStatusFailed {
				if snap.FinishedAt == nil {
					t.Error("FinishedAt should be set for terminal states")
				}
			}

			// DB must mirror in-memory state.
			r, ok := db.get("id-1")
			if !ok {
				t.Fatal("build not found in DB")
			}
			if r.Status != tc.wantStatus {
				t.Errorf("db status = %q; want %q", r.Status, tc.wantStatus)
			}
		})
	}
}

// ── log tests ─────────────────────────────────────────────────────────────────

func TestBuildAppendLog(t *testing.T) {
	s := NewBuildStore(nil)
	b := s.Create("id-log", "default", "https://example.com", "")
	lines := []string{"line one", "line two", "line three"}
	for _, l := range lines {
		b.AppendLog(l)
	}
	snap := b.Snapshot()
	if snap.LogCount != len(lines) {
		t.Fatalf("LogCount = %d; want %d", snap.LogCount, len(lines))
	}
	for i, want := range lines {
		if snap.Logs[i] != want {
			t.Errorf("Logs[%d] = %q; want %q", i, snap.Logs[i], want)
		}
	}
}

func TestLogOffsetSlicing(t *testing.T) {
	tests := []struct {
		name     string
		total    int
		offset   int
		wantLen  int
	}{
		{"zero offset returns all", 5, 0, 5},
		{"mid offset returns tail", 5, 3, 2},
		{"offset at end returns none", 5, 5, 0},
		{"offset beyond end returns none", 5, 10, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewBuildStore(nil)
			b := s.Create("id-offset", "s", "u", "")
			for i := 0; i < tc.total; i++ {
				b.AppendLog("line")
			}
			snap := b.Snapshot()
			// Simulate the poll handler offset logic.
			if tc.offset > 0 && tc.offset < len(snap.Logs) {
				snap.Logs = snap.Logs[tc.offset:]
			} else if tc.offset >= len(snap.Logs) {
				snap.Logs = nil
			}
			if len(snap.Logs) != tc.wantLen {
				t.Errorf("len(Logs) = %d; want %d", len(snap.Logs), tc.wantLen)
			}
		})
	}
}

// ── store CRUD tests ──────────────────────────────────────────────────────────

func TestBuildStoreGetList(t *testing.T) {
	t.Run("Get returns build by id", func(t *testing.T) {
		s := NewBuildStore(nil)
		s.Create("a", "s1", "url1", "")
		if _, ok := s.Get("a"); !ok {
			t.Fatal("Get(a): not found")
		}
	})

	t.Run("Get returns false for unknown id", func(t *testing.T) {
		s := NewBuildStore(nil)
		if _, ok := s.Get("missing"); ok {
			t.Fatal("Get(missing): expected not found")
		}
	})

	t.Run("List returns all builds", func(t *testing.T) {
		s := NewBuildStore(nil)
		s.Create("a", "s1", "url1", "")
		s.Create("b", "s2", "url2", "")
		if got := len(s.List()); got != 2 {
			t.Fatalf("List len = %d; want 2", got)
		}
	})
}

func TestBuildStoreDelete(t *testing.T) {
	t.Run("deletes from memory and db", func(t *testing.T) {
		db := newMemDB()
		s := NewBuildStore(db)
		b := s.Create("del-1", "default", "https://example.com", "")
		// ISO file doesn't exist on disk — Delete must not error.
		b.setCompleted("/tmp/nonexistent.iso", s)

		if !s.Delete("del-1") {
			t.Fatal("Delete returned false; want true")
		}
		if _, ok := s.Get("del-1"); ok {
			t.Fatal("build still in memory after Delete")
		}
		if _, ok := db.get("del-1"); ok {
			t.Fatal("build still in DB after Delete")
		}
	})

	t.Run("second delete returns false", func(t *testing.T) {
		s := NewBuildStore(nil)
		s.Create("del-2", "s", "u", "")
		s.Delete("del-2")
		if s.Delete("del-2") {
			t.Fatal("second Delete returned true; want false")
		}
	})

	t.Run("delete unknown id returns false", func(t *testing.T) {
		s := NewBuildStore(nil)
		if s.Delete("nope") {
			t.Fatal("Delete(unknown) returned true; want false")
		}
	})
}

// ── startup loading tests ─────────────────────────────────────────────────────

func TestNewBuildStoreLoadsFromDB(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		seedStatus BuildStatus
		wantStatus BuildStatus
		wantErrMsg string
	}{
		{
			name:       "completed build is restored unchanged",
			seedStatus: BuildStatusCompleted,
			wantStatus: BuildStatusCompleted,
		},
		{
			name:       "failed build is restored unchanged",
			seedStatus: BuildStatusFailed,
			wantStatus: BuildStatusFailed,
		},
		{
			name:       "running build is marked failed on restart",
			seedStatus: BuildStatusRunning,
			wantStatus: BuildStatusFailed,
			wantErrMsg: "interrupted by server restart",
		},
		{
			name:       "pending build is marked failed on restart",
			seedStatus: BuildStatusPending,
			wantStatus: BuildStatusFailed,
			wantErrMsg: "interrupted by server restart",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newMemDB()
			_ = db.SaveIsoBuild(IsoBuildRecord{
				ID: "build-1", SecretName: "sec", ServerURL: "url",
				Status: tc.seedStatus, StartedAt: now,
			})

			s := NewBuildStore(db)

			b, ok := s.Get("build-1")
			if !ok {
				t.Fatal("build-1 not loaded from DB")
			}
			snap := b.Snapshot()
			if snap.Status != tc.wantStatus {
				t.Errorf("status = %q; want %q", snap.Status, tc.wantStatus)
			}
			if tc.wantErrMsg != "" && snap.Error != tc.wantErrMsg {
				t.Errorf("Error = %q; want %q", snap.Error, tc.wantErrMsg)
			}
			// DB record must also reflect the updated status.
			if r, _ := db.get("build-1"); r.Status != tc.wantStatus {
				t.Errorf("db status = %q; want %q", r.Status, tc.wantStatus)
			}
		})
	}
}
