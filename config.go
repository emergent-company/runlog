package runlog

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds optional configuration loaded from a .runlog/config.yaml file.
// All fields are optional — runlog works without any config file.
type Config struct {
	// Categories maps category names to lists of test function names.
	// Tests not listed in any category are grouped under "Uncategorized".
	// Example: {"cli/install": ["TestCLIInstalled_Version", "TestCLIInstalled_Help"]}
	Categories map[string][]string `yaml:"categories"`

	// TestCommand is the command template used to launch tests from the TUI.
	// Use {name} as a placeholder for the test function name.
	// Deprecated: use WorkDir + Env + TestPackages instead.
	// Default: "go test -v -run {name} ./..."
	TestCommand string `yaml:"testCommand"`

	// DBPath is an explicit path to the runs.db SQLite database.
	// If set, overrides the default search path resolution.
	DBPath string `yaml:"db"`

	// DaemonPort is the port the local runlog daemon listens on.
	// Default: 7430.
	DaemonPort int `yaml:"daemon_port"`

	// WorkDir is the working directory for test execution.
	// If set, tests run with this as their working directory (instead of cwd).
	WorkDir string `yaml:"work_dir"`

	// Env maps environment variable names to values that are set for every test run.
	// Example: {"MEMORY_TEST_SERVER": "http://localhost:3002"}
	Env map[string]string `yaml:"env"`

	// TestPackages lists the Go packages to test. Default: ["./..."].
	// Example: ["./tests/api/...", "./tests/integration/..."]
	TestPackages []string `yaml:"test_packages"`
}

// LoadConfig searches for a config.yaml configuration file and returns the
// parsed Config. Search order:
//  1. $RUNLOG_CONFIG environment variable (exact path)
//  2. .runlog/config.yaml (in the resolved runlog directory)
//  3. The directory containing runs.db (dbDir), if provided
//  4. .runlog.yaml in the current working directory (backward compatibility)
//
// Returns an empty Config (not an error) if no config file is found.
// Only returns an error if a file is found but cannot be parsed.
func LoadConfig(dbDir string) (*Config, error) {
	var searchPaths []string

	// 1. $RUNLOG_CONFIG env var
	if p := os.Getenv("RUNLOG_CONFIG"); p != "" {
		searchPaths = append(searchPaths, p)
	}

	// 2. Project .runlog dir
	searchPaths = append(searchPaths, filepath.Join(RunlogDir(), "config.yaml"))

	// 3. DB directory
	if dbDir != "" {
		searchPaths = append(searchPaths, filepath.Join(dbDir, "config.yaml"))
		searchPaths = append(searchPaths, filepath.Join(dbDir, ".runlog.yaml"))
	}

	// 4. Current working directory
	if wd, err := os.Getwd(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(wd, ".runlog.yaml"))
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return parseConfigFile(path)
		}
	}

	// No config file found — return empty config
	return &Config{}, nil
}

