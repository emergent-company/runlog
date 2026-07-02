// Package e2eframework — env.go
//
// Helpers for loading environment variables from .env files.
package runlog

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// LoadDotEnv reads KEY=VALUE pairs from .env (and optionally .env.<name>)
// co-located with the calling source file and sets them in the process
// environment. Lines starting with '#' and blank lines are ignored.
// Variables already set in the environment are not overwritten (the shell
// always wins).
//
// If MEMORY_TEST_ENV is set (e.g. "mcj-emergent"), the file
// .env.<MEMORY_TEST_ENV> is loaded after .env so its values take precedence
// over the base file — but shell variables still win over both.
//
// Example named env files:
//
//	.env.mcj-emergent   — account-mode auth against the shared test server
//	.env.localhost      — standalone auth against a local dev server
func LoadDotEnv() { //nolint:deadcode
	_, filename, _, ok := runtime.Caller(1) // caller's source file
	if !ok {
		return
	}
	dir := filepath.Dir(filename)

	// Check if tests are being run via raw 'go test' instead of 'runlog test'
	checkRawGoTest()

	loadDotEnvDir(dir)
}

// checkRawGoTest detects when tests are being run via raw 'go test' and provides
// a helpful error message with instructions to use 'runlog test' instead.
func checkRawGoTest() { //nolint:deadcode
	// Only act when the process is 'go test' itself, not 'runlog test'.
	exe := filepath.Base(os.Args[0])
	if exe == "runlog" || exe == "runlog.test" || strings.HasPrefix(exe, "runlog-air") {
		return
	}
	// If MEMORY_TEST_ENV is not set AND we're in the e2e tests, this is likely
	// a raw 'go test' invocation. The runlog wrapper always sets MEMORY_TEST_ENV
	// (even if empty for the base .env).
	if os.Getenv("MEMORY_TEST_ENV") == "" && os.Getenv("TEST_RUNNER") == "" {
		// Only flag if we can detect we're in the e2e repository
		if wd, err := os.Getwd(); err == nil && strings.Contains(wd, "emergent.memory.e2e") {
			printRawGoTestWarning()
			os.Exit(1)
		}
	}
}

func printRawGoTestWarning() { //nolint:deadcode
	msg := `
╔════════════════════════════════════════════════════════════════════════════╗
║                                                                            ║
║  ⚠️  Tests should be run via 'runlog test', not 'go test'                 ║
║                                                                            ║
║  You are running tests with raw 'go test'. This means:                    ║
║    • Test runs won't be recorded in the runlog database                   ║
║    • Environment information won't be tracked                             ║
║    • Test results won't be visible in 'runlog runs' or 'runlog inspect'   ║
║                                                                            ║
║  ✅ Use 'runlog test' instead:                                            ║
║                                                                            ║
║    runlog test                                    # all tests              ║
║    runlog test mcj-emergent                       # with env overlay      ║
║    runlog test localhost TestCLIInstalled_Version # specific test         ║
║    runlog test mcj-emergent -- -v                 # with extra go flags    ║
║                                                                            ║
║  📖 See README.md for more details on running tests locally               ║
║                                                                            ║
╚════════════════════════════════════════════════════════════════════════════╝
`
	// Print to stderr so it's visible even if tests fail
	os.Stderr.WriteString(msg)
}

// LoadDotEnvFrom loads .env (and optionally .env.<MEMORY_TEST_ENV>) from the
// given directory.  Use this instead of LoadDotEnv when the caller is a
// compiled binary (cmd/*) whose runtime.Caller path won't resolve to the
// directory that actually contains the .env files.
func LoadDotEnvFrom(dir string) {
	loadDotEnvDir(dir)
}

// loadDotEnvDir is the shared implementation for LoadDotEnv / LoadDotEnvFrom.
func loadDotEnvDir(dir string) {
	// Load base .env first.
	loadEnvFile(filepath.Join(dir, ".env"), false)

	// If MEMORY_TEST_ENV names a profile, load the overlay.
	// The overlay is allowed to overwrite values from the base file (but
	// shell variables still win over both).
	if profile := os.Getenv("MEMORY_TEST_ENV"); profile != "" {
		loadEnvFile(filepath.Join(dir, ".env."+profile), true)
	}
}

// LoadEnvFile reads KEY=VALUE pairs from path into the process environment.
// If overwrite is true, existing env vars are replaced (used for named
// overlays); if false, existing vars are preserved (shell wins).
// Missing file is silently skipped.
func LoadEnvFile(path string, overwrite bool) {
	loadEnvFile(path, overwrite)
}

// loadEnvFile reads KEY=VALUE pairs from path into the process environment.
// If overwrite is true, existing env vars are replaced (used for named
// overlays); if false, existing vars are preserved (shell wins).
func loadEnvFile(path string, overwrite bool) {
	f, err := os.Open(path)
	if err != nil {
		return // file is optional
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		// Strip surrounding quotes if present.
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		if k == "" {
			continue
		}
		if overwrite || os.Getenv(k) == "" {
			os.Setenv(k, v) //nolint:errcheck
		}
	}
}

