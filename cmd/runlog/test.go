// cmd/runlog/test.go — "runlog test" subcommand implementation.
//
// runlog test loads .env (and optionally .env.<profile>) from the current
// working directory, then execs the go test command with those variables in
// the environment.  It is the binary equivalent of the ./test shell script
// used in emergent.memory.e2e projects.
//
// Usage:
//
//	runlog test [<env-profile>] [<test-filter>] [-- <extra go test flags>]
//
// The first bare positional argument (if it does not start with "-") is
// treated as a MEMORY_TEST_ENV profile name; the corresponding .env.<profile>
// file is loaded as an overlay on top of .env.  The second bare word is
// passed as a -run filter to go test.  Everything after "--" is forwarded
// verbatim to go test.
//
// Shell-exported variables always take precedence over values from .env files.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	runlog "github.com/emergent-company/runlog"
)

// cmdTest implements "runlog test [<profile>] [<filter>] [-- <flags>]".
//
// It loads environment variables from .env / .env.<profile> in the working
// directory (shell vars always win), prints a short summary, then execs
// "go test" so the test process replaces the runlog process and inherits the
// enriched environment.
func cmdTest(args []string) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var experiment string
	fs.StringVar(&experiment, "experiment", "", "tag all runs in this batch with an experiment name for later comparison")
	fs.StringVar(&experiment, "e", "", "shorthand for --experiment")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `runlog test — load .env and run go test

USAGE
  runlog test [<profile>] [<filter>] [-- <extra go test flags>]

ARGUMENTS
  <profile>   optional env profile name; loads .env.<profile> as an overlay
              (same as setting MEMORY_TEST_ENV=<profile> in your shell)
  <filter>    optional go test -run filter, e.g. TestMyFeature

FLAGS
  -e, --experiment <name>   tag all runs in this batch; compare later with
                            "runlog experiments"
  -- <flags>  all arguments after -- are forwarded verbatim to go test

EXAMPLES
  runlog test                                        # all tests, .env defaults
  runlog test mcj-emergent                           # overlay .env.mcj-emergent
  runlog test localhost TestCLI_Version              # named env + single test
  runlog test -e baseline localhost                  # tag as baseline
  runlog test -e after-fix localhost TestCLI_Version # tag a specific run
  runlog test -- -count=1 -timeout 5m               # pass raw go test flags
`)
	}

	// Parse flags up to the first "--" separator; everything after is extra.
	var extraFlags []string
	cutArgs := args
	for i, a := range args {
		if a == "--" {
			extraFlags = args[i+1:]
			cutArgs = args[:i]
			break
		}
	}

	if err := fs.Parse(cutArgs); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	// Shell var takes precedence over flag.
	if shellExp := os.Getenv("EXPERIMENT"); shellExp != "" {
		experiment = shellExp
	}

	// The first two positional words are the optional profile and filter.
	positional := fs.Args()
	var profile, runFilter string
	switch len(positional) {
	case 0:
		// nothing
	case 1:
		profile = positional[0]
	default:
		profile = positional[0]
		runFilter = positional[1]
		if len(positional) > 2 {
			return fmt.Errorf("unexpected arguments: %s\n       (use -- to pass raw flags to go test)", strings.Join(positional[2:], " "))
		}
	}

	// Shell var takes precedence over positional arg.
	if shellEnv := os.Getenv("MEMORY_TEST_ENV"); shellEnv != "" {
		profile = shellEnv
	}

	// ── Load .env / .env.<profile> ────────────────────────────────────────
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	// snapshot which keys were already in the environment before loading files
	// so we can report what was loaded without re-implementing LoadDotEnvFrom.
	runlog.LoadDotEnvFrom(wd)

	// Ensure MEMORY_TEST_ENV is exported so the Go test framework picks it up.
	if profile != "" {
		os.Setenv("MEMORY_TEST_ENV", profile)
	}

	// Ensure EXPERIMENT is exported so RunLog.NewRunLog picks it up in tests.
	if experiment != "" {
		os.Setenv("EXPERIMENT", experiment)
	}

	// ── Register with daemon (fail-open) ──────────────────────────────────
	registerRunWithDaemon(profile)

	// ── Build go test flags ────────────────────────────────────────────────
	goFlags := []string{"test", "-timeout", "10m"}
	// Determine which package(s) to test.
	// When a filter is set, find the package containing the test to avoid
	// "[no tests to run]" noise from unrelated packages.
	testPkgs := findTestPackages(wd, runFilter)
	if runFilter != "" {
		// Single-test mode: verbose + skip cache so output is always shown.
		goFlags = append(goFlags, "-v", "-count=1", "-run", runFilter)
	}
	goFlags = append(goFlags, extraFlags...)
	goFlags = append(goFlags, testPkgs...)

	// ── Print summary ─────────────────────────────────────────────────────
	fmt.Println("=== runlog test runner ===")
	if profile != "" {
		fmt.Printf("  env:    %s\n", profile)
		overlay := filepath.Join(wd, ".env."+profile)
		if _, err := os.Stat(overlay); err == nil {
			fmt.Printf("  overlay: %s\n", overlay)
		}
	} else {
		fmt.Println("  env:    <base .env>")
	}
	if server := os.Getenv("MEMORY_TEST_SERVER"); server != "" {
		fmt.Printf("  server: %s\n", server)
	}
	if auth := os.Getenv("MEMORY_AUTH_MODE"); auth != "" {
		fmt.Printf("  auth:   %s\n", auth)
	}
	// Provider + model info — show whichever is configured.
	if key := os.Getenv("DEEPSEEK_API_KEY"); key != "" {
		model := os.Getenv("DEEPSEEK_MODEL")
		if model == "" {
			model = "(auto)"
		}
		fmt.Printf("  provider: deepseek  model: %s\n", model)
	} else if key := os.Getenv("GOOGLE_AI_API_KEY"); key != "" {
		model := os.Getenv("GOOGLE_AI_MODEL")
		if model == "" {
			model = "(auto)"
		}
		fmt.Printf("  provider: google    model: %s\n", model)
	} else if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		model := os.Getenv("OPENAI_MODEL")
		if model == "" {
			model = "(auto)"
		}
		fmt.Printf("  provider: openai    model: %s\n", model)
	} else {
		fmt.Println("  provider: (none configured)")
	}
	if runFilter != "" {
		fmt.Printf("  filter: %s\n", runFilter)
	}
	if experiment != "" {
		fmt.Printf("  experiment: %s\n", experiment)
	}
	fmt.Println()

	// ── Exec go test (replaces the current process) ───────────────────────
	goPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go binary not found on PATH: %w", err)
	}

	// syscall.Exec replaces the process so the exit code flows naturally
	// to the caller (CI, shell, etc.) without an extra wrapper.
	return syscall.Exec(goPath, append([]string{"go"}, goFlags...), os.Environ())
}

