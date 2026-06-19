// Package e2eframework — analyzer.go
//
// Analyzer wraps a Google ADK LLM agent that reads experiment data from the
// RunDB and emits structured improvement suggestions stored in the
// experiment_suggestions table.
//
// Usage:
//
//	a, err := framework.NewAnalyzer(db)
//	suggestions, err := a.Run(ctx, "my-experiment")
package runlog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	adkmodel "google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"
)

// analyzerSystemPrompt instructs the agent to analyse experiment runs.
// Prescriptive style: exact steps, run_cli verification required.
const analyzerSystemPrompt = `You are an expert software quality analyst. You analyse end-to-end test experiment runs for the Memory platform and produce specific, actionable improvement suggestions.

The Memory platform is a system for building AI agents with persistent memory, knowledge graphs, and tool access. The "memory" CLI is the primary interface. CLI commands change between versions; never assume you know the current syntax.

## Tools

1. get_experiment_summary — High-level stats (run count, pass/fail rates, tags).
2. list_runs — All runs with status, timing, token usage.
3. get_run_events — Structured event log for a run (sections, CLI calls, errors, trace spans). Capped at 100 rows.
4. get_run_log — Raw flat log file for a run (up to 50 KB).
5. run_cli — Run a read-only memory CLI command (--help, list, get, version, status). Use this to verify which commands exist. Examples: run_cli(args: ["--help"]), run_cli(args: ["agents", "mcp-servers", "--help"]).
6. memory_ask — Ask the Memory platform assistant a question. WARNING: sometimes suggests non-existent commands. Always verify CLI syntax with run_cli.
7. create_suggestion — Store one improvement suggestion.

## Instructions

### Step 1: Understand the experiment

Call get_experiment_summary, then list_runs. Identify patterns: failures, anomalous token usage, slow runs, flakiness (same test passes sometimes, fails others).

### Step 2: Investigate failures and anomalies

For each failed or anomalous run:
1. Call get_run_events to inspect structured events.
2. If you need more detail, call get_run_log for the raw log.
3. For CLI failures ("unknown command", wrong syntax): use run_cli to find the correct command. Commands move between versions — e.g., "memory mcp-servers" moved to "memory agents mcp-servers". Check plausible parents with --help.
4. Use memory_ask only after run_cli, for additional context. Always cross-check CLI syntax with run_cli.

### Step 3: Create suggestions

For each finding, call create_suggestion with:
- title: ≤ 80 chars, clear problem statement.
- body: ≤ 500 chars. (a) What happened (run ID, event seq, error). (b) Root cause. (c) Verified fix (only CLI commands confirmed with run_cli).
- category: reliability | performance | cost | flakiness | configuration | observability
- priority: high (caused failure), medium (degrades quality), low (observation).
- run_ids: list of affected run IDs.

### Step 4: Summary

Brief summary: patterns found, suggestions created, root causes.

## Rules

- Verify every CLI command with run_cli before including it in a suggestion.
- Do not trust memory_ask for CLI syntax — cross-check with run_cli.
- Do not create vague suggestions. Be specific about the fix.
- Reference actual run IDs, test names, and event details in every suggestion.`

// AnalyzerEventKind identifies the type of conversation event.
type AnalyzerEventKind string

const (
	AEThought      AnalyzerEventKind = "thought"
	AEText         AnalyzerEventKind = "text"
	AEToolCall     AnalyzerEventKind = "tool_call"
	AEToolResult   AnalyzerEventKind = "tool_result"
	AETokenUsage   AnalyzerEventKind = "token_usage"
	AEError        AnalyzerEventKind = "error"
	AETurnComplete AnalyzerEventKind = "turn_complete"
	AESystemPrompt AnalyzerEventKind = "system_prompt"
	AEUserMessage  AnalyzerEventKind = "user_message"
)

// AnalyzerEvent represents one step in the analyzer agent's conversation.
// Callers receive these via the OnEvent callback to trace agent behavior.
type AnalyzerEvent struct {
	Kind    AnalyzerEventKind
	Author  string // agent name that produced this event
	Content string // text/thought content, or formatted tool call/result

	// Populated for tool_call events.
	ToolName string
	ToolArgs map[string]any

	// Populated for tool_result events.
	ToolResponse map[string]any

	// Populated for token_usage events.
	PromptTokens  int32
	OutputTokens  int32
	ThoughtTokens int32
	TotalTokens   int32

	// Populated for error events.
	ErrorCode    string
	ErrorMessage string
}

// Analyzer runs an ADK LLM agent that reads experiment data and produces
// structured improvement suggestions stored in the RunDB.
type Analyzer struct {
	db                *RunDB
	agent             agent.Agent // multi-run agent (experiment / test name analysis)
	llm               adkmodel.LLM
	currentExperiment string

	// filterByTestName, when true, makes the list_runs tool filter by test name
	// instead of experiment name.  Set by RunByTestName.
	filterByTestName bool
	currentTestName  string

	// OnEvent, when set, is called for each step of the agent's conversation.
	// Callers can use this to trace/log the agent's thought process, tool
	// calls, responses, and token usage in real time.
	OnEvent func(AnalyzerEvent)

	// activeTraceID and traceSeq track the current trace being written to DB.
	// Set by RunByRunID; zero means no trace is being persisted.
	activeTraceID int64
	traceSeq      int
}

