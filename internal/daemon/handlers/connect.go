package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/marko-stanojevic/kompakt/internal/common"
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
		_ = ws.Close()
	}()

	// Push playbook immediately if one is assigned.
	if e.Service != nil {
		e.Service.PushPlaybookIfAssigned(clientID, false)
	}

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
		if e.Service != nil {
			e.Service.AppendDeploymentLog(d.DeploymentID, d.JobName, d.StepIndex, common.LogLevelInfo,
				fmt.Sprintf("[%s / %s] starting", d.JobName, stepName))
		}
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
		if e.Service != nil {
			e.Service.AppendDeploymentLog(d.DeploymentID, d.JobName, d.StepIndex, common.LogLevelInfo,
				fmt.Sprintf("[%s / %s] completed", d.JobName, stepName))
		}
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
		if e.Service != nil {
			playbookName := e.Service.ResolvePlaybookNameByDeployment(d.DeploymentID)
			e.Service.AppendDeploymentLog(d.DeploymentID, "", 0, common.LogLevelInfo,
				fmt.Sprintf("playbook %q completed successfully", playbookName))
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
		if json.Unmarshal(envelope.Data, &d) != nil {
			return
		}
		playbookName := "playbook"
		if e.Service != nil {
			playbookName = e.Service.ResolvePlaybookNameByDeployment(d.DeploymentID)
		}
		msg := fmt.Sprintf("playbook %q failed", playbookName)
		if d.Error != "" {
			msg = fmt.Sprintf("%s: %s", msg, d.Error)
		}
		if e.Service != nil {
			e.Service.AppendDeploymentLog(d.DeploymentID, d.JobName, d.StepIndex, common.LogLevelError, msg)
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

func (e *Env) updateDeploy(deploymentID string, fn func(*common.DeploymentState)) {
	dep, ok := e.Store.GetDeployment(deploymentID)
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

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	clients := e.Store.ListClients()
	deployments := e.Store.ListDeployments()
	if e.Service != nil {
		clients, deployments = e.Service.StatusSnapshot()
	}
	writeJSON(w, http.StatusOK, StatusResponse{
		Clients:     clients,
		Deployments: deployments,
	})
}

// HandleStatusUI returns a live HTML dashboard.
func (e *Env) HandleStatusUI(w http.ResponseWriter, r *http.Request) {
	renderUI(w, "status.html")
}
