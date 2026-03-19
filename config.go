package runlog

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
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
	// Default: "go test -v -run {name} ./..."
	TestCommand string `yaml:"testCommand"`

	// DBPath is an explicit path to the runs.db SQLite database.
	// If set, overrides the default search path resolution.
	DBPath string `yaml:"db"`
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
	}

	scanner := bufio.NewScanner(f)
	var currentSection string // "", "categories"
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
			if trimmed == "categories:" {
				currentSection = "categories"
				currentCategory = ""
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
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	return cfg, nil
}

// TestCommandOrDefault returns the configured test command, or the default
// "go test -v -run {name} ./..." if none is configured.
func (c *Config) TestCommandOrDefault() string {
	if c.TestCommand != "" {
		return c.TestCommand
	}
	return "go test -v -run {name} ./..."
}

// CategoryForTest returns the category for a test name, or "Uncategorized"
// if the test is not assigned to any category.
func (c *Config) CategoryForTest(testName string) string {
	for cat, tests := range c.Categories {
		for _, t := range tests {
			if t == testName {
				return cat
			}
		}
	}
	return "Uncategorized"
}
