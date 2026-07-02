package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	runlog "github.com/emergent-company/runlog"
)

func cmdLint(args []string) error {
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	var projectFilter string
	fs.StringVar(&projectFilter, "project", "", "run linters only for the named project")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: runlog lint [<linter-name> ...]

Run linters discovered from lefthook.yml or .runlog/config.yaml.
With no arguments, runs all discovered linters across all projects.
With --project, runs linters only for that project.

FLAGS
  --project <name>   run linters only for the named project (from config projects:)
  --db <path>        path to runs.db  (not used by lint, accepted for consistency)
  --since <dur>      ignored, accepted for consistency

EXAMPLES
  runlog lint                    # run all linters in all projects
  runlog lint gofmt go-vet       # run specific linters only
  runlog lint --project core     # run linters only for core project
  runlog lint --project e2e      # run linters only for e2e project
`)
	}
	var db, since string
	fs.StringVar(&db, "db", "", "")
	fs.StringVar(&since, "since", "24h", "")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	filterNames := fs.Args()

	wd, _ := os.Getwd()

	// Load config to discover projects
	dbDir := filepath.Dir(resolveDBPath(db))
	cfg, _ := runlog.LoadConfig(dbDir)

	// Discover linters per project
	allLinters := cfg.DiscoverLinters(wd)

	// Filter by project if --project was given
	if projectFilter != "" {
		linters, ok := allLinters[projectFilter]
		if !ok {
			return fmt.Errorf("unknown project %q (available: %s)", projectFilter, projectNames(cfg))
		}
		allLinters = map[string][]runlog.LinterDef{projectFilter: linters}
	}

	if len(allLinters) == 0 || allLintersEmpty(allLinters) {
		fmt.Fprintln(os.Stderr, "runlog lint: no linters found")
		fmt.Fprintln(os.Stderr, "  Add a 'lint:' group to lefthook.yml or a 'linters:' section to .runlog/config.yaml")
		return nil
	}

	sep := strings.Repeat("─", 72)
	exitCode := 0

	// Sort project names for deterministic output
	projectNames := sortedKeys(allLinters)

	for _, proj := range projectNames {
		linters := allLinters[proj]
		if len(linters) == 0 {
			continue
		}

		workDir := wd
		if proj != "" {
			if pwd := cfg.ProjectWorkDir(proj); pwd != "" {
				workDir = pwd
			}
		}

		// Apply linter name filter
		if len(filterNames) > 0 {
			filtered := make([]runlog.LinterDef, 0, len(filterNames))
			for _, name := range filterNames {
				found := false
				for _, l := range linters {
					if l.Name == name {
						filtered = append(filtered, l)
						found = true
						break
					}
				}
				if !found {
					// Check other projects too
					for _, proj2 := range projectNames {
						for _, l := range allLinters[proj2] {
							if l.Name == name {
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if !found {
						return fmt.Errorf("unknown linter %q", name)
					}
				}
			}
			linters = filtered
		}

		if len(linters) == 0 {
			continue
		}

		// Print project header when multi-project
		showProject := len(projectNames) > 1 || proj != ""
		for _, l := range linters {
			cmd := runlog.EnsureRunAllCommand(l.Command)
			projPrefix := ""
			if showProject {
				projPrefix = fmt.Sprintf("[%s] ", proj)
			}
			fmt.Printf("\n%s\n  %sLint: %s\n  %sCmd:  %s\n%s\n", sep, projPrefix, l.Name, projPrefix, cmd, sep)

			c := exec.Command("sh", "-c", cmd)
			if workDir != "" {
				c.Dir = workDir
			}
			var buf bytes.Buffer
			c.Stdout = &buf
			c.Stderr = &buf

			err := c.Run()
			out := strings.TrimRight(buf.String(), "\n")
			if out != "" {
				// Indent output under the linter header
				for _, line := range strings.Split(out, "\n") {
					fmt.Printf("  %s%s\n", projPrefix, line)
				}
			}

			if err != nil {
				fmt.Printf("  %s→ FAIL (exit code: %d)\n\n", projPrefix, exitCodeFromCmd(err))
				exitCode = 1
			} else {
				fmt.Printf("  %s→ PASS\n\n", projPrefix)
			}
		}
	}

	if exitCode != 0 {
		return fmt.Errorf("some linters failed")
	}
	return nil
}

func exitCodeFromCmd(err error) int {
	if err == nil {
		return 0
	}
	type exitCoder interface{ ExitCode() int }
	if ec, ok := err.(exitCoder); ok {
		return ec.ExitCode()
	}
	return 1
}

func projectNames(cfg *runlog.Config) string {
	names := make([]string, len(cfg.Projects))
	for i, p := range cfg.Projects {
		names[i] = p.Name
	}
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

func allLintersEmpty(m map[string][]runlog.LinterDef) bool {
	for _, v := range m {
		if len(v) > 0 {
			return false
		}
	}
	return true
}
