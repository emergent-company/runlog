package runlog

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LinterDef defines a single linter that can be run from the UI.
type LinterDef struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
}

// ProjectConfig defines a named sub-project within a workspace.
// Each project has its own work_dir; linters and test_packages are
// discovered from its own lefthook.yml or .runlog/config.yaml.
type ProjectConfig struct {
	Name    string `yaml:"name"`
	WorkDir string `yaml:"work_dir"`
}

// EnvCheck defines a single requirement check for an environment variable.
// Check must be one of: nonempty, reachable, port_open, executable, file_exists.
type EnvCheck struct {
	Check   string `yaml:"check"`   // validation type
	Default string `yaml:"default"` // fallback value if env var unset
	URL     string `yaml:"url"`     // for reachable: override URL
	Hint    string `yaml:"hint"`    // human-readable fix message
}

// EnvironmentConfig defines a named test environment with its env vars
// and requirement checks.
type EnvironmentConfig struct {
	Name     string              `yaml:"name"`
	Env      map[string]string   `yaml:"env"`
	Requires map[string]EnvCheck `yaml:"requires"`
	// TestScript is an optional shell script that validates the environment
	// is ready. Run via: runlog env test <name>
	TestScript string `yaml:"test_script"`
	// SetupScript is an optional shell script that prepares the environment
	// (e.g. create projects, seed data). Run via: runlog env setup <name>
	SetupScript string `yaml:"setup_script"`
}

// knownTopLevelKeys lists all accepted top-level keys in .runlog/config.yaml.
// Unknown keys cause a parse error to prevent silent misconfiguration.
var knownTopLevelKeys = map[string]bool{
	"name":          true,
	"testCommand":   true,
	"db":            true,
	"daemon_port":   true,
	"work_dir":      true,
	"env":           true,
	"test_packages": true,
	"linters":       true,
	"projects":      true,
	"environments":  true,
	"categories":    true,
}

