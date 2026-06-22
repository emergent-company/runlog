// Package runlog — fixture.go
//
// Fixture: a thin declarative entry point for e2e tests.
//
// Replace the 7–8 line ritual:
//
//	rl := newRunLog(t); t.Cleanup(rl.Close)
//	home := t.TempDir()
//	requireServerReady(t, home)
//	srv := serverURL()
//	name := uniqueProjectName("e2e-foo")
//	projectID := createProject(t, home, srv, name)
//	deleteProjectOnCleanup(t, home, projectID)
//
// With a single call:
//
//	fx := framework.Use(t, framework.WithProject("e2e-foo"))
//
// Then run CLI commands with no extra flags:
//
//	out := fx.CLI("documents", "list")
//	out.Contains("test-document")
//
// Coexists with NewTest/TestContext — no existing tests need to change.
package runlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture is the unexported builder mutated by Option functions before Use
// builds and returns the exported *Fixture view.
type fixture struct {
	t          *testing.T
	home       string
	server     string
	token      string
	projectID  string
	binary     string
	rl         *RunLog
	hasProject bool
}

// Fixture is the exported handle the test body receives from Use.  All fields
// are populated after Use returns.
type Fixture struct {
	// T is the test instance.
	T *testing.T
	// Home is the isolated temp directory used as HOME for CLI invocations.
	Home string
	// Server is the URL of the server under test (from ServerURL()).
	Server string
	// Token is the auth token (from E2ETestToken()).
	Token string
	// ProjectID is the ID of the project created by WithProject.
	// Empty if WithProject was not used.
	ProjectID string
	// Binary is the CLI binary name.  Defaults to "memory".
	Binary string
	// RunLog is the structured log for this test.
	RunLog *RunLog
}

// Option is a functional option applied by Use before returning the Fixture.
// Options are applied in the order they are passed to Use.
type Option func(*fixture)

