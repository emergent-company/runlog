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

// sdkPageJS is the inline JavaScript for expandable detail rows and copy-to-clipboard.
var sdkPageJS = `window.toggleDetail=function(e){var t=document.getElementById(e);t&&t.classList.toggle('hidden')};document.addEventListener('click',function(e){var t=e.target.closest('[data-detail-id]');t&&toggleDetail(t.getAttribute('data-detail-id'))});window.copyCode=function(e){var t=e.parentElement;var n=t.querySelector('input[type="radio"]:checked');var r;if(n){r=n.nextElementSibling&&n.nextElementSibling.querySelector('code')}else{r=t.querySelector('code')}if(r){var i=r.innerText;navigator.clipboard.writeText(i).then(function(){var o=e.innerHTML;e.innerHTML='<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"><\/polyline><\/svg>';setTimeout(function(){e.innerHTML=o},2000)})}}`

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
		Title: "Basic Setup",
		Description: "Creates a RunLog that registers the test in the database, captures structured events, and persists results on Close. Call at the start of every test that uses runlog.",
		ID: "new-runlog", Lang: "go",
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
		Title: "HTTP Call",
		Description: "Captures HTTP request/response details as structured http_call events. Records method, URL, status code, and response body.",
		ID: "http-call", Lang: "go",
		Code: `rec := runlog.NewRunRecorder(db)
rec.RegisterRun("my-test")

resp, err := http.Get(server.URL + "/api/health")
body, _ := io.ReadAll(resp.Body)

rec.HTTPCall("GET", "/api/health",
    resp.StatusCode, "", string(body))`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "http_call", Message: "GET /api/health → 200", ElapsedS: 0.3, Details: strPtr(`{"method":"GET","url":"/api/health","status_code":200,"response_body":"{\"status\":\"ok\"}"}`)},
		},
	},
	{
		Title: "CLI Capture",
		Description: "Wraps any CLI command execution and records the full command, exit code, and stdout/stderr as a cli event.",
		ID: "cli-capture", Lang: "go",
		Code: `rec := runlog.NewRunRecorder(db)
rec.RegisterRun("deploy-test")

output := rec.CLICapture("kubectl apply -f deploy.yaml", func() error {
    cmd := exec.Command("kubectl", "apply", "-f", "deploy.yaml")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
})`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "cli", Message: "$ kubectl apply -f deploy.yaml", ElapsedS: 0.1, Details: strPtr(`{"command":"kubectl apply -f deploy.yaml","exit_code":0,"output":"deployment.apps/my-app created\n"}`)},
		},
	},
	{
		Title: "Fail",
		Description: "Records a failure event with the error message and exits the test via t.Fatal.",
		ID: "failf", Lang: "go",
		Code: `rl.Failf("expected status 200, got %d", resp.StatusCode)`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "failure", Message: "expected status 200, got 500", ElapsedS: 1.2},
			{Seq: 3, Kind: "state_change", Message: "test finished", ElapsedS: 1.2},
		},
	},
	{
		Title: "Section",
		Description: "Groups related events under named sections. Section children are collapsed by default and expand on click.",
		ID: "section", Lang: "go",
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
		Title: "Tag",
		Description: "Attaches key:value tags to a run for filtering and comparison.",
		ID: "tagging", Lang: "go",
		Code: `rl.Tag("model", "gpt-4")
rl.Tag("variant", "baseline")
rl.SetExperiment("prompt-optimization-v3")`,
	},
	{
		Title: "Screenshots",
		Description: "Emits screenshot artifacts from Playwright e2e tests. The UI renders the image inline in the event detail.",
		ID: "playwright-screenshots", Lang: "javascript",
		Code: `// In your Playwright test fixture:
const artifactsDir = '/tmp/runlog-artifacts';

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
		Title: "Daemon",
		Description: "RunRecorder auto-registers with the daemon when RUNLOG_DAEMON_URL is set.",
		ID: "daemon-integration", Lang: "go",
		Code: `rec := runlog.NewRunRecorder(db)
runID, _ := rec.RegisterRun("my-test-suite")
defer rec.MarkDone(runID, !t.Failed())

output := rec.CLICapture("runlog runs --since 1h", func() error {
    return cmdRuns(db, 24*time.Hour)
})
rec.HTTPCall("GET", "/health", 200, "", "ok")`,
		Events: []runlog.EventRow{
			{Seq: 1, Kind: "state_change", Message: "test started", ElapsedS: 0.0},
			{Seq: 2, Kind: "cli", Message: "$ runlog runs --since 1h", ElapsedS: 0.1, Details: strPtr(`{"command":"runlog runs --since 1h","exit_code":0}`)},
			{Seq: 3, Kind: "http_call", Message: "GET /health → 200", ElapsedS: 0.3, Details: strPtr(`{"method":"GET","url":"/health","status_code":200}`)},
		},
	},
}
