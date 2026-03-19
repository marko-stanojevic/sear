package handlers

import (
	"html/template"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/server/store"
)

const uiPageSize = 20

// ── Navigation ────────────────────────────────────────────────────────────────

type navItem struct {
	Label  string
	URL    string
	Active bool
}

func buildNav(active string) []navItem {
	return []navItem{
		{"Home", "/ui", active == "Home"},
		{"Agents", "/ui/agents", active == "Agents"},
		{"Vault", "/ui/vault", active == "Vault"},
		{"Playbooks", "/ui/playbooks", active == "Playbooks"},
		{"Deployments", "/ui/deployments", active == "Deployments"},
		{"Artifacts", "/ui/artifacts", active == "Artifacts"},
	}
}

// ── Page data types ───────────────────────────────────────────────────────────

type pageData struct {
	Title    string
	NavItems []navItem
	Search   string
}

func newPage(title, active, search string) pageData {
	return pageData{Title: title, NavItems: buildNav(active), Search: search}
}

// ── Partial data types ────────────────────────────────────────────────────────

type homeStatsData struct {
	Agents      int
	Playbooks   int
	Deployments int
	Artifacts   int
}

type agentsTableData struct {
	Rows        []agentRow
	Query       string
	Page        int
	TotalPages  int
	Total       int
	Shown       int
	Connected   int
	Deploying   int
}

type agentRow struct {
	*common.Agent
	DeployStep string
	StatusStr  string
}

type artifactsTableData struct {
	Rows       []*common.Artifact
	Query      string
	Page       int
	TotalPages int
	Total      int
}

type deploymentsTableData struct {
	Rows       []*common.DeploymentState
	Query      string
	Page       int
	TotalPages int
	Total      int
}

type playbooksTableData struct {
	Rows       []playbookRow
	Agents     []*common.Agent
	Query      string
	Page       int
	TotalPages int
	Total      int
}

type playbookRow struct {
	*store.PlaybookRecord
	Jobs  int
	Steps int
}

type secretsTableData struct {
	Names      []string
	Query      string
	Page       int
	TotalPages int
	Total      int
}

type logsData struct {
	DeploymentID string
	Entries      []*common.LogEntry
}

// ── Template functions ────────────────────────────────────────────────────────

var tmplFuncs = template.FuncMap{
	"formatTime": func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.Local().Format("Jan 2, 2006 15:04:05")
	},
	"formatTimePtr": func(t *time.Time) string {
		if t == nil {
			return "-"
		}
		return t.Local().Format("Jan 2, 2006 15:04:05")
	},
	"formatSize": func(n int64) string {
		switch {
		case n >= 1024*1024:
			return strconv.FormatFloat(float64(n)/(1024*1024), 'f', 1, 64) + " MB"
		case n >= 1024:
			return strconv.FormatFloat(float64(n)/1024, 'f', 1, 64) + " KB"
		default:
			return strconv.FormatInt(n, 10) + " B"
		}
	},
	"statusClass": func(s string) string {
		switch strings.ToLower(s) {
		case "done":
			return "success"
		case "failed":
			return "warning"
		case "running":
			return "success"
		case "rebooting":
			return "warning"
		case "connected", "deploying":
			return "success"
		default:
			return "muted"
		}
	},
	"policyClass": func(p string) string {
		switch p {
		case "public":
			return "badge-outline"
		case "restricted":
			return "badge-warning"
		default:
			return "badge-muted"
		}
	},
	"policyLabel": func(p string) string {
		switch p {
		case "public":
			return "Public"
		case "restricted":
			return "Restricted"
		default:
			return "Authenticated"
		}
	},
	"or": func(a, b string) string {
		if a != "" {
			return a
		}
		return b
	},
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"seq": func(n int) []int {
		r := make([]int, n)
		for i := range r {
			r[i] = i + 1
		}
		return r
	},
}

// ── Rendering helpers ─────────────────────────────────────────────────────────

