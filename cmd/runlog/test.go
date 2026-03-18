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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

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
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `runlog test — load .env and run go test

USAGE
  runlog test [<profile>] [<filter>] [-- <extra go test flags>]

ARGUMENTS
  <profile>   optional env profile name; loads .env.<profile> as an overlay
              (same as setting MEMORY_TEST_ENV=<profile> in your shell)
  <filter>    optional go test -run filter, e.g. TestMyFeature

FLAGS
  -- <flags>  all arguments after -- are forwarded verbatim to go test

EXAMPLES
  runlog test                              # all tests using .env defaults
  runlog test mcj-emergent                 # overlay .env.mcj-emergent
  runlog test localhost TestCLI_Version    # named env + single test filter
  runlog test -- -count=1 -timeout 5m     # pass raw go test flags
  runlog test mcj-emergent -- -v -count=2 # named env + extra flags
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

	// ── Build go test flags ────────────────────────────────────────────────
	goFlags := []string{"test", "-v", "-timeout", "10m"}
	if runFilter != "" {
		goFlags = append(goFlags, "-run", runFilter)
	}
	goFlags = append(goFlags, extraFlags...)
	goFlags = append(goFlags, "./...")

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
	if runFilter != "" {
		fmt.Printf("  filter: %s\n", runFilter)
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