// BlueprintEnvVar looks up key by reading dir/.env then dir/.env.local (later
// file wins), then falls back to os.Getenv(key).  dir should be the local
// path to the blueprint directory (e.g. "/root/ai-news-memory-blueprint").
func BlueprintEnvVar(dir, key string) string { //nolint:deadcode
	vars := ParseBlueprintEnvFiles(dir)
	if v, ok := vars[key]; ok && v != "" {
		return v
	}
	return os.Getenv(key)
}

// ─────────────────────────────────────────────────────────────────────────────
// Environment validation
// ─────────────────────────────────────────────────────────────────────────────

// EnvResult is the outcome of validating a single requirement.
type EnvResult struct {
	Key   string `json:"key"`
	Pass  bool   `json:"pass"`
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
	Hint  string `json:"hint,omitempty"`
}

// ValidateEnv runs all requirement checks for an environment config.
// Returns nil if all pass, or a composite error listing all failures.
func ValidateEnv(env *EnvironmentConfig) []EnvResult {
	var results []EnvResult
	for key, check := range env.Requires {
		val := os.Getenv(key)
		_ = os.WriteFile("/tmp/env_debug.log", []byte("ValidateEnv: key="+key+" getenv="+os.Getenv(key)+" envVal="+env.Env[key]+" default="+check.Default+"\n"), 0644)
		if val == "" {
			val = env.Env[key]
		}
		if val == "" {
			val = check.Default
		}
		r := EnvResult{Key: key, Value: val, Hint: check.Hint}
		err := CheckRequirement(key, val, check)
		if err != nil {
			r.Pass = false
			r.Error = err.Error()
		} else {
			r.Pass = true
		}
		results = append(results, r)
	}
	return results
}

// ValidateEnvSummary returns a quick pass/fail for the whole environment.
func ValidateEnvSummary(env *EnvironmentConfig) error {
	results := ValidateEnv(env)
	var failures []string
	for _, r := range results {
		if !r.Pass {
			msg := r.Key + ": " + r.Error
			if r.Hint != "" {
				msg += " (" + r.Hint + ")"
			}
			failures = append(failures, msg)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("environment %q validation failed:\n  - %s", env.Name, strings.Join(failures, "\n  - "))
	}
	return nil
}

// CheckRequirement validates a single env var against an EnvCheck.
func CheckRequirement(key, val string, check EnvCheck) error {
	switch check.Check {
	case "nonempty":
		return checkNonempty(key, val)
	case "reachable":
		return checkReachable(key, val, check)
	case "port_open":
		return checkPortOpen(key, val, check)
	case "executable":
		return checkExecutable(val)
	case "file_exists":
		return checkFileExists(val)
	case "":
		return nil // no check configured
	default:
		return fmt.Errorf("unknown check type %q", check.Check)
	}
}

func checkNonempty(key, val string) error {
	if val == "" {
		return fmt.Errorf("%s is not set", key)
	}
	return nil
}

func checkReachable(key, val string, check EnvCheck) error {
	url := val
	if check.URL != "" {
		url = check.URL
	}
	if url == "" {
		return fmt.Errorf("no URL to check for %s", key)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("%s unreachable at %s: %w", key, url, err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s at %s returned HTTP %d", key, url, resp.StatusCode)
	}
	return nil
}

func checkPortOpen(key, val string, check EnvCheck) error {
	addr := val
	if addr == "" {
		addr = check.Default
	}
	if addr == "" {
		return fmt.Errorf("no address to dial for %s", key)
	}
	// Support "host:port" or just "port"
	if !strings.Contains(addr, ":") {
		host := os.Getenv(strings.TrimSuffix(key, "_PORT") + "_HOST")
		if host == "" {
			host = "localhost"
		}
		addr = host + ":" + addr
	}
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return fmt.Errorf("%s not reachable at %s: %w", key, addr, err)
	}
	conn.Close()
	return nil
}

func checkExecutable(val string) error {
	if val == "" {
		return fmt.Errorf("no executable path specified")
	}
	_, err := exec.LookPath(val)
	if err != nil {
		return fmt.Errorf("executable %q not found on PATH: %w", val, err)
	}
	return nil
}

func checkFileExists(val string) error {
	if val == "" {
		return fmt.Errorf("no file path specified")
	}
	_, err := os.Stat(val)
	if err != nil {
		return fmt.Errorf("file %q not found: %w", val, err)
	}
	return nil
}

// ParseBlueprintEnvFiles reads .env then .env.local from dir and returns the
// merged key-value map.  Later files overwrite earlier ones.
func ParseBlueprintEnvFiles(dir string) map[string]string { //nolint:deadcode
	m := make(map[string]string)
	for _, name := range []string{".env", ".env.local"} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "export ")
			eq := strings.IndexByte(line, '=')
			if eq < 0 {
				continue
			}
			k := strings.TrimSpace(line[:eq])
			v := strings.TrimSpace(line[eq+1:])
			if k == "" {
				continue
			}
			// Strip quotes and inline comments.
			if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
				v = v[1 : len(v)-1]
			} else if idx := strings.Index(v, " #"); idx >= 0 {
				v = strings.TrimSpace(v[:idx])
			}
			m[k] = v
		}
	}
	return m
}