// Use is the single entry point for declarative e2e test setup.  It:
//  1. Creates a RunLog and registers rl.Close via t.Cleanup
//  2. Creates an isolated temp dir as Fixture.Home
//  3. Checks server readiness (skips test if server is unreachable or auth fails)
//  4. Applies each Option in order
//  5. Returns a fully-initialized *Fixture
//
// Example:
//
//	fx := framework.Use(t, framework.WithProject("e2e-docs"))
//	out := fx.CLI("documents", "list")
//	out.Contains("[]")
func Use(t *testing.T, opts ...Option) *Fixture {  //nolint:deadcode
	t.Helper()

	rl := NewRunLog(t)
	t.Cleanup(func() { rl.Close() })

	home := t.TempDir()
	server := ServerURL()
	token := E2ETestToken()

	// Check server readiness — skips test if server is down or auth fails.
	// RequireServerReady also calls SetupCLIAuth internally.
	RequireServerReady(t, home, rl)

	b := &fixture{
		t:      t,
		home:   home,
		server: server,
		token:  token,
		binary: "memory",
		rl:     rl,
	}

	for _, opt := range opts {
		opt(b)
	}

	return &Fixture{
		T:         b.t,
		Home:      b.home,
		Server:    b.server,
		Token:     b.token,
		ProjectID: b.projectID,
		Binary:    b.binary,
		RunLog:    b.rl,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Option functions
// ─────────────────────────────────────────────────────────────────────────────

// WithProject creates an ephemeral project with the given name prefix and
// registers its deletion via t.Cleanup (LIFO, automatic).  The project ID is
// stored in Fixture.ProjectID and written to the config file in Fixture.Home
// so that all subsequent CLI invocations find the right project without
// needing a --project flag.
//
// WithProject must appear before WithSchema and WithDocument in the options list.
func WithProject(prefix string) Option {  //nolint:deadcode
	return func(b *fixture) {
		b.t.Helper()
		name := UniqueProjectName(prefix)
		projectID := CreateProject(b.t, b.home, b.server, name)
		DeleteProjectOnCleanup(b.t, b.home, projectID)
		b.projectID = projectID
		b.hasProject = true
		b.rl.Printf("project: %s (%s)", name, projectID)
	}
}

// WithSchema uploads a schema file to the project after it has been created.
// Panics with a clear message if WithProject does not precede WithSchema.
// Fails the test via t.Fatal if the upload fails.
func WithSchema(filePath string) Option {  //nolint:deadcode
	return func(b *fixture) {
		b.t.Helper()
		if !b.hasProject {
			panic("framework.WithSchema requires WithProject to appear first in the options list")
		}
		fx := fixtureToFixture(b)
		fx.CLI("schemas", "upload", filePath)
	}
}

// WithDocument uploads a document file to the project after it has been created.
// Panics with a clear message if WithProject does not precede WithDocument.
// Fails the test via t.Fatal if the upload fails.
func WithDocument(filePath string) Option {  //nolint:deadcode
	return func(b *fixture) {
		b.t.Helper()
		if !b.hasProject {
			panic("framework.WithDocument requires WithProject to appear first in the options list")
		}
		fx := fixtureToFixture(b)
		fx.CLI("documents", "upload", filePath)
	}
}

// WithBinary overrides the CLI binary name.  Defaults to "memory".
func WithBinary(bin string) Option {  //nolint:deadcode
	return func(b *fixture) {
		b.binary = bin
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Fixture methods
// ─────────────────────────────────────────────────────────────────────────────

// CLI runs fx.Binary with the given args using fx.Home as HOME.  The invocation
// and output are logged to fx.RunLog.  Fails the test via t.Fatal if the command
// exits non-zero.  Returns a *CLIResult for chainable assertions.
//
// No --project, --server, or token flags are needed: WithProject already wrote
// project_id, server_url, and credentials to fx.Home/.memory/config.yaml.
func (fx *Fixture) CLI(args ...string) *CLIResult {  //nolint:deadcode
	fx.T.Helper()
	invocation := formatInvocation(fx.Binary, args)

	out, err := RunBinaryInDirWithHome(fx.T, fx.Binary, "", fx.Home, args...)

	fx.RunLog.CLIStepErr(invocation, invocation, strings.TrimSpace(out), err)

	if err != nil {
		fx.RunLog.Failf("CLI command failed: %s\nerror: %v\noutput:\n%s", invocation, err, out)
	}

	return newCLIResultFromCombined(fx.RunLog, out, nil)
}

// CLIExpectError runs fx.Binary with the given args but does NOT fail the test
// on non-zero exit.  The exit code and output are captured in the returned
// *CLIResult for assertion.
func (fx *Fixture) CLIExpectError(args ...string) *CLIResult {  //nolint:deadcode
	fx.T.Helper()
	invocation := formatInvocation(fx.Binary, args)

	out, err := RunBinaryInDirWithHome(fx.T, fx.Binary, "", fx.Home, args...)

	fx.RunLog.CLIStepErr(invocation, invocation, strings.TrimSpace(out), err)

	return newCLIResultFromCombined(fx.RunLog, out, err)
}

// TempFile writes content to a file named name inside a fresh t.TempDir() and
// returns the absolute path.  Fails the test via t.Fatal if the write fails.
//
// Multiple TempFile calls each return a distinct path.
func (fx *Fixture) TempFile(name, content string) string {  //nolint:deadcode
	fx.T.Helper()
	dir := fx.T.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fx.T.Fatalf("TempFile: failed to write %s: %v", path, err)
	}
	return path
}

// Log writes a formatted message to fx.RunLog.
func (fx *Fixture) Log(format string, args ...any) {  //nolint:deadcode
	fx.RunLog.Printf(format, args...)
}

// Section starts a named section in fx.RunLog (mirrors RunLog.Section).
func (fx *Fixture) Section(name string) {  //nolint:deadcode
	fx.RunLog.Section(name)
}

// ─────────────────────────────────────────────────────────────────────────────
// internal helpers
// ─────────────────────────────────────────────────────────────────────────────

// fixtureToFixture creates a temporary *Fixture view of the builder so that
// Option functions (WithSchema, WithDocument) can call Fixture.CLI internally
// during setup without duplicating the CLI invocation logic.
func fixtureToFixture(b *fixture) *Fixture {  //nolint:deadcode
	return &Fixture{
		T:         b.t,
		Home:      b.home,
		Server:    b.server,
		Token:     b.token,
		ProjectID: b.projectID,
		Binary:    b.binary,
		RunLog:    b.rl,
	}
}
