package main

import (
	"context"
	"io"

	"github.com/a-h/templ"
	runlog "github.com/emergent-company/runlog"
)

type codeExample struct {
	Title       string
	Description string
	ID          string
	Lang        string
	Code        string
	Events      []runlog.EventRow
}

func strPtr(s string) *string { return &s }
func fltPtr(f float64) *float64 { return &f }

// sdkPageJS is the inline JavaScript for expandable detail rows and language filter tabs.
var sdkPageJS = `window.toggleDetail=function(e){var t=document.getElementById(e);t&&t.classList.toggle('hidden')};document.addEventListener('click',function(e){var t=e.target.closest('[data-detail-id]');t&&toggleDetail(t.getAttribute('data-detail-id'));var f=e.target.closest('.sdk-lang-tabs .tab');if(f){var g=f.closest('.sdk-lang-tabs');var l;if(f.classList.contains('tab-active')){f.classList.remove('tab-active');l='all'}else{g.querySelectorAll('.tab').forEach(function(x){x.classList.remove('tab-active')});f.classList.add('tab-active');l=f.getAttribute('data-filter')}g.closest('.sdk-page').querySelectorAll('.sdk-example').forEach(function(x){x.style.display=l==='all'||x.dataset.lang===l?'':'none'})}});setTimeout(function(){var t=document.querySelector('.sdk-lang-tabs .tab-active');if(t){var l=t.getAttribute('data-filter');t.closest('.sdk-page').querySelectorAll('.sdk-example').forEach(function(x){x.style.display=x.dataset.lang===l?'':'none'})}},0)`

// scriptTag returns a templ component that writes a <script> tag with the given content.
// Use instead of inline <script>{ var }</script> which templ treats as raw text.
func scriptTag(js string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<script>"+js+"</script>")
		return err
	})
}

// styleTag returns a templ component that writes a <style> tag with the given content.
// Use instead of inline <style>{ var }</style> which templ treats as raw text.
func styleTag(css string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<style>"+css+"</style>")
		return err
	})
}

// sdkPageCSS provides the grid layout for the SDK reference page sidebar.
var sdkPageCSS = `.sdk-page { display: grid; grid-template-columns: 14rem 1fr; gap: 1.5rem; align-items: start; }
@media (max-width: 768px) { .sdk-page { grid-template-columns: 1fr; } }`

