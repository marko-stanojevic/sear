package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/marko-stanojevic/sear/internal/common"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// HandleWS upgrades GET /api/v1/ws to a WebSocket connection.
// Authentication uses the JWT bearer token passed as ?token=<jwt> query param
// (WebSocket clients cannot set arbitrary headers during the handshake in all
// environments, so the query param fallback is supported here).
func (e *Env) HandleWS(w http.ResponseWriter, r *http.Request) {
	clientID, err := e.clientIDFromToken(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	client, ok := e.Store.GetClient(clientID)
	if !ok {
		writeError(w, http.StatusNotFound, "client not found")
		return
	}

	ws, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	conn := newWSConn(clientID, ws)
	e.Hub.register(conn)

	client.Status = common.ClientStatusConnected
	client.IPAddress = requestIP(r)
	client.LastActivityAt = time.Now()
	_ = e.Store.SaveClient(client)

	defer func() {
		e.Hub.unregister(clientID)
		if c, ok := e.Store.GetClient(clientID); ok {
			if c.Status == common.ClientStatusConnected {
				c.Status = common.ClientStatusOffline
			}
			_ = e.Store.SaveClient(c)
		}
		ws.Close()
	}()

	// Push playbook immediately if one is assigned.
	e.pushPlaybookIfAssigned(clientID)

	// Read loop.
	if err := ws.SetReadDeadline(time.Now().Add(90 * time.Second)); err != nil {
		return
	}
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(90 * time.Second))
	})

	// Send pings every 30 s so the client resets its read deadline and replies
	// with a pong that resets ours.  WriteControl is safe to call concurrently.
	pingDone := make(chan struct{})
	defer close(pingDone)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-pingDone:
				return
			}
		}
	}()

	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if err := ws.SetReadDeadline(time.Now().Add(90 * time.Second)); err != nil {
			return
		}
		e.handleWSMessage(clientID, data)
	}
}

