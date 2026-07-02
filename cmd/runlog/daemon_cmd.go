// cmd/runlog/daemon_cmd.go — "runlog daemon" and "runlog cleanup" subcommands.
//
// runlog daemon          — start the local daemon (default)
// runlog daemon stop     — stop the running daemon
// runlog daemon status   — show daemon status
// runlog cleanup         — trigger an immediate orphan sweep
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	runlog "github.com/emergent-company/runlog"
)

// ─────────────────────────────────────────────────────────────────────────────
// cmdDaemon — "runlog daemon [stop|status]"
// ─────────────────────────────────────────────────────────────────────────────

func cmdDaemon(args []string, dbPath string) error {
	subCmd := "start"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		subCmd = args[0]
		args = args[1:]
	}

	switch subCmd {
	case "start", "":
		return cmdDaemonStart(args, dbPath)
	case "stop":
		return cmdDaemonStop(args)
	case "status":
		return cmdDaemonStatus(args)
	default:
		return fmt.Errorf("unknown daemon subcommand %q (use start, stop, status)", subCmd)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// daemon start
// ─────────────────────────────────────────────────────────────────────────────

func cmdDaemonStart(args []string, dbPath string) error {
	cfg, _ := runlog.LoadConfig("")
	port := cfg.DaemonPortOrDefault()

	// Optional --port and --dev flags override config defaults.
	fs := flag.NewFlagSet("daemon-start", flag.ContinueOnError)
	startPort := fs.Int("port", port, "daemon HTTP port")
	devMode := fs.Bool("dev", false, "disable binary-watch self-restart (for air/dev)")
	// Discard parse errors; unrecognised args are ignored for backward compat.
	_ = fs.Parse(args)
	if *startPort > 0 {
		port = *startPort
	}

	// Check if already running
	if isRunning(port) {
		pid := readPidFile()
		fmt.Printf("daemon already running (pid %d, port %d)\n", pid, port)
		return nil
	}

	// Re-exec self with --daemon flag in a new session
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("eval symlinks: %w", err)
	}

	daemonArgs := []string{"--daemon"}
	if dbPath != "" {
		daemonArgs = append(daemonArgs, "--db", dbPath)
	}
	daemonArgs = append(daemonArgs, fmt.Sprintf("--port=%d", port))
	if *devMode {
		daemonArgs = append(daemonArgs, "--dev")
	}

	cmd := exec.Command(self, daemonArgs...)
	setSysProcAttr(cmd) // Setsid: true — new session, survives terminal close
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn daemon: %w", err)
	}

	// Wait up to 2s for /health to respond
	daemonURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if isRunning(port) {
			pid := readPidFile()
			fmt.Printf("daemon started (pid %d, port %d)\n", pid, port)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	_ = daemonURL
	return fmt.Errorf("daemon did not start within 2 seconds (port %d)", port)
}

// ─────────────────────────────────────────────────────────────────────────────
// daemon stop
// ─────────────────────────────────────────────────────────────────────────────

func cmdDaemonStop(args []string) error {
	_ = args
	pidFile := runlog.DaemonPidFile()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Println("daemon not running")
		return nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		_ = os.Remove(pidFile)
		fmt.Println("daemon not running (stale pid file removed)")
		return nil
	}
	if !processAlive(pid) {
		_ = os.Remove(pidFile)
		fmt.Println("daemon not running (stale pid file removed)")
		return nil
	}

	if err := sigterm(pid); err != nil {
		return fmt.Errorf("send SIGTERM to pid %d: %w", pid, err)
	}

	// Wait up to 5s for process to exit
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			_ = os.Remove(pidFile)
			fmt.Printf("daemon stopped (pid %d)\n", pid)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon pid %d did not exit within 5 seconds", pid)
}

// ─────────────────────────────────────────────────────────────────────────────
// daemon status
// ─────────────────────────────────────────────────────────────────────────────