var codeExamples = []codeExample{
	{
		Title:       "Basic Setup",
		Description: "Creates a RunLog that registers the test in the database, captures structured events, and persists results on Close. Call at the start of every test that uses runlog.",
		ID:          "new-runlog", Lang: "go",
		Code: `import runlog "github.com/emergent-company/runlog"

func TestMyFeature(t *testing.T) {
    rl := runlog.NewRunLog(t)
    defer rl.Close()
    rl.SetCategory("api/auth")
    rl.SetTimeout(5 * time.Minute)
    rl.Printf("starting test...")
}`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "log", Message: "starting test...", ElapsedS: 0.5},
		},
	},
	{
		Title:       "HTTP Call",
		Description: "Captures HTTP request/response details as structured http_call events. Records method, URL, status code, and response body.",
		ID:          "http-call", Lang: "go",
		Code: `rl := runlog.NewRunLog(t)
defer rl.Close()

start := time.Now()
resp, err := http.Get(server.URL + "/api/health")
body, _ := io.ReadAll(resp.Body)
resp.Body.Close()

rl.HTTPCall("GET", "/api/health",
    resp.StatusCode, "", string(body), time.Since(start))`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "http_call", Message: "GET /api/health → 200", ElapsedS: 0.3, DurationMs: fltPtr(45.2), Details: strPtr(`{"method":"GET","url":"/api/health","status_code":200,"response_body":"{\"status\":\"ok\"}"}`)},
		},
	},
	{
		Title:       "CLI Capture",
		Description: "Wraps any CLI command execution and records the full command, exit code, and stdout/stderr as a cli event.",
		ID:          "cli-capture", Lang: "go",
		Code: `rl := runlog.NewRunLog(t)
defer rl.Close()

cmd := exec.Command("kubectl", "apply", "-f", "deploy.yaml")
output, err := cmd.CombinedOutput()
rl.CLIErr("kubectl apply -f deploy.yaml", string(output), err)`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "cli", Message: "$ kubectl apply -f deploy.yaml", ElapsedS: 0.1, Details: strPtr(`{"command":"kubectl apply -f deploy.yaml","exit_code":0,"output":"deployment.apps/my-app created\n"}`)},
		},
	},
	{
		Title:       "Fail",
		Description: "Records a failure event with the error message and exits the test via t.Fatal.",
		ID:          "failf", Lang: "go",
		Code: `rl.Failf("expected status 200, got %d", resp.StatusCode)`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "failure", Message: "expected status 200, got 500", ElapsedS: 1.2},
			{Seq: 3, Kind: "state_change", Message: "test finished", ElapsedS: 1.2},
		},
	},
	{
		Title:       "Section",
		Description: "Groups related events under named sections. Section children are collapsed by default and expand on click.",
		ID:          "section", Lang: "go",
		Code: `rl.Section("Setup")
rl.CLI("go build ./...")
rl.Printf("build succeeded")

rl.Section("Main assertions")
rl.Printf("testing login flow...")`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "section", Message: "Setup", ElapsedS: 0.1, Children: []runlog.ChildEvent{
				{ElapsedS: 0.2, Kind: "cli", Message: "go build ./..."},
				{ElapsedS: 0.5, Kind: "log", Message: "build succeeded"},
			}},
			{Seq: 3, Kind: "section", Message: "Main assertions", ElapsedS: 0.6, Children: []runlog.ChildEvent{
				{ElapsedS: 0.7, Kind: "log", Message: "testing login flow..."},
			}},
		},
	},
	{
		Title:       "Tag",
		Description: "Attaches key:value tags to a run for filtering and comparison.",
		ID:          "tagging", Lang: "go",
		Code: `rl.Tag("model", "gpt-4")
rl.Tag("variant", "baseline")
rl.SetExperiment("prompt-optimization-v3")`,
	},
	{
		Title:       "Screenshots",
		Description: "Emits screenshot artifacts from Playwright e2e tests. The UI renders the image inline in the event detail.",
		ID:          "playwright-screenshots", Lang: "javascript",
		Code: `// In your Playwright test fixture (run from project root):
const artifactsDir = process.env.RUNLOG_ARTIFACTS_DIR || '.runlog/artifacts';

async function saveArtifact(runId, page, name) {
    const ssPath = path.join(artifactsDir, runId, name + '.png');
    await page.screenshot({ path: ssPath });

    await fetch(DOGFOOD_URL + '/runs/' + runId + '/events', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            kind: 'artifact',
            message: 'screenshot: ' + name,
            details: {
                type: 'screenshot',
                url: '/artifact/' + runId + '/' + name + '.png',
                mime: 'image/png'
            }
        })
    });
}

await saveArtifact(runId, page, 'after-login');`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "artifact", Message: "screenshot: after-login", ElapsedS: 2.5, Details: strPtr(`{"type":"screenshot","url":"/artifact/run-42/after-login.png","mime":"image/png"}`)},
		},
	},
	{
		Title:       "Daemon",
		Description: "NewRunLog auto-registers with the daemon when RUNLOG_DAEMON_URL is set, falling back to direct DB writes otherwise.",
		ID:          "daemon-integration", Lang: "go",
		Code: `rl := runlog.NewRunLog(t)
defer rl.Close()

out, err := exec.Command("runlog", "runs", "--since", "1h").CombinedOutput()
rl.CLIErr("runlog runs --since 1h", string(out), err, 0)

rl.HTTPCall("GET", "/health", 200, "", "ok", 0)`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "cli", Message: "$ runlog runs --since 1h", ElapsedS: 0.1, Details: strPtr(`{"command":"runlog runs --since 1h","exit_code":0}`)},
			{Seq: 3, Kind: "http_call", Message: "GET /health → 200", ElapsedS: 0.3, Details: strPtr(`{"method":"GET","url":"/health","status_code":200}`)},
		},
	},

	// ── TypeScript SDK examples ──

	{
		Title:       "Basic Setup",
		Description: "Creates a RunLogClient (HTTP client for runlog daemon), registers a test run, adds a section and log event, then marks the run as done.",
		ID:          "ts-new-client", Lang: "typescript",
		Code: `import { RunLogClient } from "@emergent-company/runlog-client"

const client = new RunLogClient()
const { id: runId } = await client.createRun({
    testName: "Login flow",
    category: "auth",
    tags: ["variant:baseline", "browser:chromium"],
})

await client.section(runId, "Navigate to login")
await client.log(runId, "page loaded successfully")

await client.markDone(runId, { passed: true })`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "section", Message: "Navigate to login", ElapsedS: 0.1, Children: []runlog.ChildEvent{
				{ElapsedS: 0.2, Kind: "log", Message: "page loaded successfully"},
			}},
			{Seq: 3, Kind: "state_change", Message: "test finished", ElapsedS: 0.5},
		},
	},
	{
		Title:       "CLI Capture",
		Description: "Records shell command invocations with optional stdout as cli events via the daemon HTTP API.",
		ID:          "ts-cli-capture", Lang: "typescript",
		Code: `import { execSync } from "node:child_process"

const output = execSync("npm test -- --coverage").toString()
await client.cli(runId, "npm test -- --coverage", output)`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "cli", Message: "npm test -- --coverage", ElapsedS: 0.1, Details: strPtr(`{"command":"npm test -- --coverage","exit_code":0,"output":"12 tests passed\\n"}`)},
		},
	},
	{
		Title:       "Error Handling",
		Description: "Records failures and marks the run as failed. The fail() helper writes a failure event; markDone({ passed: false }) sets the overall outcome.",
		ID:          "ts-error-handling", Lang: "typescript",
		Code: `try {
    const resp = await fetch(apiUrl + "/users")
    if (!resp.ok) throw new Error("unexpected status " + resp.status)
} catch (err) {
    await client.fail(runId, err.message)
    await client.markDone(runId, { passed: false, reason: err.message })
    throw err  // re-throw so test runner sees the failure
}`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "failure", Message: "unexpected status 500", ElapsedS: 1.2},
			{Seq: 2, Kind: "state_change", Message: "test finished", ElapsedS: 1.3},
		},
	},
	{
		Title:       "Metadata",
		Description: "Sets category, tags, experiment, and description via per-field PUT endpoints. Tags use key:value convention for cross-run filtering.",
		ID:          "ts-metadata", Lang: "typescript",
		Code: `await client.setCategory(runId, "api/payments")
await client.setTags(runId, ["model:gpt-4o", "variant:canary"])
await client.setExperiment(runId, "prompt-optimization-v3")
await client.setDescription(runId, "Verifies payment creation, refund, and idempotency")

await client.addEvent(runId, {
    kind: "log",
    message: "experiment:prompt-optimization-v3 model:gpt-4o",
})`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "log", Message: "experiment:prompt-optimization-v3 model:gpt-4o", ElapsedS: 0.1},
		},
	},
	{
		Title:       "Jest Reporter",
		Description: "Drop-in Jest custom reporter. Each test file creates a run, each test() emits an assertion event, and the file result calls markDone. Import in jest.config.",
		ID:          "ts-jest-reporter", Lang: "typescript",
		Code: `// jest.config.ts
import type { Config } from "jest"

const config: Config = {
    reporters: [
        "default",
        "@emergent-company/runlog-client/jest",
    ],
}
export default config`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "section", Message: "src/auth/login.test.ts", ElapsedS: 0.1, Children: []runlog.ChildEvent{
				{ElapsedS: 0.2, Kind: "assertion", Message: "redirects to dashboard on success"},
				{ElapsedS: 0.5, Kind: "assertion", Message: "shows error on invalid credentials"},
			}},
			{Seq: 3, Kind: "state_change", Message: "test finished", ElapsedS: 0.6},
		},
	},
	{
		Title:       "Playwright Fixture",
		Description: "Auto-fixture applied to every Playwright test. Automatically captures network calls as http_call events, uncaught page errors as log events, and uploads screenshots + traces as artifacts on completion.",
		ID:          "ts-playwright-fixture", Lang: "typescript",
		Code: `// playwright.config.ts
import { defineConfig } from "@playwright/test"
export default defineConfig({
    testMatch: "**/*.spec.ts",
    use: { baseURL: "http://localhost:3000" },
})

// tests/auth/login.spec.ts
import { test, expect } from "@emergent-company/runlog-client/playwright"

test("login flow", async ({ page, runlog }) => {
    await runlog.section("Navigate to login")
    await page.goto("/login")

    await runlog.section("Submit credentials")
    await page.fill("[name=email]", "user@example.com")
    await page.fill("[name=password]", "s3cret")
    await page.click("[type=submit]")

    await expect(page).toHaveURL("/dashboard")
    await runlog.log("redirected to /dashboard")

    // screenshot + trace auto-uploaded on test completion
})`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "section", Message: "Navigate to login", ElapsedS: 0.1, Children: []runlog.ChildEvent{
				{ElapsedS: 0.2, Kind: "http_call", Message: "GET /login → 200"},
			}},
			{Seq: 3, Kind: "section", Message: "Submit credentials", ElapsedS: 0.3, Children: []runlog.ChildEvent{
				{ElapsedS: 0.4, Kind: "http_call", Message: "POST /api/auth/login → 200"},
				{ElapsedS: 0.5, Kind: "log", Message: "redirected to /dashboard"},
			}},
			{Seq: 4, Kind: "artifact", Message: "screenshot", ElapsedS: 0.6, Details: strPtr(`{"type":"screenshot","url":"/artifact/run-abc/screenshot.png","mime":"image/png"}`)},
			{Seq: 5, Kind: "artifact", Message: "trace", ElapsedS: 0.7, Details: strPtr(`{"type":"trace","url":"/artifact/run-abc/trace.zip","mime":"application/zip"}`)},
			{Seq: 6, Kind: "state_change", Message: "test finished", ElapsedS: 0.8},
		},
	},
}