// Config holds optional configuration loaded from a .runlog/config.yaml file.
// All fields are optional — runlog works without any config file.
type Config struct {
	// Name is the human-readable name shown in the UI page title.
	// Falls back to RUNLOG_APP_TITLE env var, then "runlog".
	Name string `yaml:"name"`

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

	// Linters lists the linters available in the UI.
	// If empty, runlog will attempt to discover linters from lefthook.yml.
	Linters []LinterDef `yaml:"linters"`

	// Projects lists sub-projects in a multi-project workspace.
	// Each project has its own work_dir; linters and test_packages are
	// discovered from each project's own lefthook.yml or .runlog/config.yaml.
	Projects []ProjectConfig `yaml:"projects"`

	// Environments lists named test environments with env vars and
	// requirement checks for pre-validation.
	Environments []EnvironmentConfig `yaml:"environments"`

	// Categories maps category names to lists of test function names.
	// Used by the TUI to group tests. Tests not listed appear as
	// "Uncategorized". Example:
	//
	//	categories:
	//	  cli/projects:
	//	    - TestCLIInstalled_ProjectCreateGetDelete
	//	    - TestCLIInstalled_ProjectsList
	Categories map[string][]string `yaml:"categories"`
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
		Env: make(map[string]string),
	}

	scanner := bufio.NewScanner(f)
	var currentSection string
	var currentProject *ProjectConfig
	var currentEnv *EnvironmentConfig
	var envSubSection string
	var currentEnvKey string
	var currentEnvCheck *EnvCheck

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Inside a section: indented content
		if currentSection != "" && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			switch currentSection {
			case "env":
				if idx := strings.Index(trimmed, ":"); idx > 0 {
					key := strings.TrimSpace(trimmed[:idx])
					value := strings.TrimSpace(trimmed[idx+1:])
					value = strings.Trim(value, "\"'")
					cfg.Env[key] = value
				}
			case "test_packages":
				if strings.HasPrefix(trimmed, "- ") {
					pkg := strings.TrimPrefix(trimmed, "- ")
					pkg = strings.Trim(pkg, "\"'")
					cfg.TestPackages = append(cfg.TestPackages, pkg)
				}
			case "linters":
				// Each linter is a dash-prefixed block with name: and command:
				if strings.HasPrefix(trimmed, "- ") {
					// Start of a new linter block; store it temporarily
					lineWithoutDash := strings.TrimPrefix(trimmed, "- ")
					if idx := strings.Index(lineWithoutDash, ":"); idx > 0 {
						key := strings.TrimSpace(lineWithoutDash[:idx])
						val := strings.TrimSpace(lineWithoutDash[idx+1:])
						val = strings.Trim(val, "\"'")
						if key == "name" {
							cfg.Linters = append(cfg.Linters, LinterDef{Name: val})
						}
					}
				} else if strings.HasPrefix(trimmed, "name:") || strings.HasPrefix(trimmed, "command:") {
					// Continuation of the current linter block
					if idx := strings.Index(trimmed, ":"); idx > 0 {
						key := strings.TrimSpace(trimmed[:idx])
						val := strings.TrimSpace(trimmed[idx+1:])
						val = strings.Trim(val, "\"'")
						if len(cfg.Linters) > 0 {
							last := &cfg.Linters[len(cfg.Linters)-1]
							switch key {
							case "name":
								last.Name = val
							case "command":
								last.Command = val
							}
						}
					}
				}
			case "projects":
				if strings.HasPrefix(trimmed, "- ") {
					if currentProject != nil {
						cfg.Projects = append(cfg.Projects, *currentProject)
					}
					currentProject = &ProjectConfig{}
					afterDash := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
					if idx := strings.Index(afterDash, ":"); idx > 0 {
						key := strings.TrimSpace(afterDash[:idx])
						val := strings.TrimSpace(afterDash[idx+1:])
						val = strings.Trim(val, "\"'")
						if key == "name" {
							currentProject.Name = val
						}
					}
				} else if strings.HasPrefix(trimmed, "work_dir:") && currentProject != nil {
					val := strings.TrimSpace(strings.TrimPrefix(trimmed, "work_dir:"))
					val = strings.Trim(val, "\"'")
					currentProject.WorkDir = val
				}
			case "environments":
				if strings.HasPrefix(trimmed, "- ") {
					if currentEnv != nil {
						cfg.Environments = append(cfg.Environments, *currentEnv)
					}
					currentEnv = &EnvironmentConfig{Env: make(map[string]string), Requires: make(map[string]EnvCheck)}
					envSubSection = ""
					afterDash := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
					if idx := strings.Index(afterDash, ":"); idx > 0 {
						key := strings.TrimSpace(afterDash[:idx])
						val := strings.TrimSpace(afterDash[idx+1:])
						val = strings.Trim(val, "\"'")
						if key == "name" && val != "" {
							currentEnv.Name = val
						}
					}
				} else if trimmed == "env:" && currentEnv != nil {
					envSubSection = "env"
				} else if trimmed == "requires:" && currentEnv != nil {
					envSubSection = "requires"
					currentEnvKey = ""
					currentEnvCheck = nil
				} else if envSubSection == "env" && currentEnv != nil {
					if strings.HasPrefix(trimmed, "test_script:") {
						envSubSection = ""
						currentEnv.TestScript = strings.TrimSpace(strings.TrimPrefix(trimmed, "test_script:"))
						currentEnv.TestScript = strings.Trim(currentEnv.TestScript, "\"'")
					} else if strings.HasPrefix(trimmed, "setup_script:") {
						envSubSection = ""
						currentEnv.SetupScript = strings.TrimSpace(strings.TrimPrefix(trimmed, "setup_script:"))
						currentEnv.SetupScript = strings.Trim(currentEnv.SetupScript, "\"'")
					} else if idx := strings.Index(trimmed, ":"); idx > 0 {
						k := strings.TrimSpace(trimmed[:idx])
						v := strings.TrimSpace(trimmed[idx+1:])
						v = strings.Trim(v, "\"'")
						if k != "" {
							currentEnv.Env[k] = v
						}
					}
				} else if envSubSection == "requires" && currentEnv != nil {
					// Check for top-level env fields that exit requires subsection
					if strings.HasPrefix(trimmed, "test_script:") {
						if currentEnvCheck != nil && currentEnvKey != "" {
							currentEnv.Requires[currentEnvKey] = *currentEnvCheck
						}
						currentEnvKey = ""
						currentEnvCheck = nil
						envSubSection = ""
						currentEnv.TestScript = strings.TrimSpace(strings.TrimPrefix(trimmed, "test_script:"))
						currentEnv.TestScript = strings.Trim(currentEnv.TestScript, "\"'")
					} else if strings.HasPrefix(trimmed, "setup_script:") {
						if currentEnvCheck != nil && currentEnvKey != "" {
							currentEnv.Requires[currentEnvKey] = *currentEnvCheck
						}
						currentEnvKey = ""
						currentEnvCheck = nil
						envSubSection = ""
						currentEnv.SetupScript = strings.TrimSpace(strings.TrimPrefix(trimmed, "setup_script:"))
						currentEnv.SetupScript = strings.Trim(currentEnv.SetupScript, "\"'")
					} else if strings.HasSuffix(trimmed, ":") && !strings.ContainsAny(trimmed, " ") {
						if currentEnvCheck != nil && currentEnvKey != "" {
							currentEnv.Requires[currentEnvKey] = *currentEnvCheck
						}
						currentEnvKey = strings.TrimSuffix(trimmed, ":")
						currentEnvCheck = &EnvCheck{}
					} else if currentEnvCheck != nil && currentEnvKey != "" {
						if idx := strings.Index(trimmed, ":"); idx > 0 {
							k := strings.TrimSpace(trimmed[:idx])
							v := strings.TrimSpace(trimmed[idx+1:])
							v = strings.Trim(v, "\"'")
							switch k {
							case "check":
								currentEnvCheck.Check = v
							case "default":
								currentEnvCheck.Default = v
							case "url":
								currentEnvCheck.URL = v
							case "hint":
								currentEnvCheck.Hint = v
							}
						}
					}
				} else if envSubSection == "" && currentEnv != nil {
					// Top-level env fields: test_script, setup_script
					if strings.HasPrefix(trimmed, "test_script:") {
						currentEnv.TestScript = strings.TrimSpace(strings.TrimPrefix(trimmed, "test_script:"))
						currentEnv.TestScript = strings.Trim(currentEnv.TestScript, "\"'")
					} else if strings.HasPrefix(trimmed, "setup_script:") {
						currentEnv.SetupScript = strings.TrimSpace(strings.TrimPrefix(trimmed, "setup_script:"))
						currentEnv.SetupScript = strings.Trim(currentEnv.SetupScript, "\"'")
					}
				}
			}
			continue
		}

		// Top-level keys (not indented)
		currentSection = ""

		if strings.HasPrefix(trimmed, "name:") {
			cfg.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
			cfg.Name = strings.Trim(cfg.Name, "\"'")
			continue
		}
		if strings.HasPrefix(trimmed, "testCommand:") {
			cfg.TestCommand = strings.TrimSpace(strings.TrimPrefix(trimmed, "testCommand:"))
			cfg.TestCommand = strings.Trim(cfg.TestCommand, "\"'")
			continue
		}
		if strings.HasPrefix(trimmed, "db:") {
			cfg.DBPath = strings.TrimSpace(strings.TrimPrefix(trimmed, "db:"))
			cfg.DBPath = strings.Trim(cfg.DBPath, "\"'")
			continue
		}
		if strings.HasPrefix(trimmed, "daemon_port:") {
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "daemon_port:"))
			raw = strings.Trim(raw, "\"'")
			if p, err := strconv.Atoi(raw); err == nil {
				cfg.DaemonPort = p
			}
			continue
		}
		if strings.HasPrefix(trimmed, "work_dir:") {
			cfg.WorkDir = strings.TrimSpace(strings.TrimPrefix(trimmed, "work_dir:"))
			cfg.WorkDir = strings.Trim(cfg.WorkDir, "\"'")
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
		if trimmed == "linters:" {
			currentSection = "linters"
			continue
		}
		if trimmed == "projects:" {
			currentSection = "projects"
			currentProject = nil
			continue
		}
		if trimmed == "environments:" {
			currentSection = "environments"
			currentEnv = nil
			envSubSection = ""
			continue
		}

		// Unknown top-level key — reject with error
		key := strings.SplitN(trimmed, ":", 2)[0]
		if !knownTopLevelKeys[key] {
			return nil, fmt.Errorf("config %s: unknown key %q (supported: testCommand, db, daemon_port, work_dir, env, test_packages, linters, projects, environments, categories)", path, key)
		}
	}

	// Flush last project
	if currentProject != nil {
		cfg.Projects = append(cfg.Projects, *currentProject)
		currentProject = nil
	}
	// Flush last environment
	if currentEnv != nil {
		if currentEnvCheck != nil && currentEnvKey != "" {
			currentEnv.Requires[currentEnvKey] = *currentEnvCheck
		}
		cfg.Environments = append(cfg.Environments, *currentEnv)
		currentEnv = nil
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

// ProjectWorkDir returns the work_dir for the named project, or "" if not found.
func (c *Config) ProjectWorkDir(name string) string {
	for _, p := range c.Projects {
		if p.Name == name {
			return p.WorkDir
		}
	}
	return ""
}

// DiscoverLinters returns linters discovered across all configured sources.
// When projects are defined, returns map[projectName]linters.
// When no projects, uses root-level config/lefthook and returns a single
// entry keyed by "".
func (c *Config) DiscoverLinters(workDir string) map[string][]LinterDef {
	result := make(map[string][]LinterDef)
	if len(c.Projects) > 0 {
		for _, p := range c.Projects {
			dir := p.WorkDir
			if dir == "" {
				dir = workDir
			}
			result[p.Name] = DiscoverLintersFromDir(dir)
		}
	} else {
		result[""] = DiscoverLintersFromDir(workDir)
	}
	return result
}

// DiscoverLintersForProject returns linters for a single named project.
func (c *Config) DiscoverLintersForProject(name, workDir string) []LinterDef {
	dir := c.ProjectWorkDir(name)
	if dir == "" {
		dir = workDir
	}
	return DiscoverLintersFromDir(dir)
}

// DiscoverLintersFromDir discovers linters from a directory, checking config
// linters first, then falling back to lefthook.yml "lint" group.
func DiscoverLintersFromDir(dir string) []LinterDef {
	// Try loading a per-directory config
	cfg, err := LoadConfigFromDir(dir)
	if err == nil && len(cfg.Linters) > 0 {
		return cfg.Linters
	}
	linters, _ := DiscoverLintersFromLefthook(dir, "lint")
	return linters
}

// LoadConfigFromDir loads config from a specific directory's .runlog/config.yaml.
func LoadConfigFromDir(dir string) (*Config, error) {
	paths := []string{
		filepath.Join(dir, ".runlog", "config.yaml"),
		filepath.Join(dir, ".runlog.yaml"),
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return parseConfigFile(path)
		}
	}
	return &Config{}, nil
}

// LookupEnvironment returns the named environment config, or nil.
func (c *Config) LookupEnvironment(name string) *EnvironmentConfig {
	for i := range c.Environments {
		if c.Environments[i].Name == name {
			return &c.Environments[i]
		}
	}
	return nil
}

func envNames(cfg *Config) string { //nolint:deadcode
	var names []string
	for _, e := range cfg.Environments {
		names = append(names, e.Name)
	}
	return strings.Join(names, ", ")
}

// LinterSource is a single linter with its originating directory.
type LinterSource struct {
	Project string
	LinterDef
}