func cmdDaemonStatus(args []string) error {
	_ = args
	cfg, _ := runlog.LoadConfig("")
	port := cfg.DaemonPortOrDefault()

	if !isRunning(port) {
		fmt.Println("status: stopped")
		return nil
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/status", port)
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		fmt.Println("status: stopped (no response)")
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var info map[string]any
	if err := json.Unmarshal(body, &info); err != nil {
		fmt.Printf("status: running\n%s\n", string(body))
		return nil
	}

	pid := int64(0)
	if v, ok := info["pid"].(float64); ok {
		pid = int64(v)
	}
	uptime := int64(0)
	if v, ok := info["uptime_s"].(float64); ok {
		uptime = int64(v)
	}
	activeRuns := int64(0)
	if v, ok := info["active_runs"].(float64); ok {
		activeRuns = int64(v)
	}
	trackedRes := int64(0)
	if v, ok := info["tracked_resources"].(float64); ok {
		trackedRes = int64(v)
	}

	fmt.Printf("status:            running\n")
	fmt.Printf("pid:               %d\n", pid)
	fmt.Printf("port:              %d\n", port)
	fmt.Printf("uptime:            %s\n", time.Duration(uptime)*time.Second)
	fmt.Printf("active_runs:       %d\n", activeRuns)
	fmt.Printf("tracked_resources: %d\n", trackedRes)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// cmdCleanup — "runlog cleanup"
// ─────────────────────────────────────────────────────────────────────────────

func cmdCleanup(args []string) error {
	fs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `runlog cleanup — trigger an immediate orphan sweep on the daemon

USAGE
  runlog cleanup

The daemon must be running. This calls POST /cleanup and prints the result.
`)
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	cfg, _ := runlog.LoadConfig("")
	port := cfg.DaemonPortOrDefault()

	if !isRunning(port) {
		fmt.Println("daemon not running")
		return nil
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/cleanup", port)
	resp, err := http.Post(url, "application/json", nil) //nolint:gosec,noctx
	if err != nil {
		return fmt.Errorf("POST /cleanup: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result map[string]int
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("cleanup result: %s\n", string(body))
		return nil
	}
	fmt.Printf("cleanup: deleted=%d failed=%d\n", result["deleted"], result["failed"])
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// runDaemonInternal — internal mode: write PID file, start HTTP server
// ─────────────────────────────────────────────────────────────────────────────

func runDaemonInternal(args []string) error {
	fs := flag.NewFlagSet("daemon-internal", flag.ContinueOnError)
	dbFlag := fs.String("db", "", "path to runs.db")
	portFlag := fs.Int("port", 7430, "daemon HTTP port")
	timeoutFlag := fs.Duration("timeout", 30*time.Minute, "max run duration before auto-timeout (e.g. 30m, 1h)")
	devFlag := fs.Bool("dev", false, "disable binary-watch self-restart (for air-managed dev servers)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Determine DB path
	dbPath := *dbFlag
	if dbPath == "" {
		dbPath = resolveDBPath("")
	}

	// Open the database (creates + migrates)
	db, err := runlog.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("open db %s: %w", dbPath, err)
	}
	defer db.Close()

	// Walk up from DB directory to find .runlog.yaml config
	cfgDir := filepath.Dir(dbPath)
	for i := 0; i < 5; i++ {
		if info, err := os.Stat(filepath.Join(cfgDir, ".runlog.yaml")); err == nil && !info.IsDir() {
			break
		}
		parent := filepath.Dir(cfgDir)
		if parent == cfgDir {
			cfgDir = filepath.Dir(dbPath)
			break
		}
		cfgDir = parent
	}
	cfg, _ := runlog.LoadConfig(cfgDir)

	// Infer project root: parent of .runlog directory, or the config directory.
	workDir := cfgDir
	if filepath.Base(workDir) == ".runlog" {
		workDir = filepath.Dir(workDir)
	}
	// If workDir doesn't look like a project root (no tests/ or go.mod), fall
	// back to the current working directory so test discovery still works.
	if _, err := os.Stat(filepath.Join(workDir, "go.mod")); os.IsNotExist(err) {
		if wd, err := os.Getwd(); err == nil {
			workDir = wd
		}
	}

	port := *portFlag
	if port <= 0 {
		port = cfg.DaemonPortOrDefault()
	}

	// Write PID file
	pidFile := runlog.DaemonPidFile()
	if err := os.MkdirAll(filepath.Dir(pidFile), 0o755); err != nil {
		return fmt.Errorf("mkdir for pid file: %w", err)
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer os.Remove(pidFile) //nolint:errcheck

	// Handle SIGTERM for clean shutdown
	trapSigterm(func() {
		log.Println("daemon: received SIGTERM, shutting down")
		os.Remove(pidFile) //nolint:errcheck
		os.Exit(0)
	})

	// Kill any existing process on the target port before starting.
	killProcessOnPort(port)

	// Resolve artifacts directory: <workDir>/.runlog/artifacts/
	artifactsDir := filepath.Join(workDir, ".runlog", "artifacts")

	srv := newDaemonServer(db, port, *timeoutFlag, artifactsDir, *devFlag)
	srv.pidFile = pidFile

	// Mount web UI under /ui/
	webApp := newWebApp(db, cfg, workDir)
	srv.mux.Handle("/ui/", http.StripPrefix("/ui", webApp))

	return srv.Start()
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// killProcessOnPort kills any process listening on the given TCP port.
func killProcessOnPort(port int) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		ln.Close()
		return
	}
	// Port is in use — find and kill the owning process.
	pid, _ := findPIDOnPort(port)
	if pid > 0 {
		p, err := os.FindProcess(pid)
		if err == nil {
			_ = p.Signal(os.Interrupt)
			done := make(chan struct{}, 1)
			go func() {
				time.Sleep(3 * time.Second)
				done <- struct{}{}
			}()
			select {
			case <-done:
				_ = p.Kill()
			case <-time.After(1 * time.Second):
				_ = p.Kill()
			}
			_, _ = p.Wait()
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// findPIDOnPort returns the PID of the process listening on the given port.
func findPIDOnPort(port int) (int, error) {
	output, err := exec.Command("fuser", fmt.Sprintf("%d/tcp", port)).Output()
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// isRunning checks if the daemon is reachable on the given port.
func isRunning(port int) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// readPidFile reads the PID from the daemon PID file. Returns 0 on error.
func readPidFile() int {
	data, err := os.ReadFile(runlog.DaemonPidFile())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// daemonURL returns the URL to the local daemon. It respects the
// RUNLOG_DAEMON_URL env var, falling back to http://localhost:<port>.
func daemonURL() string {
	if u := os.Getenv("RUNLOG_DAEMON_URL"); u != "" {
		return u
	}
	cfg, err := runlog.LoadConfig("")
	if err != nil || cfg == nil {
		return "http://127.0.0.1:7430"
	}
	return fmt.Sprintf("http://127.0.0.1:%d", cfg.DaemonPortOrDefault())
}