func renderPage(w http.ResponseWriter, page string, data any) {
	t, err := template.New("").Funcs(tmplFuncs).ParseFS(uiFS,
		"ui/templates/layout.html",
		"ui/templates/pages/"+page+".html",
	)
	if err != nil {
		http.Error(w, "template parse error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	setSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		// headers already sent, nothing we can do
		_ = err
	}
}

func renderPartialTemplate(w http.ResponseWriter, name string, data any) {
	filename := name + ".html"
	t, err := template.New(filename).Funcs(tmplFuncs).ParseFS(uiFS, "ui/templates/partials/"+filename)
	if err != nil {
		http.Error(w, "template parse error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	setSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	if err := t.Execute(w, data); err != nil {
		_ = err
	}
}

func pageParam(r *http.Request) int {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if p < 1 {
		p = 1
	}
	return p
}

func totalPages(total, size int) int {
	if total == 0 {
		return 1
	}
	return int(math.Ceil(float64(total) / float64(size)))
}

func paginate[T any](items []T, page, size int) []T {
	start := (page - 1) * size
	if start >= len(items) {
		return nil
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// ── UI page handlers ──────────────────────────────────────────────────────────

func (e *Handler) HandleHomeUI(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "home", newPage("Dashboard", "Home", ""))
}

func (e *Handler) HandleAgentsUI(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	renderPage(w, "agents", newPage("Agents", "Agents", q))
}

func (e *Handler) HandleArtifactsUI(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "artifacts", newPage("Artifacts", "Artifacts", ""))
}

func (e *Handler) HandlePlaybooksUI(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "playbooks", newPage("Playbooks", "Playbooks", ""))
}

func (e *Handler) HandleDeploymentsUI(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "deployments", newPage("Deployments", "Deployments", ""))
}

func (e *Handler) HandleVaultUI(w http.ResponseWriter, r *http.Request) {
	renderPage(w, "vault", newPage("Vault", "Vault", ""))
}

// ── Partial handlers (all behind RequireRootAuth) ─────────────────────────────

func (e *Handler) HandlePartialHomeStats(w http.ResponseWriter, r *http.Request) {
	agents, deployments := e.Store.ListAgents(), e.Store.ListDeployments()
	if e.Service != nil {
		agents, deployments = e.Service.StatusSnapshot()
	}
	running := 0
	for _, d := range deployments {
		if d.Status == common.DeploymentStatusRunning {
			running++
		}
	}
	renderPartialTemplate(w, "home-stats", homeStatsData{
		Agents:      len(agents),
		Playbooks:   len(e.Store.ListPlaybooks()),
		Deployments: running,
		Artifacts:   len(e.Store.ListArtifacts()),
	})
}

func (e *Handler) HandlePartialAgents(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	page := pageParam(r)

	agents, deployments := e.Store.ListAgents(), e.Store.ListDeployments()
	if e.Service != nil {
		agents, deployments = e.Service.StatusSnapshot()
	}
	depByAgent := make(map[string]*common.DeploymentState, len(deployments))
	for _, d := range deployments {
		depByAgent[d.AgentID] = d
	}

	// Filter
	var filtered []*common.Agent
	for _, a := range agents {
		if q != "" {
			hay := strings.ToLower((a.Hostname + " " + a.OS + " " + a.Vendor + " " + a.Model + " " + a.IPAddress + " " + string(a.ID)))
			if !strings.Contains(hay, q) {
				continue
			}
		}
		filtered = append(filtered, a)
	}

	connected, deploying := 0, 0
	for _, a := range filtered {
		if a.Status == common.AgentStatusConnected {
			connected++
		}
		if a.Status == common.AgentStatusDeploying {
			deploying++
		}
	}

	paged := paginate(filtered, page, uiPageSize)
	rows := make([]agentRow, len(paged))
	for i, a := range paged {
		step := "-"
		status := string(a.Status)
		if dep, ok := depByAgent[a.ID]; ok {
			step = "#" + strconv.Itoa(dep.ResumeStepIndex)
			status = string(dep.Status)
		}
		rows[i] = agentRow{Agent: a, DeployStep: step, StatusStr: status}
	}

	renderPartialTemplate(w, "agents-table", agentsTableData{
		Rows:       rows,
		Query:      r.URL.Query().Get("q"),
		Page:       page,
		TotalPages: totalPages(len(filtered), uiPageSize),
		Total:      len(agents),
		Shown:      len(filtered),
		Connected:  connected,
		Deploying:  deploying,
	})
}