// NewAnalyzer creates an Analyzer.  It reads GOOGLE_AI_API_KEY (required) and
// ANALYZER_MODEL (optional, defaults to "gemini-2.5-flash") from the environment.
func NewAnalyzer(db *RunDB) (*Analyzer, error) {
	apiKey := os.Getenv("GOOGLE_AI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("analyzer: GOOGLE_AI_API_KEY is not set")
	}

	modelName := os.Getenv("ANALYZER_MODEL")
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}

	ctx := context.Background()
	llm, err := gemini.NewModel(ctx, modelName, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("analyzer: create gemini model: %w", err)
	}

	a := &Analyzer{db: db, llm: llm}

	tools, err := a.buildTools()
	if err != nil {
		return nil, fmt.Errorf("analyzer: build tools: %w", err)
	}

	ag, err := llmagent.New(llmagent.Config{
		Name:        "experiment_analyzer",
		Description: "Analyses e2e test experiment runs and generates improvement suggestions.",
		Instruction: analyzerSystemPrompt,
		Model:       llm,
		Tools:       tools,
	})
	if err != nil {
		return nil, fmt.Errorf("analyzer: create agent: %w", err)
	}

	a.agent = ag
	return a, nil
}

// emit sends an AnalyzerEvent to the OnEvent callback if set, and persists
// it to the DB when a trace is active (activeTraceID > 0).
func (a *Analyzer) emit(ev AnalyzerEvent) {
	if a.activeTraceID > 0 {
		a.traceSeq++
		// Best-effort: don't fail the analysis if trace write fails.
		_ = a.db.InsertAnalyzerTraceEvent(a.activeTraceID, a.traceSeq, ev)
	}
	if a.OnEvent != nil {
		a.OnEvent(ev)
	}
}

// processRunnerEvents drains all events from an ADK runner, extracting the
// full agent conversation (thoughts, text, tool calls, tool results, token
// usage, errors) and emitting them via OnEvent.
// Returns the first fatal error encountered (e.g. 429 spending-cap), or nil.
func (a *Analyzer) processRunnerEvents(events func(func(*session.Event, error) bool)) error {
	var firstErr error
	for event, eventErr := range events {
		if eventErr != nil {
			msg := eventErr.Error()
			a.emit(AnalyzerEvent{
				Kind:         AEError,
				ErrorMessage: msg,
				Content:      msg,
			})
			if firstErr == nil {
				firstErr = eventErr
			}
			continue
		}
		if event == nil {
			continue
		}

		// Skip partial/streaming events — we only want completed turns.
		if event.Partial {
			continue
		}

		author := event.Author

		// Extract error info from the LLMResponse.
		if event.ErrorCode != "" || event.ErrorMessage != "" {
			a.emit(AnalyzerEvent{
				Kind:         AEError,
				Author:       author,
				ErrorCode:    event.ErrorCode,
				ErrorMessage: event.ErrorMessage,
				Content:      fmt.Sprintf("error [%s]: %s", event.ErrorCode, event.ErrorMessage),
			})
		}

		// Process content parts.
		if event.Content != nil {
			for _, part := range event.Content.Parts {
				if part == nil {
					continue
				}

				// Thought
				if part.Thought && part.Text != "" {
					a.emit(AnalyzerEvent{
						Kind:    AEThought,
						Author:  author,
						Content: part.Text,
					})
					continue
				}

				// Text output
				if part.Text != "" {
					a.emit(AnalyzerEvent{
						Kind:    AEText,
						Author:  author,
						Content: part.Text,
					})
					continue
				}

				// Function call
				if part.FunctionCall != nil {
					fc := part.FunctionCall
					// Format args for display.
					argsJSON, _ := json.Marshal(fc.Args)
					a.emit(AnalyzerEvent{
						Kind:     AEToolCall,
						Author:   author,
						ToolName: fc.Name,
						ToolArgs: fc.Args,
						Content:  fmt.Sprintf("%s(%s)", fc.Name, string(argsJSON)),
					})
					continue
				}

				// Function response
				if part.FunctionResponse != nil {
					fr := part.FunctionResponse
					respJSON, _ := json.Marshal(fr.Response)
					content := string(respJSON)
					// Truncate very long responses for display.
					if len(content) > 500 {
						content = content[:497] + "..."
					}
					a.emit(AnalyzerEvent{
						Kind:         AEToolResult,
						Author:       author,
						ToolName:     fr.Name,
						ToolResponse: fr.Response,
						Content:      fmt.Sprintf("%s -> %s", fr.Name, content),
					})
					continue
				}
			}
		}

		// Token usage.
		if event.UsageMetadata != nil {
			um := event.UsageMetadata
			if um.TotalTokenCount > 0 {
				a.emit(AnalyzerEvent{
					Kind:          AETokenUsage,
					Author:        author,
					PromptTokens:  um.PromptTokenCount,
					OutputTokens:  um.CandidatesTokenCount,
					ThoughtTokens: um.ThoughtsTokenCount,
					TotalTokens:   um.TotalTokenCount,
					Content: fmt.Sprintf("tokens: prompt=%d output=%d thought=%d total=%d",
						um.PromptTokenCount, um.CandidatesTokenCount, um.ThoughtsTokenCount, um.TotalTokenCount),
				})
			}
		}

		// Turn complete marker.
		if event.TurnComplete {
			a.emit(AnalyzerEvent{
				Kind:   AETurnComplete,
				Author: author,
			})
		}
	}
	return firstErr
}

