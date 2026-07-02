// Package e2eframework — test_context.go
//
// TestContext: the top-level entry point for the structured step API.
// A test calls NewTest once to set up RunLog, isolated HOME, CLI auth,
// server readiness, description, tags, and an optional project — then uses
// tc.Step to run scoped action blocks.
package runlog

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// TestOpts configures a TestContext created by NewTest.
type TestOpts struct {
	// Describe is a one-line summary of what the test verifies.
	Describe string
	// Bullets are optional detail points shown in the TUI inspector.
	Bullets []string
	// Project, when non-empty, causes NewTest to create an ephemeral project
	// with this name prefix and register deletion on t.Cleanup.
	Project string
	// Tags are variant tags applied at creation (e.g. "model:gemini").
	Tags []string
	// AppVersion records the version of the application-under-test.  When set,
	// it is stored in test_runs.app_version and emitted as an app_version event.
	AppVersion string
	// Binary is the CLI binary to execute in Step.CLI calls.
	// Defaults to "memory" when empty.
	Binary string
	// TestType overrides the auto-derived test classification
	// (unit/integration/e2e/component/etc.). When empty, the type is derived
	// from the test source file name.
	TestType string
}

// TestContext bundles the per-test lifecycle state: a *testing.T, a RunLog,
// an isolated home directory, server URL, auth token, project ID, and the
// configured binary name.  All fields are exported for direct access.
type TestContext struct {
	T         *testing.T
	RunLog    *RunLog
	Home      string
	Server    string
	Token     string
	ProjectID string
	Binary    string

	doneOnce sync.Once
}

// NewTest creates a TestContext with automatic preamble:
//  1. Creates a RunLog and registers rl.Close via t.Cleanup
//  2. Creates an isolated temp dir as Home
//  3. Checks server readiness (skips test if server is down)
//  4. Sets up CLI auth in the isolated home
//  5. Calls rl.Describe with opts.Describe and opts.Bullets
//  6. Applies opts.Tags
//  7. If opts.Project is non-empty, creates the project and registers cleanup
//
// The caller should defer tc.Done() immediately after NewTest.
func NewTest(t *testing.T, opts TestOpts) *TestContext { //nolint:deadcode
	t.Helper()

	binary := opts.Binary
	if binary == "" {
		binary = "memory"
	}

	rl := NewRunLog(t)
	t.Cleanup(func() { rl.Close() })

	home := t.TempDir()
	server := ServerURL()
	token := E2ETestToken()

	// Check server readiness — skip if server is down.
	SkipIfServerDown(t, rl)

	// Set up CLI auth in the isolated home.
	SetupCLIAuth(t, home)

	// Set description.
	if opts.Describe != "" {
		rl.Describe(opts.Describe, opts.Bullets...)
	}

	// Apply tags.
	if len(opts.Tags) > 0 {
		rl.Tag(opts.Tags...)
	}

	// Override test type (if provided).
	if opts.TestType != "" {
		rl.testType = opts.TestType
	}

	// Set app version (if provided).
	if opts.AppVersion != "" {
		rl.SetAppVersion(opts.AppVersion)
	}

	tc := &TestContext{
		T:      t,
		RunLog: rl,
		Home:   home,
		Server: server,
		Token:  token,
		Binary: binary,
	}

	// Optionally create a project.
	if opts.Project != "" {
		projectID := CreateProject(t, home, server, opts.Project)
		tc.ProjectID = projectID
		DeleteProjectOnCleanup(t, home, projectID)
		rl.Printf("project: %s (%s)", opts.Project, projectID)
	}

	return tc
}

// Step creates a RunLog Section, constructs a Step, calls fn, and records
// the step duration.  Use this to organize test logic into named blocks:
//
//	tc.Step("Create agent", func(s *Step) {
//	    s.CLI("agents", "create", "--name", "test").Contains("Created")
//	})
func (tc *TestContext) Step(name string, fn func(s *Step)) { //nolint:deadcode
	tc.T.Helper()
	tc.RunLog.Section(name)

	s := &Step{
		tc:      tc,
		name:    name,
		startAt: time.Now(),
	}

	fn(s)

	elapsed := time.Since(s.startAt)
	tc.RunLog.Printf("step %q completed in %s", name, elapsed.Round(time.Millisecond))
}

// Done is an idempotent finalizer.  Tests should call defer tc.Done()
// immediately after NewTest.  It is safe to call multiple times.
// Currently a no-op placeholder for future cleanup logic (RunLog.Close is
// handled by t.Cleanup registered in NewTest).
func (tc *TestContext) Done() { //nolint:deadcode
	tc.doneOnce.Do(func() {
		// Placeholder for future finalization logic.
		// RunLog.Close is already registered via t.Cleanup in NewTest.
	})
}

// Log writes a log event to RunLog.
func (tc *TestContext) Log(format string, args ...any) { //nolint:deadcode
	tc.RunLog.Printf(format, args...)
}

// Tag appends variant tags to the RunLog.
func (tc *TestContext) Tag(tags ...string) { //nolint:deadcode
	tc.RunLog.Tag(tags...)
}

// Skip records a skip reason and skips the test.
func (tc *TestContext) Skip(reason string) { //nolint:deadcode
	tc.T.Helper()
	tc.RunLog.Skipf("%s", reason)
}

// ─────────────────────────────────────────────────────────────────────────────
// TestMain integration — validates environment before running any tests
// ─────────────────────────────────────────────────────────────────────────────

// EnvTestMain runs environment validation + scripts before m.Run().
// Call from TestMain to fail-fast if env is not ready:
//
//	func TestMain(m *testing.M) {
//	    os.Exit(runlog.EnvTestMain(m, "mcj-emergent"))
//	}
//
// It loads config, validates all requirements, runs test_script and
// setup_script (if configured), then runs m.Run(). Returns 1 if any
// env check fails, m.Run() exit code otherwise.
func EnvTestMain(m *testing.M, envName string) int { //nolint:deadcode
	cfg, err := LoadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "env: cannot load config: %v\n", err)
		return 1
	}

	env := cfg.LookupEnvironment(envName)
	if env == nil {
		fmt.Fprintf(os.Stderr, "env: environment %q not found in config\n", envName)
		return 1
	}

	fmt.Printf("═══ Environment: %s ═══\n\n", envName)

	// 1. Validate requirements
	if err := ValidateEnvSummary(env); err != nil {
		fmt.Fprintf(os.Stderr, "env: %v\n", err)
		return 1
	}
	fmt.Printf("  ✓ requirements passed\n\n")

	// 2. Run test_script if configured
	if env.TestScript != "" {
		fmt.Printf("  Running test_script: %s\n", env.TestScript)
		if err := runEnvScript(env.TestScript); err != nil {
			fmt.Fprintf(os.Stderr, "env: test_script failed: %v\n", err)
			return 1
		}
		fmt.Printf("  ✓ test_script passed\n\n")
	}

	// 3. Run setup_script if configured
	if env.SetupScript != "" {
		fmt.Printf("  Running setup_script: %s\n", env.SetupScript)
		if err := runEnvScript(env.SetupScript); err != nil {
			fmt.Fprintf(os.Stderr, "env: setup_script failed: %v\n", err)
			return 1
		}
		fmt.Printf("  ✓ setup_script passed\n\n")
	}

	fmt.Printf("═══ Running tests ═══\n\n")
	return m.Run()
}

// runEnvScript executes a shell script with output forwarded to stderr.
func runEnvScript(script string) error { //nolint:deadcode
	wd, _ := os.Getwd()
	cmd := exec.Command("sh", script)
	if wd != "" {
		cmd.Dir = wd
	}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