func (e *Handler) HandlePartialArtifacts(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	page := pageParam(r)

	all := e.Store.ListArtifacts()
	var filtered []*common.Artifact
	for _, a := range all {
		if q != "" {
			hay := strings.ToLower(a.ID + " " + a.Name + " " + a.FileName)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		filtered = append(filtered, a)
	}

	renderPartialTemplate(w, "artifacts-table", artifactsTableData{
		Rows:       paginate(filtered, page, uiPageSize),
		Query:      r.URL.Query().Get("q"),
		Page:       page,
		TotalPages: totalPages(len(filtered), uiPageSize),
		Total:      len(all),
	})
}

func (e *Handler) HandlePartialDeployments(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	page := pageParam(r)

	all := e.Store.ListDeployments()
	var filtered []*common.DeploymentState
	for _, d := range all {
		if q != "" {
			hay := strings.ToLower(d.ID + " " + d.AgentID + " " + d.PlaybookID + " " + d.Hostname + " " + d.PlaybookName)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		filtered = append(filtered, d)
	}

	renderPartialTemplate(w, "deployments-table", deploymentsTableData{
		Rows:       paginate(filtered, page, uiPageSize),
		Query:      r.URL.Query().Get("q"),
		Page:       page,
		TotalPages: totalPages(len(filtered), uiPageSize),
		Total:      len(all),
	})
}

func (e *Handler) HandlePartialDeploymentLogs(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/ui/partials/deployments/")
	id = strings.TrimSuffix(id, "/logs")
	entries := e.Store.GetLogsForDeployment(id)
	renderPartialTemplate(w, "deployments-logs", logsData{DeploymentID: id, Entries: entries})
}

func (e *Handler) HandlePartialPlaybooks(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	page := pageParam(r)

	all := e.Store.ListPlaybooks()
	agents := e.Store.ListAgents()

	var filtered []playbookRow
	for _, pb := range all {
		if q != "" {
			hay := strings.ToLower(pb.ID + " " + pb.Name + " " + pb.Description)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		jobs, steps := 0, 0
		if pb.Playbook != nil {
			jobs = len(pb.Playbook.Jobs)
			for _, j := range pb.Playbook.Jobs {
				steps += len(j.Steps)
			}
		}
		filtered = append(filtered, playbookRow{PlaybookRecord: pb, Jobs: jobs, Steps: steps})
	}

	renderPartialTemplate(w, "playbooks-table", playbooksTableData{
		Rows:       paginate(filtered, page, uiPageSize),
		Agents:     agents,
		Query:      r.URL.Query().Get("q"),
		Page:       page,
		TotalPages: totalPages(len(filtered), uiPageSize),
		Total:      len(all),
	})
}

func (e *Handler) HandlePartialVault(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	page := pageParam(r)

	all := e.Store.ListSecretNames()
	var filtered []string
	for _, n := range all {
		if q == "" || strings.Contains(strings.ToLower(n), q) {
			filtered = append(filtered, n)
		}
	}

	renderPartialTemplate(w, "vault-table", secretsTableData{
		Names:      paginate(filtered, page, uiPageSize),
		Query:      r.URL.Query().Get("q"),
		Page:       page,
		TotalPages: totalPages(len(filtered), uiPageSize),
		Total:      len(all),
	})
}
