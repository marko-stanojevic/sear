// Package client implements the sear deployment client.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/marko-stanojevic/sear/internal/client/executor"
	"github.com/marko-stanojevic/sear/internal/client/registration"
	"github.com/marko-stanojevic/sear/internal/common"
)

// localState is persisted to disk so the client can resume after a reboot.
type localState struct {
	ClientID     string `json:"client_id"`
	Token        string `json:"token"`
	DeploymentID string `json:"deployment_id,omitempty"`
}

// Client is the sear deployment client.
type Client struct {
	cfg       *common.ClientConfig
	state     localState
	logBuf    []common.LogEntry
	logMu     sync.Mutex
	httpClient *http.Client
}

// New creates a new Client using the given configuration.
func New(cfg *common.ClientConfig) *Client {
	if cfg.PollIntervalSeconds == 0 {
		cfg.PollIntervalSeconds = 10
	}
	if cfg.LogBatchSize == 0 {
		cfg.LogBatchSize = 100
	}
	if cfg.StateFile == "" {
		cfg.StateFile = "/var/lib/sear/state.json"
	}
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Run starts the client loop.  It registers, then repeatedly polls /connect.
func (c *Client) Run(ctx context.Context) error {
	if err := c.loadState(); err != nil {
		log.Printf("warn: could not load local state: %v", err)
	}

	// Register / re-register if we have no token.
	if c.state.Token == "" {
		if err := c.register(ctx); err != nil {
			return fmt.Errorf("registration: %w", err)
		}
	}

	// Start log flusher.
	go c.flushLogsLoop(ctx)

	// Poll /connect.
	tick := time.NewTicker(time.Duration(c.cfg.PollIntervalSeconds) * time.Second)
	defer tick.Stop()
	// Poll immediately on start.
	if err := c.poll(ctx); err != nil {
		log.Printf("poll: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			if err := c.poll(ctx); err != nil {
				log.Printf("poll: %v", err)
			}
		}
	}
}

// register sends a registration request to the server.
func (c *Client) register(ctx context.Context) error {
	pf := registration.Collect(c.cfg.Platform)
	req := common.RegistrationRequest{
		Platform:           common.PlatformType(pf.Platform),
		PlatformID:         pf.ID,
		Hostname:           pf.Hostname,
		RegistrationSecret: c.cfg.RegistrationSecret,
		Metadata:           pf.Metadata,
	}
	var resp common.RegistrationResponse
	if err := c.post(ctx, "/api/v1/register", req, &resp, ""); err != nil {
		return err
	}
	c.state.ClientID = resp.ClientID
	c.state.Token = resp.Token
	log.Printf("registered as %s", c.state.ClientID)
	return c.saveState()
}

// poll calls /api/v1/connect and acts on the response.
func (c *Client) poll(ctx context.Context) error {
	var resp common.ConnectResponse
	if err := c.get(ctx, "/api/v1/connect", &resp, c.state.Token); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	switch resp.Action {
	case "wait":
		// Nothing to do.
	case "deploy":
		if resp.Playbook == nil {
			return fmt.Errorf("deploy action with no playbook")
		}
		c.state.DeploymentID = resp.DeploymentID
		if err := c.saveState(); err != nil {
			log.Printf("warn: save state: %v", err)
		}
		c.runPlaybook(ctx, &resp)
	case "reboot":
		log.Println("server requested reboot")
		return c.reboot()
	default:
		log.Printf("unknown action: %q", resp.Action)
	}
	return nil
}