// Run deletes any existing suggestions for experiment, runs the ADK agent to
// analyse the experiment data, and returns the newly created suggestions.
func (a *Analyzer) Run(ctx context.Context, experiment string) ([]SuggestionRow, error) {
	// Store the current experiment so create_suggestion can reference it.
	a.currentExperiment = experiment

	// Clear previous suggestions so the result is always fresh.
	// Traces are kept for history — only suggestions are replaced.
	if err := a.db.DeleteSuggestions(experiment); err != nil {
		return nil, fmt.Errorf("analyzer: delete existing suggestions: %w", err)
	}

	// Start a new trace for this analysis run.
	traceID, err := a.db.InsertAnalyzerTrace(experiment, nil)
	if err != nil {
		return nil, fmt.Errorf("analyzer: create trace: %w", err)
	}
	a.activeTraceID = traceID
	a.traceSeq = 0
	defer func() {
		_ = a.db.FinishAnalyzerTrace(traceID)
		a.activeTraceID = 0
	}()

	sessionService := session.InMemoryService()
	createResp, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName:   "analyzer",
		UserID:    "system",
		SessionID: "analyzer-" + experiment,
	})
	if err != nil {
		return nil, fmt.Errorf("analyzer: create session: %w", err)
	}

	r, err := runner.New(runner.Config{
		Agent:          a.agent,
		SessionService: sessionService,
		AppName:        "analyzer",
	})
	if err != nil {
		return nil, fmt.Errorf("analyzer: create runner: %w", err)
	}

	userMsgText := fmt.Sprintf("Please analyse the experiment %q and create improvement suggestions.", experiment)
	userMsg := genai.NewContentFromText(userMsgText, genai.RoleUser)

	// Emit the prompts so trace viewers can see what was sent to the LLM.
	a.emit(AnalyzerEvent{
		Kind:    AESystemPrompt,
		Author:  "experiment_analyzer",
		Content: analyzerSystemPrompt,
	})
	a.emit(AnalyzerEvent{
		Kind:    AEUserMessage,
		Author:  "user",
		Content: userMsgText,
	})

	// Process all events, emitting conversation trace via OnEvent.
	if err := a.processRunnerEvents(r.Run(ctx, "system", createResp.Session.ID(), userMsg, agent.RunConfig{})); err != nil {
		return nil, fmt.Errorf("analyzer: LLM run failed: %w", err)
	}

	return a.db.ListSuggestions(experiment)
}

// RunByTestName analyses all runs for a specific test name (regardless of
// experiment tag) and stores suggestions under the synthetic key
// "test:<testName>" in the experiment_suggestions table.
func (a *Analyzer) RunByTestName(ctx context.Context, testName string) ([]SuggestionRow, error) {
	// Synthetic key used as the suggestions table primary key.
	suggKey := "test:" + testName

	a.currentExperiment = suggKey
	a.filterByTestName = true
	a.currentTestName = testName
	defer func() {
		a.filterByTestName = false
		a.currentTestName = ""
	}()

	// Clear previous suggestions so the result is always fresh.
	// Traces are kept for history — only suggestions are replaced.
	if err := a.db.DeleteSuggestions(suggKey); err != nil {
		return nil, fmt.Errorf("analyzer: delete existing suggestions: %w", err)
	}

	// Start a new trace for this analysis run.
	traceID, err := a.db.InsertAnalyzerTrace(suggKey, nil)
	if err != nil {
		return nil, fmt.Errorf("analyzer: create trace: %w", err)
	}
	a.activeTraceID = traceID
	a.traceSeq = 0
	defer func() {
		_ = a.db.FinishAnalyzerTrace(traceID)
		a.activeTraceID = 0
	}()

	sessionService := session.InMemoryService()
	createSessResp, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName:   "analyzer",
		UserID:    "system",
		SessionID: "analyzer-test-" + strings.NewReplacer("/", "-", " ", "-").Replace(testName),
	})
	if err != nil {
		return nil, fmt.Errorf("analyzer: create session: %w", err)
	}

	r, err := runner.New(runner.Config{
		Agent:          a.agent,
		SessionService: sessionService,
		AppName:        "analyzer",
	})
	if err != nil {
		return nil, fmt.Errorf("analyzer: create runner: %w", err)
	}

	userMsgText := fmt.Sprintf("Please analyse all runs for the test %q and create improvement suggestions.", testName)
	userMsg := genai.NewContentFromText(userMsgText, genai.RoleUser)

	// Emit the prompts so trace viewers can see what was sent to the LLM.
	a.emit(AnalyzerEvent{
		Kind:    AESystemPrompt,
		Author:  "experiment_analyzer",
		Content: analyzerSystemPrompt,
	})
	a.emit(AnalyzerEvent{
		Kind:    AEUserMessage,
		Author:  "user",
		Content: userMsgText,
	})

	// Process all events, emitting conversation trace via OnEvent.
	if err := a.processRunnerEvents(r.Run(ctx, "system", createSessResp.Session.ID(), userMsg, agent.RunConfig{})); err != nil {
		return nil, fmt.Errorf("analyzer: LLM run failed: %w", err)
	}

	return a.db.ListSuggestions(suggKey)
}

