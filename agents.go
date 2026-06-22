// Package e2eframework — agents.go
//
// Helpers for creating, triggering, and polling Memory agents via the CLI and REST API.
package runlog

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// PollUntilSuccess polls agent runs until a run with status=success is found,
// or the deadline is exceeded.
//
// The polling loop runs in the background and only emits log lines (and DB
// events) when the observed status actually changes — not on every tick.
// This keeps logs focused on meaningful transitions rather than repetitive
// "still waiting" noise.
//
// Returns true on success, false on timeout or terminal error.
func PollUntilSuccess(  //nolint:deadcode
	t *testing.T,
	rl *RunLog,
	home, srv, token, projectID, agentID, agentName string,
	timeout, pollInterval time.Duration,
) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)

	// lastStatus tracks the most-recently observed status string so we can
	// detect transitions without logging every poll tick.
	lastStatus := ""

	// lastCompact and lastRunsOut hold the most recent poll output so they
	// are available outside the loop for timeout logging.
	var lastCompact, lastRunsOut string

	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)

		runsOut, _ := RunCLIInDirWithHome(t, "", home,
			"agents", "runs", agentID,
			"--project", projectID,
		)
		compact := CompactRunsOutput(runsOut)
		lastCompact, lastRunsOut = compact, runsOut

		// Extract the current status from the compact output.
		currentStatus := extractStatus(compact, runsOut)

		// Only log when something changed.
		if currentStatus != lastStatus {
			if lastStatus == "" {
				rl.Printf("agent %s: status=%s", agentName, currentStatus)
			} else {
				rl.Event("state_change",
					fmt.Sprintf("agent %s: %s → %s", agentName, lastStatus, currentStatus),
					map[string]any{
						"entity_type": "agent_run",
						"agent_name":  agentName,
						"agent_id":    agentID,
						"from":        lastStatus,
						"to":          currentStatus,
					},
				)
			}
			lastStatus = currentStatus
		}

		// Terminal: success
		if currentStatus == "success" {
			return true
		}

		// Terminal: error or failed
		if currentStatus == "error" || currentStatus == "failed" {
			rl.Printf("agent %s reached terminal status: %s\n%s", agentName, currentStatus, compact)
			rl.CLI("memory agents runs "+agentID, runsOut)
			return false
		}
	}

	rl.Printf("agent %s timed out after %s (last status: %s)\n%s", agentName, timeout, lastStatus, lastCompact)
	rl.CLI("memory agents runs "+agentID, lastRunsOut)
	return false
}

// extractStatus parses the current status from compact or raw runs output.
// Returns an empty string if no status can be determined.
func extractStatus(compact, raw string) string {  //nolint:deadcode
	// Compact output format: "status=<value>"
	for _, part := range strings.Fields(compact) {
		if strings.HasPrefix(part, "status=") {
			return strings.TrimPrefix(part, "status=")
		}
	}
	// Fall back to raw "Status:    <value>" lines.
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Status:") {
			return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "Status:")))
		}
	}
	return ""
}

