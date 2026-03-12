package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/sear-project/sear/internal/common"
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
	client.LastSeenAt = time.Now()
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
	var env struct {
		Type common.WSMessageType `json:"type"`
		Data json.RawMessage      `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}

	switch env.Type {
	case common.WSMsgLog:
		var d common.WSLogData
		if json.Unmarshal(env.Data, &d) != nil {
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
		if json.Unmarshal(env.Data, &d) != nil {
			return
		}
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusRunning
			dep.ResumeStepIndex = d.StepIndex
		})

	case common.WSMsgStepComplete:
		var d common.WSStepData
		if json.Unmarshal(env.Data, &d) != nil {
			return
		}
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.ResumeStepIndex = d.StepIndex + 1
		})

	case common.WSMsgStepFailed:
		var d common.WSStepData
		if json.Unmarshal(env.Data, &d) != nil {
			return
		}
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusFailed
			dep.ErrorDetail = d.Error
		})

	case common.WSMsgReboot:
		var d common.WSRebootData
		if json.Unmarshal(env.Data, &d) != nil {
			return
		}
		e.updateDeploy(d.DeploymentID, func(dep *common.DeploymentState) {
			dep.Status = common.DeploymentStatusRebooting
			dep.ResumeStepIndex = d.ResumeStepIndex
		})

	case common.WSMsgDeployDone:
		var d common.WSStepData
		if json.Unmarshal(env.Data, &d) != nil {
			return
		}
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
		if json.Unmarshal(env.Data, &d) != nil {
			return
		}
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

// ── Status UI ─────────────────────────────────────────────────────────────────

// HandleStatus returns a JSON summary of all clients and deployments.
func (e *Env) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, StatusResponse{
		Clients:     e.Store.ListClients(),
		Deployments: e.Store.ListDeployments(),
	})
}

// HandleStatusUI returns a live HTML dashboard.
func (e *Env) HandleStatusUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
  header{background:#161b22;border-bottom:1px solid #30363d;padding:16px 24px;display:flex;align-items:center;gap:12px}
  header h1{font-size:1.25rem;font-weight:600}
  .badge{background:#238636;color:#fff;font-size:.75rem;padding:2px 8px;border-radius:12px}
  .container{max-width:1200px;margin:0 auto;padding:24px}
  .meta{color:#8b949e;font-size:.875rem;margin-bottom:20px;display:flex;align-items:center;gap:16px}
  .grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(340px,1fr));gap:16px}
  .card{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px}
  .card-header{display:flex;justify-content:space-between;align-items:flex-start;margin-bottom:12px}
  .card-title{font-weight:600;font-size:1rem}
  .card-sub{font-size:.8rem;color:#8b949e;margin-top:2px}
  .pill{display:inline-flex;align-items:center;gap:6px;font-size:.8rem;padding:3px 10px;border-radius:12px;font-weight:500}
  .pill.running{background:#1f6feb33;color:#58a6ff}
  .pill.done{background:#23863633;color:#3fb950}
  .pill.failed{background:#da363333;color:#f85149}
  .pill.rebooting{background:#9e6a0333;color:#e3b341}
  .pill.pending,.pill.registered,.pill.offline{background:#21262d;color:#8b949e}
  .pill.connected,.pill.deploying{background:#1f6feb22;color:#79c0ff}
  .dot{width:7px;height:7px;border-radius:50%;background:currentColor}
  .detail{font-size:.8rem;color:#8b949e;margin-top:6px}
  .detail span{color:#e6edf3}
  button{background:#21262d;border:1px solid #30363d;color:#e6edf3;padding:6px 14px;border-radius:6px;cursor:pointer;font-size:.85rem}
  button:hover{background:#30363d}
  .empty{text-align:center;color:#8b949e;padding:60px 20px}
</style>
</head>
<body>
<header>
  <h1>⚡ Sear Daemon</h1>
  <span class="badge">LIVE</span>
</header>
<div class="container">
  <div class="meta">
    <span id="counts">Loading…</span>
    <button onclick="load()">↻ Refresh</button>
  </div>
  <div id="root"></div>
</div>
<script>
const depByClient = {};
async function load() {
  const r = await fetch('/status');
  const d = await r.json();
  const deps = {};
  (d.deployments||[]).forEach(dep => { deps[dep.client_id] = dep; });
  const root = document.getElementById('root');
  const clients = d.clients || [];
  if (!clients.length) { root.innerHTML='<div class="empty">No clients registered yet.</div>'; return; }
  let conn=0, active=0;
  const cards = clients.map(c => {
    const dep = deps[c.id];
    const s = dep ? dep.status : c.status;
    if (c.status === 'connected' || c.status === 'deploying') conn++;
    if (dep && dep.status === 'running') active++;
	  const playbookDetail = dep
	    ? '<div class="detail">Playbook step: <span>#' + dep.resume_step_index + '</span></div>' +
	      (dep.error_detail ? '<div class="detail" style="color:#f85149">' + dep.error_detail + '</div>' : '')
	    : '';
	  return '<div class="card">' +
	    '<div class="card-header">' +
	      '<div>' +
	        '<div class="card-title">' + c.hostname + '</div>' +
	        '<div class="card-sub">' + c.platform + ' · ' + (c.platform_id || c.id.slice(0,8)) + '</div>' +
	      '</div>' +
	      '<span class="pill ' + s + '"><span class="dot"></span>' + s + '</span>' +
	    '</div>' +
	    playbookDetail +
	    '<div class="detail">Last seen: <span>' + new Date(c.last_seen_at).toLocaleString() + '</span></div>' +
	  '</div>';
  }).join('');
  document.getElementById('counts').textContent =
    'Clients: '+clients.length+' · Connected: '+conn+' · Deploying: '+active;
  root.innerHTML = '<div class="grid">'+cards+'</div>';
}
load();
setInterval(load, 10000);
</script>
</body>
</html>`