// singleRunSystemPrompt instructs the agent for in-depth single-run analysis.
// All run data is injected into the user message, so the agent only needs
// the create_suggestion, memory_ask, and run_cli tools.
//
// The prompt is deliberately prescriptive: exact steps, no ambiguity.
// The LLM must use run_cli to verify which CLI commands exist before creating
// any suggestion, and must cross-check memory_ask answers with run_cli.
const singleRunSystemPrompt = `You are an expert software quality analyst. You analyse a single end-to-end test run for the Memory platform and produce specific, actionable improvement suggestions.

The Memory platform is a system for building AI agents with persistent memory, knowledge graphs, and tool access. The "memory" CLI is the primary interface. These e2e tests exercise the platform exactly as a real user would — through the CLI and API. CLI commands change between versions; never assume you know the current syntax.

The user message contains everything for this run: platform context (what the test is supposed to do) and a full JSON dump (metadata, events, children with details).

## Tools

1. run_cli — Run a read-only memory CLI command (--help, list, get, version, status only). Use this to discover which commands exist and verify syntax. Examples: run_cli(args: ["--help"]), run_cli(args: ["agents", "--help"]), run_cli(args: ["agents", "mcp-servers", "configure", "--help"]).
2. memory_ask — Ask the Memory platform assistant a question. WARNING: it sometimes suggests non-existent commands. Always verify with run_cli.
3. create_suggestion — Store one improvement suggestion.

## Instructions

### Step 1: Read the run data

Read the JSON. Note every cli event with non-zero exit_code, every error, the overall passed field, and any slow steps or anomalies.

### Step 2: For each problem, investigate with run_cli first

For CLI failures ("unknown command", wrong syntax):
1. run_cli(args: ["--help"]) to see top-level commands.
2. Commands often move between versions. Check plausible parents: run_cli(args: ["agents", "--help"]), run_cli(args: ["skills", "--help"]), etc. For example, "memory mcp-servers" moved to "memory agents mcp-servers".
3. When found, verify the exact subcommand: run_cli(args: ["agents", "mcp-servers", "configure", "--help"]).

Use memory_ask only after run_cli, for additional context. Always cross-check any CLI command memory_ask suggests with run_cli — do not trust it blindly.

### Step 3: Create suggestions

For each problem, call create_suggestion with:
- title: ≤ 80 chars, clear problem statement.
- body: ≤ 500 chars. (a) What happened (event seq + error). (b) Root cause (what you found via run_cli/memory_ask). (c) Verified fix (only include CLI commands you confirmed with run_cli).
- category: reliability | performance | cost | flakiness | configuration | observability
- priority: high (caused failure), medium (degrades quality), low (observation).
- run_ids: include the current run ID.

### Step 4: Summary

Brief summary: problems found, suggestions created, root cause.

## Rules

- Verify every CLI command with run_cli before including it in a suggestion.
- Do not trust memory_ask for CLI syntax — cross-check with run_cli.
- Do not create vague suggestions. Be specific about the fix.
- Do not skip investigation. Every suggestion must be backed by run_cli evidence.`

// RunByRunID analyses a single run identified by its database ID and stores
// suggestions under the synthetic key "run:<runID>" in the experiment_suggestions
// table.  Unlike experiment/test analysis, this method injects all run data
// directly into the prompt so the LLM doesn't need read tools — only
// create_suggestion.
func (a *Analyzer) RunByRunID(ctx context.Context, runID int64) ([]SuggestionRow, error) {
	suggKey := fmt.Sprintf("run:%d", runID)
	a.currentExperiment = suggKey

	// Clear previous suggestions so the result is always fresh.
	// Traces are kept for history — only suggestions are replaced.
	if err := a.db.DeleteSuggestions(suggKey); err != nil {
		return nil, fmt.Errorf("analyzer: delete existing suggestions: %w", err)
	}

	// Start a new trace for this analysis run.
	rid := runID
	traceID, err := a.db.InsertAnalyzerTrace(suggKey, &rid)
	if err != nil {
		return nil, fmt.Errorf("analyzer: create trace: %w", err)
	}
	a.activeTraceID = traceID
	a.traceSeq = 0
	defer func() {
		_ = a.db.FinishAnalyzerTrace(traceID)
		a.activeTraceID = 0
	}()

	// ── Build full run context from the database ──────────────────────────
	runCtx, err := a.buildRunContext(runID)
	if err != nil {
		return nil, fmt.Errorf("analyzer: build run context: %w", err)
	}

	// ── Ask the Memory platform about the test for domain context ────────
	// (Pre-call: result is injected into the user message as context.
	// The agent also has the memory_ask tool for follow-up questions.)
	testName := a.getRunTestName(runID)
	var platformContext string
	if testName != "" {
		question := fmt.Sprintf(
			"This is an e2e test named %q. What Memory platform features and CLI commands is it likely testing? What would success and failure look like?",
			testName,
		)
		if resp := memoryAsk(ctx, question); resp != "" {
			platformContext = resp
		}
	}

	// ── Create a lightweight agent with create_suggestion + memory_ask + run_cli ──
	createSuggTool, err := functiontool.New(
		functiontool.Config{
			Name:        "create_suggestion",
			Description: "Store one improvement suggestion for the current run.",
		},
		a.toolCreateSuggestion,
	)
	if err != nil {
		return nil, fmt.Errorf("analyzer: build create_suggestion tool: %w", err)
	}

	memAskTool, err := functiontool.New(
		functiontool.Config{
			Name:        "memory_ask",
			Description: "Ask the Memory platform assistant a question. Use this to learn about Memory CLI commands, platform features, or how specific tests work. Returns the assistant's text response.",
		},
		a.toolMemoryAsk,
	)
	if err != nil {
		return nil, fmt.Errorf("analyzer: build memory_ask tool: %w", err)
	}

	runCLITool, err := functiontool.New(
		functiontool.Config{
			Name:        "run_cli",
			Description: "Run a read-only memory CLI command and return its output. Use this to verify which commands and subcommands actually exist. Only --help, list, get, version, and status commands are allowed.",
		},
		a.toolRunCLI,
	)
	if err != nil {
		return nil, fmt.Errorf("analyzer: build run_cli tool: %w", err)
	}

	singleRunAgent, err := llmagent.New(llmagent.Config{
		Name:        "single_run_analyzer",
		Description: "Analyses a single e2e test run in depth and generates improvement suggestions.",
		Instruction: singleRunSystemPrompt,
		Model:       a.llm,
		Tools:       []tool.Tool{createSuggTool, memAskTool, runCLITool},
	})
	if err != nil {
		return nil, fmt.Errorf("analyzer: create single-run agent: %w", err)
	}

	sessionService := session.InMemoryService()
	createSessResp, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName:   "analyzer",
		UserID:    "system",
		SessionID: fmt.Sprintf("analyzer-run-%d", runID),
	})
	if err != nil {
		return nil, fmt.Errorf("analyzer: create session: %w", err)
	}

	r, err := runner.New(runner.Config{
		Agent:          singleRunAgent,
		SessionService: sessionService,
		AppName:        "analyzer",
	})
	if err != nil {
		return nil, fmt.Errorf("analyzer: create runner: %w", err)
	}

	// ── Build user message with optional platform context ────────────────
	var msgBuilder strings.Builder
	msgBuilder.WriteString("Analyse the following test run and create improvement suggestions.\n\n")
	if platformContext != "" {
		msgBuilder.WriteString("## Platform context (from Memory assistant)\n\n")
		msgBuilder.WriteString(platformContext)
		msgBuilder.WriteString("\n\n")
	}
	msgBuilder.WriteString("## Run data\n\n")
	msgBuilder.WriteString(runCtx)

	userMsg := genai.NewContentFromText(msgBuilder.String(), genai.RoleUser)

	// Emit the prompts so trace viewers can see what was sent to the LLM.
	a.emit(AnalyzerEvent{
		Kind:    AESystemPrompt,
		Author:  "single_run_analyzer",
		Content: singleRunSystemPrompt,
	})
	a.emit(AnalyzerEvent{
		Kind:    AEUserMessage,
		Author:  "user",
		Content: msgBuilder.String(),
	})

	// Process all events, emitting conversation trace via OnEvent.
	if err := a.processRunnerEvents(r.Run(ctx, "system", createSessResp.Session.ID(), userMsg, agent.RunConfig{})); err != nil {
		return nil, fmt.Errorf("analyzer: LLM run failed: %w", err)
	}

	return a.db.ListSuggestions(suggKey)
}

