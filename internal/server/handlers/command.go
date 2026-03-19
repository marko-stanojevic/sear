package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/marko-stanojevic/kompakt/internal/common"
)

// HandleCommand handles the command sub-resource of an agent.
//
//	POST /api/v1/agents/{id}/command              — submit a command
//	GET  /api/v1/agents/{id}/command/{cmd_id}     — poll output
//
// agentID is the agent extracted by the parent router.
// subPath is the portion of the URL after "/command" (empty or "/{cmd_id}").
func (e *Handler) HandleCommand(w http.ResponseWriter, r *http.Request, agentID, subPath string) {
	cmdID := strings.TrimPrefix(subPath, "/")

	switch r.Method {
	case http.MethodPost:
		if cmdID != "" {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body struct {
			Command string `json:"command"`
			Shell   string `json:"shell,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		body.Command = strings.TrimSpace(body.Command)
		if body.Command == "" {
			writeError(w, http.StatusBadRequest, "command is required")
			return
		}
		agent, ok := e.Store.GetAgent(agentID)
		if !ok {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		if len(agent.Shells) > 0 && body.Shell != "" {
			available := false
			for _, s := range agent.Shells {
				if s == body.Shell {
					available = true
					break
				}
			}
			if !available {
				writeError(w, http.StatusBadRequest, "shell "+body.Shell+" is not available on this agent")
				return
			}
		}
		if !e.Hub.IsConnected(agentID) {
			writeError(w, http.StatusConflict, "agent is not connected")
			return
		}

		newCmdID := common.NewID()
		e.Commands.Create(newCmdID, agentID, body.Command)

		sent := e.Hub.Send(agentID, common.WSMessage{
			Type:      common.WSMsgCommand,
			Timestamp: time.Now(),
			Data: common.WSCommandData{
				CmdID:   newCmdID,
				Command: body.Command,
				Shell:   body.Shell,
			},
		})
		if !sent {
			writeError(w, http.StatusConflict, "failed to deliver command to agent")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"cmd_id": newCmdID})

	case http.MethodGet:
		if cmdID == "" {
			writeError(w, http.StatusBadRequest, "cmd_id required in path")
			return
		}
		sess, ok := e.Commands.Get(cmdID)
		if !ok || sess.AgentID != agentID {
			writeError(w, http.StatusNotFound, "command not found")
			return
		}
		output, done, exitCode, errMsg := sess.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"cmd_id":    cmdID,
			"output":    output,
			"done":      done,
			"exit_code": exitCode,
			"error":     errMsg,
		})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