// findTestPackages returns the Go package patterns to pass to go test.
// When filter is empty, returns ["./tests/..."] to run all test packages
// while excluding non-test packages (cmd/, framework/, fixtures/).
// When filter is set, scans ./tests/ subdirectories for a file containing
// "func <filter>(" and returns only the matching package; falls back to
// "./tests/..." if not found.
func findTestPackages(wd, filter string) []string {
	testsDir := filepath.Join(wd, "tests")
	if _, err := os.Stat(testsDir); err != nil {
		// No tests/ directory — fall back to ./...
		return []string{"./..."}
	}
	if filter == "" {
		return []string{"./tests/..."}
	}
	// Search for "func <filter>(" in test files under tests/.
	entries, err := os.ReadDir(testsDir)
	if err != nil {
		return []string{"./tests/..."}
	}
	needle := "func " + filter + "("
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pkgDir := filepath.Join(testsDir, e.Name())
		files, err := os.ReadDir(pkgDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(pkgDir, f.Name()))
			if err != nil {
				continue
			}
			if bytes.Contains(data, []byte(needle)) {
				return []string{"./" + filepath.Join("tests", e.Name())}
			}
		}
	}
	// Not found — run all test packages.
	return []string{"./tests/..."}
}

// registerRunWithDaemon attempts to register the current run with the local
// daemon. On success, RUNLOG_RUN_ID and RUNLOG_DAEMON_URL are set in the
// process environment so the exec'd go test process inherits them.
// Any error is silently ignored (fail-open).
func registerRunWithDaemon(profile string) {
	dURL := daemonURL()

	// Quick reachability check with a short timeout
	client := &http.Client{Timeout: 500 * time.Millisecond}
	healthResp, err := client.Get(dURL + "/health")
	if err != nil {
		return // daemon not running — proceed normally
	}
	_ = healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		return
	}

	body, _ := json.Marshal(map[string]any{
		"pid":         os.Getpid(),
		"env_profile": profile,
		"server_url":  os.Getenv("MEMORY_TEST_SERVER"),
		"token":       os.Getenv("MEMORY_TEST_TOKEN"),
	})

	resp, err := client.Post(dURL+"/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return
	}

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || result.ID == "" {
		return
	}

	// Inject into current process env so syscall.Exec inherits them
	os.Setenv("RUNLOG_RUN_ID", result.ID)
	os.Setenv("RUNLOG_DAEMON_URL", dURL)
}

// testFuncRe matches Go test function declarations: func TestXxx(t *testing.T) or (tb testing.TB).
var testFuncRe = regexp.MustCompile(`func\s+(Test\w+)\s*\(\s*t\s*\*?testing\.(T|TB)\s*\)`)

// DiscoverTestFunctions scans tests/*/ directories for Go test functions and
// returns them grouped by category (subdirectory name). Returns an empty map
// on error. Only functions matching the TestXxx pattern are returned.
func DiscoverTestFunctions(wd string) map[string][]string {
	result := make(map[string][]string)

	// Scan tests/*/ directories (conventional test locations).
	testsDir := filepath.Join(wd, "tests")
	if entries, err := os.ReadDir(testsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			cat := e.Name()
			pkgDir := filepath.Join(testsDir, cat)
			scanTestFiles(pkgDir, cat, result)
		}
	}

	// Scan project root for *test.go files.
	scanTestFiles(wd, "root", result)

	// Scan cmd/runlog/ for CLI/web handler tests.
	scanTestFiles(filepath.Join(wd, "cmd", "runlog"), "cli", result)

	return result
}

// scanTestFiles reads *_test.go files in dir and adds matching TestXxx functions
// to result under the given category key.
func scanTestFiles(dir, category string, result map[string][]string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		matches := testFuncRe.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			result[category] = append(result[category], m[1])
		}
	}
}