// pushPlaybookIfAssigned sends a WSMsgPlaybook message to a connected client
// if it has a pending, running, or rebooting deployment.
// Safe to call from the admin assign handler while the client is connected.
func (e *Env) pushPlaybookIfAssigned(clientID string) {
	client, ok := e.Store.GetClient(clientID)
	if !ok || client.PlaybookID == "" {
		return
	}

	dep, hasDep := e.Store.GetActiveDeploymentForClient(clientID)

	pb, ok := e.Store.GetPlaybook(client.PlaybookID)
	if !ok {
		return
	}

	var depID string
	resumeStep := 0

	if hasDep &&
		(dep.Status == common.DeploymentStatusPending ||
			dep.Status == common.DeploymentStatusRunning ||
			dep.Status == common.DeploymentStatusRebooting) {
		// Resume an existing deployment.
		depID = dep.ID
		resumeStep = dep.ResumeStepIndex
		dep.Status = common.DeploymentStatusRunning
		dep.UpdatedAt = time.Now()
		_ = e.Store.SaveDeployment(dep)
	} else if !hasDep ||
		dep.Status == common.DeploymentStatusDone ||
		dep.Status == common.DeploymentStatusFailed {
		// Start a fresh deployment.
		depID = uuid.New().String()
		newDep := &common.DeploymentState{
			ID:              depID,
			ClientID:        clientID,
			PlaybookID:      client.PlaybookID,
			Status:          common.DeploymentStatusRunning,
			ResumeStepIndex: 0,
			StartedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		_ = e.Store.SaveDeployment(newDep)
	} else {
		return
	}

	client.Status = common.ClientStatusDeploying
	_ = e.Store.SaveClient(client)

	pbName := pb.Name
	if pb.Playbook != nil && pb.Playbook.Name != "" {
		pbName = pb.Playbook.Name
	}
	e.appendDeploymentLog(depID, "", 0, common.LogLevelInfo,
		fmt.Sprintf("starting playbook %q (deployment %s, resume step %d)", pbName, depID, resumeStep))

	e.Hub.Send(clientID, common.WSMessage{
		Type:      common.WSMsgPlaybook,
		Timestamp: time.Now(),
		Data: common.WSPlaybookData{
			DeploymentID:     depID,
			Playbook:         pb.Playbook,
			ResumeStepIndex:  resumeStep,
			Secrets:          e.Store.AllSecrets(),
			ArtifactsBaseURL: e.ServerURL + "/artifacts",
		},
	})
}

// handleWSMessage dispatches an inbound WebSocket message from a client.
func (e *Env) handleWSMessage(clientID string, data []byte) {
	var envelope struct {
		Type common.WSMessageType `json:"type"`
		Data json.RawMessage      `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return
	}

	switch envelope.Type {
	case common.WSMsgLog:
		var d common.WSLogData
		if json.Unmarshal(envelope.Data, &d) != nil {
			return
		}
		entry := &common.LogEntry{
			DeploymentID: d.DeploymentID,
			JobName:      d.JobName,
			StepIndex:    d.StepIndex,
			Level:        d.Level,
			Message:      d.Message,
			Timestamp:    time.Now(),
		}
		_ = e.Store.AppendLogs([]*common.LogEntry{entry})

	case common.WSMsgStepStart:
		var d common.WSStepData
		if json.Unmarshal(envelope.Data, &d) != nil {
			return
		}
		stepName := d.StepName
		if stepName == "" {
			stepName = fmt.Sprintf("step-%d", d.StepIndex)
		}
		e.appendDeploymentLog(d.DeploymentID, d.JobName, d.StepIndex, common.LogLevelInfo,
			fmt.Sprintf("[%s / %s] starting", d.JobName, stepName))
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusRunning
			dep.ResumeStepIndex = d.StepIndex
		})

	case common.WSMsgStepComplete:
		var d common.WSStepData
		if json.Unmarshal(envelope.Data, &d) != nil {
			return
		}
		stepName := d.StepName
		if stepName == "" {
			stepName = fmt.Sprintf("step-%d", d.StepIndex)
		}
		e.appendDeploymentLog(d.DeploymentID, d.JobName, d.StepIndex, common.LogLevelInfo,
			fmt.Sprintf("[%s / %s] completed", d.JobName, stepName))
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.ResumeStepIndex = d.StepIndex + 1
		})

	case common.WSMsgStepFailed:
		var d common.WSStepData
		if json.Unmarshal(envelope.Data, &d) != nil {
			return
		}
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusFailed
			dep.ErrorDetail = d.Error
		})

	case common.WSMsgReboot:
		var d common.WSRebootData
		if json.Unmarshal(envelope.Data, &d) != nil {
			return
		}
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusRebooting
			dep.ResumeStepIndex = d.ResumeStepIndex
		})

	case common.WSMsgDeployDone:
		var d common.WSStepData
		if json.Unmarshal(envelope.Data, &d) != nil {
			return
		}
		pbName := "playbook"
		if dep, ok := e.Store.GetDeployment(d.DeploymentID); ok {
			if pb, ok := e.Store.GetPlaybook(dep.PlaybookID); ok {
				if pb.Playbook != nil && pb.Playbook.Name != "" {
					pbName = pb.Playbook.Name
				} else if pb.Name != "" {
					pbName = pb.Name
				}
			}
		}
		e.appendDeploymentLog(d.DeploymentID, "", 0, common.LogLevelInfo,
			fmt.Sprintf("playbook %q completed successfully", pbName))
		now := time.Now()
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusDone
			dep.FinishedAt = &now
		})
		if c, ok := e.Store.GetClient(clientID); ok {
			c.Status = common.ClientStatusDone
			_ = e.Store.SaveClient(c)
		}

	case common.WSMsgDeployFailed:
		var d common.WSStepData
		if json.Unmarshal(envelope.Data, &d) != nil {
			return
		}
		pbName := "playbook"
		if dep, ok := e.Store.GetDeployment(d.DeploymentID); ok {
			if pb, ok := e.Store.GetPlaybook(dep.PlaybookID); ok {
				if pb.Playbook != nil && pb.Playbook.Name != "" {
					pbName = pb.Playbook.Name
				} else if pb.Name != "" {
					pbName = pb.Name
				}
			}
		}
		msg := fmt.Sprintf("playbook %q failed", pbName)
		if d.Error != "" {
			msg = fmt.Sprintf("%s: %s", msg, d.Error)
		}
		e.appendDeploymentLog(d.DeploymentID, d.JobName, d.StepIndex, common.LogLevelError, msg)
		now := time.Now()
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusFailed
			dep.ErrorDetail = d.Error
			dep.FinishedAt = &now
		})
		if c, ok := e.Store.GetClient(clientID); ok {
			c.Status = common.ClientStatusFailed
			_ = e.Store.SaveClient(c)
		}

	case common.WSMsgPong:
		// keepalive — handled by SetPongHandler
	}
}

func (e *Env) updateDeploy(depID string, fn func(*common.DeploymentState)) {
	dep, ok := e.Store.GetDeployment(depID)
	if !ok {
		return
	}
	fn(dep)
	dep.UpdatedAt = time.Now()
	_ = e.Store.SaveDeployment(dep)
}

func (e *Env) appendDeploymentLog(deploymentID, jobName string, stepIndex int, level common.LogLevel, message string) {
	if deploymentID == "" || message == "" {
		return
	}
	_ = e.Store.AppendLogs([]*common.LogEntry{{
		DeploymentID: deploymentID,
		JobName:      jobName,
		StepIndex:    stepIndex,
		Level:        level,
		Message:      message,
		Timestamp:    time.Now(),
	}})
}

// ── Status UI ─────────────────────────────────────────────────────────────────

// HandleStatus returns a JSON summary of all clients and deployments.
func (e *Env) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	writeJSON(w, http.StatusOK, StatusResponse{
		Clients:     e.Store.ListClients(),
		Deployments: e.Store.ListDeployments(),
	})
}

// HandleStatusUI returns a live HTML dashboard.
func (e *Env) HandleStatusUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	fmt.Fprint(w, statusHTML)
}

const statusHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sear — Deployment Status</title>
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  body{font-family:'Segoe UI',system-ui,sans-serif;background:#0d1117;color:#e6edf3;min-height:100vh}
  header{background:#161b22;border-bottom:1px solid #30363d;padding:0 24px;display:flex;align-items:stretch;gap:0}
  .header-brand{display:flex;align-items:center;gap:12px;padding:16px 0}
  header h1{font-size:1.25rem;font-weight:600}
  nav{display:flex;gap:2px;align-items:stretch;margin-left:24px}
  nav a{color:#8b949e;text-decoration:none;padding:0 14px;display:flex;align-items:center;font-size:.875rem;border-bottom:2px solid transparent}
  nav a:hover{color:#e6edf3}
  nav a.active{color:#e6edf3;border-bottom-color:#f78166}
  .header-right{margin-left:auto;display:flex;align-items:center}
  .badge{background:#238636;color:#fff;font-size:.75rem;padding:2px 8px;border-radius:12px}
	.container{max-width:1200px;margin:0 auto;padding:24px}
  .meta{color:#8b949e;font-size:.875rem;margin-bottom:20px;display:flex;align-items:center;gap:16px}
	.filters{display:grid;grid-template-columns:minmax(220px,1fr) 180px 160px auto;gap:10px;margin-bottom:14px}
	.filters input,.filters select{background:#0d1117;border:1px solid #30363d;color:#e6edf3;padding:8px 10px;border-radius:6px;font-size:.85rem;outline:none}
	.filters input:focus,.filters select:focus{border-color:#58a6ff}
	.list{background:#161b22;border:1px solid #30363d;border-radius:8px;overflow:hidden}
	.list-head,.list-row{display:grid;grid-template-columns:1.2fr 1fr .7fr .9fr .9fr .8fr .8fr 1fr 46px;gap:10px;align-items:center;padding:10px 12px}
	.list-head{background:#0d1117;color:#8b949e;font-size:.75rem;text-transform:uppercase;letter-spacing:.04em;border-bottom:1px solid #30363d;position:sticky;top:0;z-index:2}
	.list-row{border-bottom:1px solid #21262d;font-size:.875rem}
	.list-row:last-child{border-bottom:none}
	.cell-main{display:flex;flex-direction:column;gap:2px;min-width:0}
	.name{font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
	.sub{font-size:.75rem;color:#8b949e;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
	.mono{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:inherit}
	.pill{display:inline-flex;align-items:center;gap:6px;font-size:inherit;padding:3px 10px;border-radius:12px;font-weight:500}
  .pill.running{background:#1f6feb33;color:#58a6ff}
  .pill.done{background:#23863633;color:#3fb950}
  .pill.failed{background:#da363333;color:#f85149}
  .pill.rebooting{background:#9e6a0333;color:#e3b341}
  .pill.pending,.pill.registered,.pill.offline{background:#21262d;color:#8b949e}
  .pill.connected,.pill.deploying{background:#1f6feb22;color:#79c0ff}
  .dot{width:7px;height:7px;border-radius:50%;background:currentColor}
	button{background:#21262d;border:1px solid #30363d;color:#e6edf3;padding:6px 14px;border-radius:6px;cursor:pointer;font-size:.85rem}
  button:hover{background:#30363d}
	.card-top-right{display:flex;align-items:flex-start;gap:8px}
  .card-menu{position:relative}
  .menu-btn{background:transparent;border:none;color:#8b949e;padding:2px 7px;border-radius:4px;cursor:pointer;font-size:1.1rem;line-height:1}
  .menu-btn:hover{background:#30363d;color:#e6edf3}
  .menu-dropdown{display:none;position:absolute;right:0;top:calc(100% + 4px);background:#161b22;border:1px solid #30363d;border-radius:6px;min-width:150px;z-index:10;box-shadow:0 8px 24px #010409cc}
  .menu-dropdown.open{display:block}
  .menu-item{display:block;width:100%;text-align:left;background:transparent;border:none;color:#f85149;padding:8px 14px;cursor:pointer;font-size:.85rem;border-radius:6px}
  .menu-item:hover{background:#da363320}
  .empty{text-align:center;color:#8b949e;padding:60px 20px}
	@media (max-width: 1100px){
		.filters{grid-template-columns:1fr 1fr;}
		.list-head{display:none}
		.list-row{grid-template-columns:1fr;gap:6px;padding:12px}
		.list-row > div{display:flex;justify-content:space-between;gap:12px}
		.list-row .cell-main{display:block}
		.list-row .cell-main .name{margin-bottom:2px}
		.mobile-label{display:inline;color:#8b949e;font-size:.75rem}
	}
	@media (min-width: 1101px){
		.mobile-label{display:none}
	}
  #login-overlay{display:none;position:fixed;inset:0;background:#0d1117cc;backdrop-filter:blur(4px);align-items:center;justify-content:center;z-index:100}
  #login-overlay.show{display:flex}
  #login-box{background:#161b22;border:1px solid #30363d;border-radius:10px;padding:32px;min-width:320px}
  #login-box h2{font-size:1.1rem;margin-bottom:20px;color:#e6edf3}
  #login-box label{display:block;font-size:.8rem;color:#8b949e;margin-bottom:4px}
  #login-box input{width:100%;background:#0d1117;border:1px solid #30363d;color:#e6edf3;padding:8px 10px;border-radius:6px;font-size:.9rem;margin-bottom:14px;outline:none}
  #login-box input:focus{border-color:#58a6ff}
  #login-error{color:#f85149;font-size:.8rem;margin-bottom:10px;min-height:1em}
  #login-box button{width:100%;background:#238636;border:none;color:#fff;padding:9px;border-radius:6px;font-size:.9rem;cursor:pointer}
  #login-box button:hover{background:#2ea043}
</style>
</head>
<body>
<div id="login-overlay">
  <div id="login-box">
    <h2>🔒 Sear Root Login</h2>
    <label>Username</label>
    <input id="lu" type="text" value="root" readonly>
    <label>Password</label>
    <input id="lp" type="password" autocomplete="current-password" placeholder="root password">
    <div id="login-error"></div>
    <button onclick="doLogin()">Sign in</button>
  </div>
</div>
<header>
	<div class="header-brand">
		<h1>&#x26A1; Sear Daemon</h1>
		<span class="badge">LIVE</span>
	</div>
	<nav>
		<a href="/ui" class="active">Clients</a>
		<a href="/ui/secrets">Secrets</a>
		<a href="/ui/playbooks">Playbooks</a>
		<a href="/ui/deployments">Deployments</a>
	</nav>
	<div class="header-right">
		<button onclick="logout()">Sign out</button>
	</div>
</header>
<div class="container">
	<div class="meta">
		<span id="counts">Loading&#x2026;</span>
		<button onclick="load()">&#x21BB; Refresh</button>
	</div>
	<div class="filters">
		<input id="f-query" type="text" placeholder="Search hostname, OS, vendor, model, IP">
		<select id="f-status">
			<option value="">All statuses</option>
			<option value="registered">registered</option>
			<option value="connected">connected</option>
			<option value="deploying">deploying</option>
			<option value="running">running</option>
			<option value="rebooting">rebooting</option>
			<option value="done">done</option>
			<option value="failed">failed</option>
			<option value="offline">offline</option>
			<option value="pending">pending</option>
		</select>
		<select id="f-platform">
			<option value="">All platforms</option>
			<option value="linux">linux</option>
			<option value="windows">windows</option>
			<option value="mac">mac</option>
		</select>
		<button onclick="clearFilters()">Clear filters</button>
	</div>
  <div id="root"></div>
</div>
<script>
function esc(s){const d=document.createElement('div');d.textContent=String(s);return d.innerHTML;}
function authHeader(){const c=sessionStorage.getItem('sear_creds');return c?'Basic '+c:null;}
function showLogin(msg){
  document.getElementById('login-error').textContent=msg||'';
  document.getElementById('lp').value='';
  document.getElementById('login-overlay').classList.add('show');
  setTimeout(()=>document.getElementById('lp').focus(),50);
}
function hideLogin(){document.getElementById('login-overlay').classList.remove('show');}
function logout(){sessionStorage.removeItem('sear_creds');showLogin('');}
async function doLogin(){
  const u=document.getElementById('lu').value;
  const p=document.getElementById('lp').value;
  if(!p){document.getElementById('login-error').textContent='Password required';return;}
  const creds=btoa(u+':'+p);
	const r=await fetch('/api/v1/status',{headers:{Authorization:'Basic '+creds}});
  if(r.status===401){document.getElementById('login-error').textContent='Invalid password';return;}
  sessionStorage.setItem('sear_creds',creds);
  hideLogin();
  load();
}
document.getElementById('lp').addEventListener('keydown',e=>{if(e.key==='Enter')doLogin();});
async function removeClient(id, hostname) {
  if (!confirm('Remove client "' + hostname + '"?\nThis cannot be undone.')) return;
  const creds = sessionStorage.getItem('sear_creds');
  const headers = {'Content-Type':'application/json'};
  if (creds) headers['Authorization'] = 'Basic ' + creds;
	const r = await fetch('/api/v1/clients/' + encodeURIComponent(id), {method:'DELETE', headers});
  if (r.status === 401) { showLogin('Session expired — sign in again'); return; }
  if (!r.ok) { alert('Failed to remove client: ' + r.status); return; }
  load();
}
document.addEventListener('click', function(e) {
  const menuBtn = e.target.closest('.menu-btn');
  if (menuBtn) {
    e.stopPropagation();
    const dropdown = menuBtn.nextElementSibling;
    const wasOpen = dropdown.classList.contains('open');
    document.querySelectorAll('.menu-dropdown.open').forEach(d => d.classList.remove('open'));
    if (!wasOpen) dropdown.classList.add('open');
    return;
  }
  const item = e.target.closest('.menu-item');
  if (item && item.dataset.action === 'remove') {
    document.querySelectorAll('.menu-dropdown.open').forEach(d => d.classList.remove('open'));
    removeClient(item.dataset.id, item.dataset.hostname);
    return;
  }
  document.querySelectorAll('.menu-dropdown.open').forEach(d => d.classList.remove('open'));
});
let latestClients = [];
let latestDeps = {};

function filterClients(clients, deps) {
	const q = (document.getElementById('f-query').value || '').trim().toLowerCase();
	const status = (document.getElementById('f-status').value || '').trim().toLowerCase();
	const platform = (document.getElementById('f-platform').value || '').trim().toLowerCase();
	return clients.filter(c => {
		const dep = deps[c.id];
		const s = ((dep ? dep.status : c.status) || '').toLowerCase();
		const p = (c.platform || '').toLowerCase();
		const os = (c.os || (c.metadata && c.metadata.os) || (c.metadata && c.metadata.type) || (c.metadata && c.metadata.os_type) || '');
		const vendor = (c.vendor || (c.metadata && c.metadata.vendor) || '');
		const model = (c.model || (c.metadata && c.metadata.model) || '');
		const ip = (c.ip_address || '');
		const haystack = (c.hostname || c.id || '') + ' ' + os + ' ' + vendor + ' ' + model + ' ' + ip;
		if (status && s !== status) return false;
		if (platform && p !== platform) return false;
		if (q && !haystack.toLowerCase().includes(q)) return false;
		return true;
	});
}

function render() {
	const root = document.getElementById('root');
	if (!latestClients.length) {
		root.innerHTML = '<div class="empty">No clients registered yet.</div>';
		document.getElementById('counts').textContent = 'Clients: 0';
		return;
	}

	const filtered = filterClients(latestClients, latestDeps);

	let conn = 0, active = 0;
	filtered.forEach(c => {
		const dep = latestDeps[c.id];
		if (c.status === 'connected' || c.status === 'deploying') conn++;
		if (dep && dep.status === 'running') active++;
	});

	if (!filtered.length) {
		root.innerHTML = '<div class="empty">No clients match your filters.</div>';
		document.getElementById('counts').textContent = 'Shown: 0 / ' + latestClients.length + ' · Connected: 0 · Deploying: 0';
		return;
	}

	const rows = filtered.map(c => {
		const dep = latestDeps[c.id];
		const s = dep ? dep.status : c.status;
		const os = c.os || (c.metadata && c.metadata.os) || (c.metadata && c.metadata.type) || (c.metadata && c.metadata.os_type) || '-';
		const vendor = c.vendor || (c.metadata && c.metadata.vendor) || '-';
		const model = c.model || (c.metadata && c.metadata.model) || '-';
		const step = dep ? '#' + dep.resume_step_index : '-';
		const when = c.last_activity_at ? new Date(c.last_activity_at).toLocaleString() : '-';
		return '<div class="list-row">' +
			'<div class="cell-main"><div class="name">' + esc(c.hostname || c.id) + '</div><div class="sub mono">' + esc(c.id || '-') + '</div></div>' +
			'<div><span class="mobile-label">OS</span><span>' + esc(os) + '</span></div>' +
			'<div><span class="mobile-label">Platform</span><span>' + esc(c.platform || '-') + '</span></div>' +
			'<div><span class="mobile-label">Vendor</span><span>' + esc(vendor) + '</span></div>' +
			'<div><span class="mobile-label">Model</span><span>' + esc(model) + '</span></div>' +
			'<div><span class="mobile-label">IP</span><span class="mono">' + esc(c.ip_address || '-') + '</span></div>' +
			'<div><span class="mobile-label">Status</span><span class="pill ' + s + '"><span class="dot"></span>' + esc(s || '-') + '</span></div>' +
			'<div><span class="mobile-label">Last</span><span>' + esc(when) + '</span><div class="sub">Step: ' + esc(step) + '</div></div>' +
			'<div class="card-top-right"><div class="card-menu">' +
				'<button class="menu-btn" title="Actions">⋮</button>' +
				'<div class="menu-dropdown">' +
					'<button class="menu-item" data-action="remove" data-id="' + esc(c.id) + '" data-hostname="' + esc(c.hostname || c.id) + '">✕ Remove client</button>' +
				'</div>' +
			'</div></div>' +
		'</div>';
	}).join('');

	root.innerHTML = '<div class="list">' +
		'<div class="list-head">' +
			'<div>Client</div><div>OS</div><div>Platform</div><div>Vendor</div><div>Model</div><div>IP</div><div>Status</div><div>Last Activity</div><div></div>' +
		'</div>' +
		rows +
	'</div>';

	document.getElementById('counts').textContent =
		'Shown: ' + filtered.length + ' / ' + latestClients.length + ' · Connected: ' + conn + ' · Deploying: ' + active;
}

function clearFilters() {
	document.getElementById('f-query').value = '';
	document.getElementById('f-status').value = '';
	document.getElementById('f-platform').value = '';
	render();
}

async function load() {
	const r = await fetch('/api/v1/status');
  if (r.status === 401) {
    const root = document.getElementById('root');
    root.innerHTML = '<div class="empty">Unauthorized. Reload the page to sign in.</div>';
    return;
  }
  const d = await r.json();
  latestDeps = {};
  (d.deployments||[]).forEach(dep => { latestDeps[dep.client_id] = dep; });
  latestClients = d.clients || [];
  render();
}
['f-query','f-status','f-platform'].forEach(id => {
	const el = document.getElementById(id);
	el.addEventListener(id === 'f-query' ? 'input' : 'change', render);
});
load();
setInterval(load, 10000);
</script>
</body>
</html>`
