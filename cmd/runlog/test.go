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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

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
	var envName string
	fs.StringVar(&experiment, "experiment", "", "tag all runs in this batch with an experiment name for later comparison")
	fs.StringVar(&experiment, "e", "", "shorthand for --experiment")
	fs.StringVar(&envName, "env", "", "validate named environment before running")
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

	// Set MEMORY_TEST_ENV before LoadDotEnvFrom so the profile overlay
	// (.env.<profile>) is picked up during env loading.
	if profile != "" {
		os.Setenv("MEMORY_TEST_ENV", profile)
	}

	// snapshot which keys were already in the environment before loading files
	// so we can report what was loaded without re-implementing LoadDotEnvFrom.
	runlog.LoadDotEnvFrom(wd)

	// Ensure EXPERIMENT is exported so RunLog.NewRunLog picks it up in tests.
	if experiment != "" {
		os.Setenv("EXPERIMENT", experiment)
	}

	// Discover org ID now so all child test binaries inherit it.
	// Avoids concurrent server requests from parallel go test packages.
	if org := runlog.DiscoverOrgIDForce(); org != "" {
		os.Setenv("MEMORY_ORG_ID", org)
	}

	// ── Build go test flags ────────────────────────────────────────────────
	goFlags := []string{"test", "-count=1", "-timeout", "10m"}
	// Determine which package(s) to test.
	// When a filter is set, find the package containing the test to avoid
	// "[no tests to run]" noise from unrelated packages.
	testPkgs := findTestPackages(wd, runFilter)
	if runFilter != "" {
		// Single-test mode: verbose so output is always shown.
		goFlags = append(goFlags, "-v", "-run", runFilter)
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

	// ── Load env vars from --env config ──────────────────────────────────
	if envName != "" {
		cfg, _ := runlog.LoadConfig(filepath.Dir(resolveDBPath("")))
		env := cfg.LookupEnvironment(envName)
		if env == nil {
			return fmt.Errorf("environment %q not found (available: %s)", envName, envNames(cfg))
		}
		// Set env vars from the environment config into the process env
		// so both validation and the exec'd go test process can use them.
		for k, v := range env.Env {
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
		// Use env name as profile if none set yet
		if profile == "" {
			profile = envName
			os.Setenv("MEMORY_TEST_ENV", envName)
		}
		if err := runlog.ValidateEnvSummary(env); err != nil {
			return err
		}
	}

	// ── Run go test with captured output ────────────────────────────────────
	goPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go binary not found on PATH: %w", err)
	}

	cmd := exec.Command(goPath, goFlags...)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("go test: %w", err)
	}
	return nil
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

func envNames(cfg *runlog.Config) string {
	names := make([]string, len(cfg.Environments))
	for i, e := range cfg.Environments {
		names[i] = e.Name
	}
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
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