// runPlaybook executes the playbook returned by the server.
func (c *Client) runPlaybook(ctx context.Context, resp *common.ConnectResponse) {
	pb := resp.Playbook
	c.emit(common.LogLevelInfo, resp.DeploymentID, "", 0, fmt.Sprintf("starting playbook %q", pb.Name))

	// Build ordered job list.
	jobs := orderedJobs(pb)

	// Find resume position.
	resumeJob := resp.ResumeJobName
	resumeStep := resp.ResumeStepIndex
	started := resumeJob == ""

	for _, jobEntry := range jobs {
		jobName := jobEntry.name
		job := jobEntry.job
		if !started {
			if jobName != resumeJob {
				continue
			}
			started = true
		}

		c.emit(common.LogLevelInfo, resp.DeploymentID, jobName, 0, fmt.Sprintf("starting job %q", jobName))
		for i, step := range job.Steps {
			startIdx := 0
			if jobName == resumeJob {
				startIdx = resumeStep
			}
			if i < startIdx {
				continue
			}
			stepName := step.Name
			if stepName == "" {
				stepName = fmt.Sprintf("step-%d", i)
			}
			c.emit(common.LogLevelInfo, resp.DeploymentID, jobName, i, fmt.Sprintf("running step %q", stepName))

			// Update server state.
			_ = c.updateState(ctx, resp.DeploymentID, common.DeploymentStatusRunning, jobName, i, "")

			logger := func(level common.LogLevel, msg string) {
				c.emit(level, resp.DeploymentID, jobName, i, msg)
			}
			result := executor.RunStep(ctx, step, resp.Secrets, resp.ArtifactsBaseURL, c.state.Token, logger)

			if result.NeedsReboot {
				c.emit(common.LogLevelInfo, resp.DeploymentID, jobName, i, "step requested reboot; persisting state")
				nextStep := i + 1
				if nextStep >= len(job.Steps) {
					nextStep = 0
					// Advance to next job (simplified: mark this job as done).
				}
				_ = c.updateState(ctx, resp.DeploymentID, common.DeploymentStatusRebooting, jobName, nextStep, "")
				_ = c.flushLogs(ctx)
				if err := c.reboot(); err != nil {
					log.Printf("reboot error: %v", err)
				}
				return
			}
			if result.Err != nil {
				if !step.ContinueOnError {
					c.emit(common.LogLevelError, resp.DeploymentID, jobName, i, "step failed: "+result.Err.Error())
					_ = c.updateState(ctx, resp.DeploymentID, common.DeploymentStatusFailed, jobName, i, result.Err.Error())
					_ = c.flushLogs(ctx)
					return
				}
				c.emit(common.LogLevelWarn, resp.DeploymentID, jobName, i, "step failed (continue-on-error): "+result.Err.Error())
			}
		}
	}

	c.emit(common.LogLevelInfo, resp.DeploymentID, "", 0, "playbook completed successfully")
	_ = c.updateState(ctx, resp.DeploymentID, common.DeploymentStatusDone, "", 0, "")
	_ = c.flushLogs(ctx)
}

// updateState sends a state update to the server.
func (c *Client) updateState(ctx context.Context, deploymentID string, status common.DeploymentStatus, job string, step int, errDetail string) error {
	req := common.StateUpdateRequest{
		DeploymentID:     deploymentID,
		Status:           status,
		CurrentJobName:   job,
		CurrentStepIndex: step,
		ErrorDetail:      errDetail,
	}
	var resp map[string]string
	return c.post(ctx, "/api/v1/state", req, &resp, c.state.Token)
}

// ---- Logging ---------------------------------------------------------------

func (c *Client) emit(level common.LogLevel, depID, jobName string, stepIdx int, msg string) {
	entry := common.LogEntry{
		DeploymentID: depID,
		JobName:      jobName,
		StepIndex:    stepIdx,
		Level:        level,
		Message:      msg,
		Timestamp:    time.Now(),
	}
	c.logMu.Lock()
	c.logBuf = append(c.logBuf, entry)
	c.logMu.Unlock()
	log.Printf("[%s] %s", level, msg)
}

func (c *Client) flushLogsLoop(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			_ = c.flushLogs(ctx)
		}
	}
}

func (c *Client) flushLogs(ctx context.Context) error {
	c.logMu.Lock()
	if len(c.logBuf) == 0 {
		c.logMu.Unlock()
		return nil
	}
	batch := c.logBuf
	c.logBuf = nil
	c.logMu.Unlock()

	payload := common.LogBatch{Entries: batch}
	var resp map[string]string
	if err := c.post(ctx, "/api/v1/logs", payload, &resp, c.state.Token); err != nil {
		// Put logs back.
		c.logMu.Lock()
		c.logBuf = append(batch, c.logBuf...)
		c.logMu.Unlock()
		return err
	}
	return nil
}

// ---- Local state -----------------------------------------------------------

func (c *Client) loadState() error {
	data, err := os.ReadFile(c.cfg.StateFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &c.state)
}

func (c *Client) saveState() error {
	_ = os.MkdirAll(filepath.Dir(c.cfg.StateFile), 0o700)
	data, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.cfg.StateFile, data, 0o600)
}

// ---- HTTP helpers ----------------------------------------------------------

func (c *Client) post(ctx context.Context, path string, body, out any, token string) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.ServerURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, path)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, out any, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.ServerURL+path, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, path)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// ---- Reboot ----------------------------------------------------------------

func (c *Client) reboot() error {
	return rebootOS()
}

// ---- Job ordering ----------------------------------------------------------

type namedJob struct {
	name string
	job  common.Job
}

// orderedJobs returns jobs in a deterministic order.
// Since Go maps are unordered, jobs are sorted by key name.
func orderedJobs(pb *common.Playbook) []namedJob {
	// Build alphabetically sorted list for deterministic execution.
	// In practice, operators should use a single-job playbook or rely on
	// job dependency ordering; a future enhancement could add "needs:" support.
	keys := make([]string, 0, len(pb.Jobs))
	for k := range pb.Jobs {
		keys = append(keys, k)
	}
	// Sort for stability.
	sortStrings(keys)
	out := make([]namedJob, 0, len(keys))
	for _, k := range keys {
		out = append(out, namedJob{name: k, job: pb.Jobs[k]})
	}
	return out
}

// sortStrings performs an in-place insertion sort (avoids importing "sort" for
// a small slice).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