// getRunTestName returns the test name for a run ID, or "" if not found.
func (a *Analyzer) getRunTestName(runID int64) string {
	allRuns, err := a.db.ListRuns(time.Time{}, 0)
	if err != nil {
		return ""
	}
	for _, r := range allRuns {
		if r.ID == runID {
			return r.TestName
		}
	}
	return ""
}

// buildRunContext assembles a comprehensive JSON document with all data the
// database has for a single run: metadata, token summary, and every event
// (including children with full details).  This is injected into the user
// message so the LLM has everything in one shot.
func (a *Analyzer) buildRunContext(runID int64) (string, error) {
	// Fetch run row.
	allRuns, err := a.db.ListRuns(time.Time{}, 0)
	if err != nil {
		return "", fmt.Errorf("list runs: %w", err)
	}
	var targetRun *RunRow
	for i := range allRuns {
		if allRuns[i].ID == runID {
			targetRun = &allRuns[i]
			break
		}
	}
	if targetRun == nil {
		return "", fmt.Errorf("run %d not found", runID)
	}

	// Fetch all events for this run.
	events, err := a.db.ListEvents(runID)
	if err != nil {
		return "", fmt.Errorf("list events: %w", err)
	}

	// Build the context document.
	type childEntry struct {
		ElapsedS float64 `json:"elapsed_s"`
		Kind     string  `json:"kind"`
		Message  string  `json:"message"`
		Details  any     `json:"details,omitempty"`
	}
	type eventEntry struct {
		Seq      int          `json:"seq"`
		ElapsedS float64      `json:"elapsed_s"`
		Kind     string       `json:"kind"`
		Message  string       `json:"message"`
		Details  any          `json:"details,omitempty"`
		Children []childEntry `json:"children,omitempty"`
	}
	type tokenSummary struct {
		InputTokens  int64   `json:"input_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		CostUSD      float64 `json:"cost_usd"`
	}
	type runContext struct {
		RunID        int64         `json:"run_id"`
		TestName     string        `json:"test_name"`
		StartedAt    string        `json:"started_at"`
		FinishedAt   string        `json:"finished_at,omitempty"`
		Passed       *bool         `json:"passed,omitempty"`
		Skipped      bool          `json:"skipped,omitempty"`
		Tags         []string      `json:"tags,omitempty"`
		Experiment   string        `json:"experiment,omitempty"`
		Runner       string        `json:"runner,omitempty"`
		TokenSummary *tokenSummary `json:"token_summary,omitempty"`
		Events       []eventEntry  `json:"events"`
	}

	rc := runContext{
		RunID:     targetRun.ID,
		TestName:  targetRun.TestName,
		StartedAt: targetRun.StartedAt.UTC().Format(time.RFC3339),
		Passed:    targetRun.Passed,
		Skipped:   targetRun.Skipped,
		Tags:      targetRun.Tags,
	}
	if targetRun.FinishedAt != nil {
		rc.FinishedAt = targetRun.FinishedAt.UTC().Format(time.RFC3339)
	}
	if targetRun.Experiment != nil {
		rc.Experiment = *targetRun.Experiment
	}
	if targetRun.Runner != nil {
		rc.Runner = *targetRun.Runner
	}
	if targetRun.TokenSummary != nil {
		rc.TokenSummary = &tokenSummary{
			InputTokens:  targetRun.TokenSummary.InputTokens,
			OutputTokens: targetRun.TokenSummary.OutputTokens,
			CostUSD:      targetRun.TokenSummary.CostUSD,
		}
	}

	for _, e := range events {
		ee := eventEntry{
			Seq:      e.Seq,
			ElapsedS: e.ElapsedS,
			Kind:     e.Kind,
			Message:  e.Message,
		}
		// Parse details JSON into a raw value so it nests properly.
		if e.Details != nil && *e.Details != "" {
			var parsed any
			if json.Unmarshal([]byte(*e.Details), &parsed) == nil {
				ee.Details = parsed
			} else {
				ee.Details = *e.Details // keep as string if not valid JSON
			}
		}
		// Include children with full details.
		for _, c := range e.Children {
			ce := childEntry{
				ElapsedS: c.ElapsedS,
				Kind:     c.Kind,
				Message:  c.Message,
			}
			if c.Details != "" {
				var parsed any
				if json.Unmarshal([]byte(c.Details), &parsed) == nil {
					ce.Details = parsed
				} else {
					ce.Details = c.Details
				}
			}
			ee.Children = append(ee.Children, ce)
		}
		rc.Events = append(rc.Events, ee)
	}

	b, err := json.MarshalIndent(rc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal run context: %w", err)
	}
	return string(b), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool implementations
// ─────────────────────────────────────────────────────────────────────────────

func (a *Analyzer) buildTools() ([]tool.Tool, error) {
	expSummaryTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_experiment_summary",
			Description: "Get high-level statistics for a named experiment (run count, pass/fail rates, tags, last run time).",
		},
		a.toolGetExperimentSummary,
	)
	if err != nil {
		return nil, fmt.Errorf("get_experiment_summary: %w", err)
	}

	listRunsTool, err := functiontool.New(
		functiontool.Config{
			Name:        "list_runs",
			Description: "List all runs in the given experiment with status, timing, and token usage.",
		},
		a.toolListRuns,
	)
	if err != nil {
		return nil, fmt.Errorf("list_runs: %w", err)
	}

	getEventsTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_run_events",
			Description: "Get the structured event log for a single run (tool calls, sections, metrics, trace spans). Capped at 100 rows.",
		},
		a.toolGetRunEvents,
	)
	if err != nil {
		return nil, fmt.Errorf("get_run_events: %w", err)
	}

	getLogTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_run_log",
			Description: "Get the raw flat log file for a run (up to 50 KB). Returns '(log file not available)' when the file is absent.",
		},
		a.toolGetRunLog,
	)
	if err != nil {
		return nil, fmt.Errorf("get_run_log: %w", err)
	}

	createSuggTool, err := functiontool.New(
		functiontool.Config{
			Name:        "create_suggestion",
			Description: "Store one improvement suggestion for the current experiment.",
		},
		a.toolCreateSuggestion,
	)
	if err != nil {
		return nil, fmt.Errorf("create_suggestion: %w", err)
	}

	memAskTool, err := functiontool.New(
		functiontool.Config{
			Name:        "memory_ask",
			Description: "Ask the Memory platform assistant a question. Use this to learn about Memory CLI commands, platform features, or how specific tests work. Returns the assistant's text response.",
		},
		a.toolMemoryAsk,
	)
	if err != nil {
		return nil, fmt.Errorf("memory_ask: %w", err)
	}

	runCLITool, err := functiontool.New(
		functiontool.Config{
			Name:        "run_cli",
			Description: "Run a read-only memory CLI command and return its output. Use this to verify which commands and subcommands actually exist. Only --help, list, get, version, and status commands are allowed.",
		},
		a.toolRunCLI,
	)
	if err != nil {
		return nil, fmt.Errorf("run_cli: %w", err)
	}

	return []tool.Tool{expSummaryTool, listRunsTool, getEventsTool, getLogTool, createSuggTool, memAskTool, runCLITool}, nil
}

// ── get_experiment_summary ────────────────────────────────────────────────────

type getExpSummaryArgs struct {
	Experiment string `json:"experiment"`
}

type getExpSummaryResult struct {
	Result string `json:"result"`
}

func (a *Analyzer) toolGetExperimentSummary(_ tool.Context, args getExpSummaryArgs) (getExpSummaryResult, error) {
	// When analysing a single test, compute summary from runs filtered by test name.
	if a.filterByTestName {
		allRuns, err := a.db.ListRuns(time.Time{}, 0)
		if err != nil {
			return getExpSummaryResult{Result: fmt.Sprintf("error: %v", err)}, nil
		}
		type summary struct {
			TestName  string   `json:"test_name"`
			RunCount  int      `json:"run_count"`
			PassCount int      `json:"pass_count"`
			FailCount int      `json:"fail_count"`
			SkipCount int      `json:"skip_count"`
			LastRunAt string   `json:"last_run_at"`
			Tags      []string `json:"tags"`
		}
		s := summary{TestName: a.currentTestName}
		var lastRunAt time.Time
		tagSet := map[string]bool{}
		for _, r := range allRuns {
			if r.TestName != a.currentTestName {
				continue
			}
			s.RunCount++
			if r.Skipped {
				s.SkipCount++
			} else if r.Passed != nil {
				if *r.Passed {
					s.PassCount++
				} else {
					s.FailCount++
				}
			}
			if r.StartedAt.After(lastRunAt) {
				lastRunAt = r.StartedAt
			}
			for _, t := range r.Tags {
				tagSet[t] = true
			}
		}
		if s.RunCount == 0 {
			return getExpSummaryResult{Result: fmt.Sprintf("no runs found for test: %s", a.currentTestName)}, nil
		}
		if !lastRunAt.IsZero() {
			s.LastRunAt = lastRunAt.UTC().Format(time.RFC3339)
		}
		for t := range tagSet {
			s.Tags = append(s.Tags, t)
		}
		b, _ := json.MarshalIndent(s, "", "  ")
		return getExpSummaryResult{Result: string(b)}, nil
	}

	experiments, err := a.db.ListExperiments()
	if err != nil {
		return getExpSummaryResult{Result: fmt.Sprintf("error: %v", err)}, nil
	}
	for _, exp := range experiments {
		if exp.Name == args.Experiment {
			type summary struct {
				Name      string   `json:"name"`
				RunCount  int      `json:"run_count"`
				PassCount int      `json:"pass_count"`
				FailCount int      `json:"fail_count"`
				SkipCount int      `json:"skip_count"`
				LastRunAt string   `json:"last_run_at"`
				Tags      []string `json:"tags"`
			}
			s := summary{
				Name:      exp.Name,
				RunCount:  exp.RunCount,
				PassCount: exp.PassCount,
				FailCount: exp.FailCount,
				SkipCount: exp.SkipCount,
				LastRunAt: exp.LastRunAt.UTC().Format(time.RFC3339),
				Tags:      exp.Tags,
			}
			b, _ := json.MarshalIndent(s, "", "  ")
			return getExpSummaryResult{Result: string(b)}, nil
		}
	}
	return getExpSummaryResult{Result: fmt.Sprintf("experiment not found: %s", args.Experiment)}, nil
}

// ── list_runs ─────────────────────────────────────────────────────────────────

type listRunsArgs struct {
	Experiment string `json:"experiment"`
}

type listRunsResult struct {
	Result string `json:"result"`
}

func (a *Analyzer) toolListRuns(_ tool.Context, args listRunsArgs) (listRunsResult, error) {
	allRuns, err := a.db.ListRuns(time.Time{}, 0)
	if err != nil {
		return listRunsResult{Result: fmt.Sprintf("error: %v", err)}, nil
	}

	type runSummary struct {
		ID           int64    `json:"id"`
		TestName     string   `json:"test_name"`
		StartedAt    string   `json:"started_at"`
		FinishedAt   string   `json:"finished_at,omitempty"`
		Passed       *bool    `json:"passed,omitempty"`
		Skipped      bool     `json:"skipped,omitempty"`
		EventCount   int      `json:"event_count"`
		Tags         []string `json:"tags,omitempty"`
		InputTokens  *int64   `json:"input_tokens,omitempty"`
		OutputTokens *int64   `json:"output_tokens,omitempty"`
		CostUSD      *float64 `json:"cost_usd,omitempty"`
	}

	var result []runSummary
	for _, r := range allRuns {
		if a.filterByTestName {
			// When analysing a single test, filter by test name instead of experiment.
			if r.TestName != a.currentTestName {
				continue
			}
		} else {
			if r.Experiment == nil || *r.Experiment != args.Experiment {
				continue
			}
		}
		s := runSummary{
			ID:         r.ID,
			TestName:   r.TestName,
			StartedAt:  r.StartedAt.UTC().Format(time.RFC3339),
			EventCount: r.EventCount,
			Tags:       r.Tags,
			Passed:     r.Passed,
			Skipped:    r.Skipped,
		}
		if r.FinishedAt != nil {
			s.FinishedAt = r.FinishedAt.UTC().Format(time.RFC3339)
		}
		if r.TokenSummary != nil {
			s.InputTokens = &r.TokenSummary.InputTokens
			s.OutputTokens = &r.TokenSummary.OutputTokens
			s.CostUSD = &r.TokenSummary.CostUSD
		}
		result = append(result, s)
	}

	b, _ := json.MarshalIndent(result, "", "  ")
	return listRunsResult{Result: string(b)}, nil
}

// ── get_run_events ────────────────────────────────────────────────────────────

type getRunEventsArgs struct {
	RunID int64 `json:"run_id"`
}

type getRunEventsResult struct {
	Result string `json:"result"`
}

func (a *Analyzer) toolGetRunEvents(_ tool.Context, args getRunEventsArgs) (getRunEventsResult, error) {
	events, err := a.db.ListEvents(args.RunID)
	if err != nil {
		return getRunEventsResult{Result: fmt.Sprintf("error: %v", err)}, nil
	}

	const maxRows = 100
	truncated := false
	if len(events) > maxRows {
		events = events[:maxRows]
		truncated = true
	}

	type eventSummary struct {
		Seq        int     `json:"seq"`
		ElapsedS   float64 `json:"elapsed_s"`
		Kind       string  `json:"kind"`
		Message    string  `json:"message"`
		Details    string  `json:"details,omitempty"`
		ChildCount int     `json:"child_count,omitempty"`
	}

	var rows []eventSummary
	for _, e := range events {
		s := eventSummary{
			Seq:      e.Seq,
			ElapsedS: e.ElapsedS,
			Kind:     e.Kind,
			Message:  e.Message,
		}
		if e.Details != nil {
			s.Details = *e.Details
		}
		if len(e.Children) > 0 {
			s.ChildCount = len(e.Children)
		}
		rows = append(rows, s)
	}
	if truncated {
		rows = append(rows, eventSummary{Kind: "(truncated)", Message: fmt.Sprintf("first %d events shown", maxRows)})
	}

	b, _ := json.MarshalIndent(rows, "", "  ")
	return getRunEventsResult{Result: string(b)}, nil
}

// ── get_run_log ───────────────────────────────────────────────────────────────

type getRunLogArgs struct {
	RunID int64 `json:"run_id"`
}

type getRunLogResult struct {
	Result string `json:"result"`
}

func (a *Analyzer) toolGetRunLog(_ tool.Context, args getRunLogArgs) (getRunLogResult, error) {
	// Fetch the run row to get its test name and start time.
	allRuns, err := a.db.ListRuns(time.Time{}, 0)
	if err != nil {
		return getRunLogResult{Result: fmt.Sprintf("error fetching runs: %v", err)}, nil
	}
	var targetRun *RunRow
	for i := range allRuns {
		if allRuns[i].ID == args.RunID {
			targetRun = &allRuns[i]
			break
		}
	}
	if targetRun == nil {
		return getRunLogResult{Result: fmt.Sprintf("run %d not found", args.RunID)}, nil
	}

	// Resolve the logs directory (same logic as db.go dbPath but for logs/).
	logsDir := logsDirPath()

	// The directory name pattern: <timestamp>-<safeName>
	// We glob for dirs matching the run's test name (safe name with / → _).
	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "-").Replace(targetRun.TestName)
	timestamp := targetRun.StartedAt.UTC().Format("2006-01-02T15-04-05Z")
	pattern := filepath.Join(logsDir, fmt.Sprintf("%s-%s", timestamp, safeName))

	// Try exact match first, then glob with wildcard in case of slight format differences.
	candidates, _ := filepath.Glob(pattern)
	if len(candidates) == 0 {
		// Broader glob: just prefix on safe name.
		candidates, _ = filepath.Glob(filepath.Join(logsDir, "*-"+safeName))
	}

	for _, dir := range candidates {
		logPath := filepath.Join(dir, "run.log")
		data, err := os.ReadFile(logPath)
		if err != nil {
			continue
		}
		const maxBytes = 50 * 1024
		if len(data) > maxBytes {
			data = data[:maxBytes]
			data = append(data, []byte("\n... (truncated at 50 KB)")...)
		}
		return getRunLogResult{Result: string(data)}, nil
	}

	return getRunLogResult{Result: "(log file not available)"}, nil
}

// logsDirPath resolves the logs/ directory using the same logic as db.go.
func logsDirPath() string {
	if d := os.Getenv("TEST_LOG_DIR"); d != "" {
		return d
	}
	_, srcFile, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Join(filepath.Dir(srcFile), "..", "logs")
	}
	if _, err := os.Stat("/test-logs"); err == nil {
		return "/test-logs"
	}
	return filepath.Join(os.TempDir(), "memory-cli-docker-tests")
}

// ── create_suggestion ─────────────────────────────────────────────────────────

type createSuggestionArgs struct {
	Title    string  `json:"title"   `
	Body     string  `json:"body"    `
	Category string  `json:"category"`
	Priority string  `json:"priority"`
	RunIDs   []int64 `json:"run_ids,omitempty"`
}

type createSuggestionResult struct {
	Result string `json:"result"`
}

func (a *Analyzer) toolCreateSuggestion(_ tool.Context, args createSuggestionArgs) (createSuggestionResult, error) {
	// Normalise priority.
	priority := strings.ToLower(strings.TrimSpace(args.Priority))
	switch priority {
	case "high", "medium", "low":
		// ok
	default:
		priority = "medium"
	}

	if args.RunIDs == nil {
		args.RunIDs = []int64{}
	}

	exp := a.currentExperiment
	if exp == "" {
		return createSuggestionResult{Result: "error: no active experiment"}, nil
	}

	_, err := a.db.InsertSuggestion(exp, args.Title, args.Body, args.Category, priority, args.RunIDs)
	if err != nil {
		return createSuggestionResult{Result: fmt.Sprintf("error: %v", err)}, nil
	}
	return createSuggestionResult{Result: fmt.Sprintf("suggestion created: %s", args.Title)}, nil
}

// ── memory_ask tool ───────────────────────────────────────────────────────────

type memoryAskArgs struct {
	Question string `json:"question"`
}

type memoryAskResult struct {
	Result string `json:"result"`
}

func (a *Analyzer) toolMemoryAsk(tc tool.Context, args memoryAskArgs) (memoryAskResult, error) {
	resp := memoryAsk(tc, args.Question)
	if resp == "" {
		return memoryAskResult{Result: "(no response — memory CLI may not be available)"}, nil
	}
	return memoryAskResult{Result: resp}, nil
}

// ── run_cli tool ──────────────────────────────────────────────────────────────

type runCLIArgs struct {
	// Args is the list of CLI arguments (e.g. ["mcp-servers", "--help"]).
	Args []string `json:"args"`
}

type runCLIResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

func (a *Analyzer) toolRunCLI(tc tool.Context, args runCLIArgs) (runCLIResult, error) {
	binary := findMemoryBinary()
	if binary == "" {
		return runCLIResult{ExitCode: -1, Stderr: "memory binary not found"}, nil
	}

	// Safety: only allow read-only / help commands.
	if len(args.Args) == 0 {
		return runCLIResult{ExitCode: -1, Stderr: "no arguments provided"}, nil
	}
	if !isSafeCLICommand(args.Args) {
		return runCLIResult{ExitCode: -1, Stderr: "command not allowed — only --help, list, get, status, version, and read-only commands are permitted"}, nil
	}

	cmdCtx, cancel := context.WithTimeout(tc, 10*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(cmdCtx, binary, args.Args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return runCLIResult{ExitCode: -1, Stderr: fmt.Sprintf("exec error: %v", err)}, nil
		}
	}

	// Truncate long output to keep token usage sane.
	out := stdout.String()
	if len(out) > 4000 {
		out = out[:4000] + "\n... (truncated)"
	}
	errOut := stderr.String()
	if len(errOut) > 2000 {
		errOut = errOut[:2000] + "\n... (truncated)"
	}
	return runCLIResult{ExitCode: exitCode, Stdout: out, Stderr: errOut}, nil
}

// isSafeCLICommand returns true if the command is read-only / safe.
// Allowed: --help (anywhere), and known read-only subcommands.
func isSafeCLICommand(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" || a == "help" {
			return true
		}
	}
	if len(args) == 0 {
		return false
	}
	// Allow read-only top-level commands.
	safeRoots := map[string]bool{
		"version": true, "status": true, "completion": true,
		"mcp-guide": true,
	}
	if safeRoots[args[0]] {
		return true
	}
	// Allow "list", "get" subcommands under any parent.
	if len(args) >= 2 {
		sub := args[1]
		if sub == "list" || sub == "get" || sub == "--help" || sub == "-h" {
			return true
		}
	}
	return false
}

// findMemoryBinary locates the memory binary.
func findMemoryBinary() string {
	if home := os.Getenv("HOME"); home != "" {
		candidate := filepath.Join(home, ".memory", "bin", "memory")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if p, err := exec.LookPath("memory"); err == nil {
		return p
	}
	return ""
}

// ── memory ask helper ─────────────────────────────────────────────────────────

// memoryAsk runs `memory ask "<question>"` and returns the response text.
// Returns an empty string (no error) when the binary is not found or the
// command fails — the caller treats the result as optional enrichment.
func memoryAsk(ctx context.Context, question string) string {
	binary := findMemoryBinary()
	if binary == "" {
		return ""
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(cmdCtx, binary, "ask", question)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "" // non-fatal: binary missing, server down, etc.
	}
	return strings.TrimSpace(stdout.String())
}