// parseConfigFile reads and parses a configuration file.
// We use a simple hand-rolled parser to avoid adding a YAML dependency.
func parseConfigFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()

	cfg := &Config{
		Categories: make(map[string][]string),
		Env:        make(map[string]string),
	}

	scanner := bufio.NewScanner(f)
	var currentSection string // "", "categories", "env"
	var currentCategory string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Top-level keys
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if strings.HasPrefix(trimmed, "testCommand:") {
				cfg.TestCommand = strings.TrimSpace(strings.TrimPrefix(trimmed, "testCommand:"))
				cfg.TestCommand = strings.Trim(cfg.TestCommand, "\"'")
				currentSection = ""
				continue
			}
			if strings.HasPrefix(trimmed, "db:") {
				cfg.DBPath = strings.TrimSpace(strings.TrimPrefix(trimmed, "db:"))
				cfg.DBPath = strings.Trim(cfg.DBPath, "\"'")
				currentSection = ""
				continue
			}
			if strings.HasPrefix(trimmed, "daemon_port:") {
				raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "daemon_port:"))
				raw = strings.Trim(raw, "\"'")
				if p, err := strconv.Atoi(raw); err == nil {
					cfg.DaemonPort = p
				}
				currentSection = ""
				continue
			}
			if strings.HasPrefix(trimmed, "work_dir:") {
				cfg.WorkDir = strings.TrimSpace(strings.TrimPrefix(trimmed, "work_dir:"))
				cfg.WorkDir = strings.Trim(cfg.WorkDir, "\"'")
				currentSection = ""
				continue
			}
			if trimmed == "categories:" {
				currentSection = "categories"
				currentCategory = ""
				continue
			}
			if trimmed == "env:" {
				currentSection = "env"
				continue
			}
			if trimmed == "test_packages:" {
				currentSection = "test_packages"
				continue
			}
			currentSection = ""
			continue
		}

		// Inside categories section
		if currentSection == "categories" {
			// Category key line: "  cli/install:"
			if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
				currentCategory = strings.TrimSuffix(trimmed, ":")
				currentCategory = strings.Trim(currentCategory, "\"'")
				if cfg.Categories[currentCategory] == nil {
					cfg.Categories[currentCategory] = []string{}
				}
				continue
			}
			// List item: "    - TestCLIInstalled_Version"
			if strings.HasPrefix(trimmed, "- ") && currentCategory != "" {
				name := strings.TrimPrefix(trimmed, "- ")
				name = strings.Trim(name, "\"'")
				cfg.Categories[currentCategory] = append(cfg.Categories[currentCategory], name)
				continue
			}
		}

		// Inside env section: "  KEY: value"
		if currentSection == "env" {
			if idx := strings.Index(trimmed, ":"); idx > 0 {
				key := strings.TrimSpace(trimmed[:idx])
				value := strings.TrimSpace(trimmed[idx+1:])
				value = strings.Trim(value, "\"'")
				cfg.Env[key] = value
			}
			continue
		}

		// Inside test_packages section: "  - ./tests/api/..."
		if currentSection == "test_packages" {
			if strings.HasPrefix(trimmed, "- ") {
				pkg := strings.TrimPrefix(trimmed, "- ")
				pkg = strings.Trim(pkg, "\"'")
				cfg.TestPackages = append(cfg.TestPackages, pkg)
			}
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	return cfg, nil
}

// TestCommandOrDefault returns the configured test command, or the default
// "go test -v -run {name} ./..." if none is configured.
// Deprecated: use BuildTestCommand instead for structured config support.
func (c *Config) TestCommandOrDefault() string {
	if c.TestCommand != "" {
		return c.TestCommand
	}
	return c.BuildTestCommand("{name}")
}

// BuildTestCommand builds the shell command to run a test from the structured
// config fields (WorkDir, Env, TestPackages). Falls back to TestCommand if set.
// The {name} placeholder is replaced with the test function name.
func (c *Config) BuildTestCommand(name string) string {
	if c.TestCommand != "" {
		return ExpandTestCommand(c.TestCommand, name, "")
	}
	var parts []string
	if c.WorkDir != "" {
		parts = append(parts, "cd", c.WorkDir, "&&")
	}
	for k, v := range c.Env {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	parts = append(parts, "go test", "-v", "-count=1", "-run", name)
	if len(c.TestPackages) > 0 {
		parts = append(parts, c.TestPackages...)
	} else {
		parts = append(parts, "./...")
	}
	return strings.Join(parts, " ")
}

// ExpandTestCommand replaces {name} and {env} placeholders in a command template.
func ExpandTestCommand(tmpl, name, env string) string {
	s := tmpl
	buf := make([]byte, 0, len(s)*2)
	for i := 0; i < len(s); i++ {
		if s[i] == '{' {
			end := i + 1
			for end < len(s) && s[end] != '}' {
				end++
			}
			if end < len(s) {
				key := s[i+1 : end]
				switch key {
				case "name":
					buf = append(buf, name...)
				case "env":
					buf = append(buf, env...)
				default:
					buf = append(buf, s[i:end+1]...)
				}
				i = end
				continue
			}
		}
		buf = append(buf, s[i])
	}
	return string(buf)
}

// DaemonPortOrDefault returns the configured daemon port, or 7430 if not set.
func (c *Config) DaemonPortOrDefault() int {
	if c.DaemonPort > 0 {
		return c.DaemonPort
	}
	return 7430
}

// CategoryForTest returns the category for a test name, or "Uncategorized"
// if the test is not assigned to any category. Patterns support glob wildcards
// (* and ?) via filepath.Match, so "login*" matches "login_page_test" etc.
func (c *Config) CategoryForTest(testName string) string {
	for cat, tests := range c.Categories {
		for _, t := range tests {
			if t == testName {
				return cat
			}
			if matched, _ := filepath.Match(t, testName); matched {
				return cat
			}
		}
	}
	return "Uncategorized"
}