// DumpAgentRunDetails fetches messages and tool calls for every run of every
// agent and writes one detail file per run into the run-log folder.
// File name: <agentName>-<runID[:8]>.log
//
// agentNames and agentIDs must be parallel slices of the same length.
// Must be called before the project is deleted (the API needs the project alive).
func DumpAgentRunDetails(t *testing.T, rl *RunLog, srv, token, projectID string, agentNames, agentIDs []string) {  //nolint:deadcode
	t.Helper()
	dir := rl.Dir()
	if dir == "" {
		return
	}
	for i, agentID := range agentIDs {
		agentName := agentNames[i]

		runsURL := fmt.Sprintf("%s/api/projects/%s/agents/%s/runs?limit=20", srv, projectID, agentID)
		runsResp := DoJSON(t, "GET", runsURL, token, projectID, nil)
		runsBody := ReadBody(t, runsResp)
		if runsResp.StatusCode != 200 {
			rl.Printf("warn: DumpAgentRunDetails: agent runs API for %s returned %d", agentName, runsResp.StatusCode)
			continue
		}
		var runsAPIResp struct {
			Data []struct {
				ID          string  `json:"id"`
				Status      string  `json:"status"`
				StartedAt   string  `json:"startedAt"`
				CompletedAt string  `json:"completedAt"`
				DurationMs  float64 `json:"durationMs"`
				StepCount   int     `json:"stepCount"`
				Summary     any     `json:"summary"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(runsBody)), &runsAPIResp); err != nil {
			rl.Printf("warn: DumpAgentRunDetails: could not parse runs for %s: %v", agentName, err)
			continue
		}

		for _, run := range runsAPIResp.Data {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("=== AGENT RUN: %s / %s ===\n", agentName, run.ID))
			sb.WriteString(fmt.Sprintf("status:      %s\n", run.Status))
			sb.WriteString(fmt.Sprintf("started_at:  %s\n", run.StartedAt))
			sb.WriteString(fmt.Sprintf("completed:   %s\n", run.CompletedAt))
			sb.WriteString(fmt.Sprintf("duration_ms: %.0f\n", run.DurationMs))
			sb.WriteString(fmt.Sprintf("steps:       %d\n", run.StepCount))
			if run.Summary != nil {
				if b, err := json.MarshalIndent(run.Summary, "", "  "); err == nil {
					sb.WriteString(fmt.Sprintf("summary:\n%s\n", string(b)))
				}
			}

			if run.ID != "" {
				inTok, outTok, costUSD := FetchRunTokenUsage(t, srv, token, projectID, run.ID)
				if inTok > 0 || outTok > 0 {
					sb.WriteString(fmt.Sprintf("token_usage: %s in / %s out  est. $%.6f\n",
						FormatInt(inTok), FormatInt(outTok), costUSD))
				} else {
					sb.WriteString("token_usage: (none)\n")
				}
			}

			// Messages
			msgsURL := fmt.Sprintf("%s/api/projects/%s/agent-runs/%s/messages", srv, projectID, run.ID)
			msgsResp := DoJSON(t, "GET", msgsURL, token, projectID, nil)
			msgsBody := ReadBody(t, msgsResp)
			if msgsResp.StatusCode == 200 {
				sb.WriteString("\n--- messages ---\n")
				sb.WriteString(PrettyJSONOutput(msgsBody))
				sb.WriteString("\n")
			}

			// Write file
			shortID := run.ID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			filename := fmt.Sprintf("%s-%s.log", agentName, shortID)
			filePath := dir + "/" + filename
			_ = writeFile(filePath, sb.String())
			rl.Printf("agent run detail: %s", filePath)
		}
	}
}

// writeFile writes content to path, creating or truncating the file.
func writeFile(path, content string) error {  //nolint:deadcode
	return os.WriteFile(path, []byte(content), 0o644)
}

// TriggerAgent triggers an agent via the `memory agents trigger` CLI command.
// It wraps MustRunCLIInDirWithHome (fatal on non-zero exit) and logs the result
// to rl when rl is non-nil. Returns the raw CLI output string.
func TriggerAgent(t *testing.T, rl *RunLog, home, projectID, agentID string) string {  //nolint:deadcode
	t.Helper()
	out := MustRunCLIInDirWithHome(t, "", home,
		"agents", "trigger", agentID,
		"--project", projectID,
	)
	if rl != nil {
		rl.CLI("memory agents trigger "+agentID, out)
	}
	return out
}

// AgentQuestion represents a single agent question from the CLI JSON output.
type AgentQuestion struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Status   string `json:"status"`
}

// ListPendingQuestions lists pending agent questions for a project via the CLI.
// Returns the parsed questions and raw CLI output. On CLI error returns nil
// questions and the error. Does NOT call t.Fatal — safe for poll loops.
func ListPendingQuestions(t *testing.T, rl *RunLog, home, projectID string) ([]AgentQuestion, string, error) {  //nolint:deadcode
	t.Helper()
	out, err := RunCLIInDirWithHome(t, "", home,
		"agents", "questions", "list-project",
		"--status", "pending",
		"--project", projectID,
		"--output", "json",
	)
	if err != nil {
		return nil, out, err
	}
	if rl != nil {
		rl.CLI("memory agents questions list-project --status pending", out)
	}

	var resp struct {
		Data []AgentQuestion `json:"data"`
	}
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); jsonErr != nil {
		return nil, out, fmt.Errorf("parse agent-questions JSON: %w", jsonErr)
	}
	return resp.Data, out, nil
}

// ListAllQuestions lists all agent questions for a project (no status filter).
// Returns parsed questions and raw CLI output. Does NOT call t.Fatal.
func ListAllQuestions(t *testing.T, rl *RunLog, home, projectID string) ([]AgentQuestion, string, error) {  //nolint:deadcode
	t.Helper()
	out, err := RunCLIInDirWithHome(t, "", home,
		"agents", "questions", "list-project",
		"--project", projectID,
		"--output", "json",
	)
	if err != nil {
		return nil, out, err
	}
	if rl != nil {
		rl.CLI("memory agents questions list-project", out)
	}

	var resp struct {
		Data []AgentQuestion `json:"data"`
	}
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); jsonErr != nil {
		return nil, out, fmt.Errorf("parse agent-questions JSON: %w", jsonErr)
	}
	return resp.Data, out, nil
}

// RespondToQuestion responds to a pending agent question via the CLI.
// Returns raw CLI output. On CLI error returns the output and the error.
// Does NOT call t.Fatal — callers decide how to handle errors.
func RespondToQuestion(t *testing.T, rl *RunLog, home, projectID, questionID, response string) (string, error) {  //nolint:deadcode
	t.Helper()
	out, err := RunCLIInDirWithHome(t, "", home,
		"agents", "questions", "respond",
		questionID, response,
		"--project", projectID,
	)
	if rl != nil {
		rl.CLI(fmt.Sprintf("memory agents questions respond %s %q", questionID, response), out)
	}
	return out, err
}
