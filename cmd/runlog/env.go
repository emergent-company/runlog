package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	runlog "github.com/emergent-company/runlog"
)

func cmdEnv(args []string) error {
	subCmd := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		subCmd = args[0]
		args = args[1:]
	}

	switch subCmd {
	case "list", "":
		return cmdEnvList(args)
	case "show":
		return cmdEnvShow(args)
	case "validate":
		return cmdEnvValidate(args)
	case "test":
		return cmdEnvTest(args)
	case "setup":
		return cmdEnvSetup(args)
	default:
		return fmt.Errorf("unknown env subcommand %q (use list, show, validate, test, setup)", subCmd)
	}
}

func cmdEnvList(args []string) error {
	fs := flag.NewFlagSet("env list", flag.ContinueOnError)
	var db string
	fs.StringVar(&db, "db", "", "")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: runlog env list

List all configured environments from .runlog/config.yaml.
Shows env var count, check count, and validation status.
`)
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	dbDir := filepath.Dir(resolveDBPath(db))
	cfg, _ := runlog.LoadConfig(dbDir)

	if len(cfg.Environments) == 0 {
		fmt.Println("No environments configured.")
		fmt.Println("  Add an 'environments:' section to .runlog/config.yaml")
		return nil
	}

	fmt.Printf("%-24s %8s %8s  %7s %7s  Status\n", "ENVIRONMENT", "VARS", "CHECKS", "TEST", "SETUP")
	fmt.Println(strings.Repeat("─", 80))
	for _, e := range cfg.Environments {
		varCount := len(e.Env)
		checkCount := len(e.Requires)
		testFlag := "—"
		setupFlag := "—"
		if e.TestScript != "" {
			testFlag = "✓"
		}
		if e.SetupScript != "" {
			setupFlag = "✓"
		}
		status := runlog.ValidateEnvSummary(&e)
		statusStr := "✓"
		if status != nil {
			statusStr = "✗"
		}
		fmt.Printf("%-24s %8d %8d  %7s %7s  %s\n", e.Name, varCount, checkCount, testFlag, setupFlag, statusStr)
	}
	return nil
}

func cmdEnvShow(args []string) error {
	fs := flag.NewFlagSet("env show", flag.ContinueOnError)
	var db string
	fs.StringVar(&db, "db", "", "")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: runlog env show <name>

Show full environment configuration: env vars, requirement checks, and scripts.
`)
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	name := strings.Join(fs.Args(), " ")
	if name == "" {
		return fmt.Errorf("missing environment name")
	}

	dbDir := filepath.Dir(resolveDBPath(db))
	cfg, _ := runlog.LoadConfig(dbDir)
	env := cfg.LookupEnvironment(name)
	if env == nil {
		return fmt.Errorf("environment %q not found", name)
	}

	fmt.Printf("Environment: %s\n\n", env.Name)

	if len(env.Env) > 0 {
		fmt.Println("  Env Vars:")
		for k, v := range env.Env {
			fmt.Printf("    %s=%s\n", k, v)
		}
		fmt.Println()
	}

	if len(env.Requires) > 0 {
		fmt.Println("  Requirements:")
		for key, check := range env.Requires {
			val := os.Getenv(key)
			if val == "" {
				val = env.Env[key]
			}
			if val == "" {
				val = check.Default
			}
			status := "?"
			if val != "" {
				err := runlog.CheckRequirement(key, val, check)
				if err != nil {
					status = "✗ " + err.Error()
				} else {
					status = "✓"
				}
			} else {
				status = "✗ not set"
				if check.Hint != "" {
					status += " (" + check.Hint + ")"
				}
			}
			fmt.Printf("    %-24s %s\n", key+":", status)
		}
		fmt.Println()
	}

	if env.TestScript != "" {
		fmt.Printf("  Test Script:  %s\n", env.TestScript)
	}
	if env.SetupScript != "" {
		fmt.Printf("  Setup Script: %s\n", env.SetupScript)
	}

	if len(env.Env) == 0 && len(env.Requires) == 0 && env.TestScript == "" && env.SetupScript == "" {
		fmt.Println("  (no env vars or requirements configured)")
	}

	return nil
}

func cmdEnvValidate(args []string) error {
	fs := flag.NewFlagSet("env validate", flag.ContinueOnError)
	var db string
	fs.StringVar(&db, "db", "", "")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: runlog env validate <name>

Validate all requirements for a named environment.
Exits with code 1 if any check fails.
`)
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	name := strings.Join(fs.Args(), " ")
	if name == "" {
		return fmt.Errorf("missing environment name")
	}

	dbDir := filepath.Dir(resolveDBPath(db))
	cfg, _ := runlog.LoadConfig(dbDir)
	env := cfg.LookupEnvironment(name)
	if env == nil {
		return fmt.Errorf("environment %q not found", name)
	}

	results := runlog.ValidateEnv(env)
	allPass := true
	for _, r := range results {
		icon := "✓"
		if !r.Pass {
			icon = "✗"
			allPass = false
		}
		msg := fmt.Sprintf("  %s %s", icon, r.Key)
		if r.Value != "" {
			msg += " → " + r.Value
		}
		if !r.Pass {
			msg += "  " + r.Error
			if r.Hint != "" {
				msg += " (" + r.Hint + ")"
			}
		}
		fmt.Println(msg)
	}

	if allPass {
		fmt.Printf("\n  Environment %q: all checks passed ✓\n", name)
		return nil
	}
	return fmt.Errorf("environment %q: some checks failed", name)
}

func cmdEnvTest(args []string) error {
	fs := flag.NewFlagSet("env test", flag.ContinueOnError)
	var db string
	fs.StringVar(&db, "db", "", "")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: runlog env test <name>

Run the environment's test_script to validate the environment is ready.
`)
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	name := strings.Join(fs.Args(), " ")
	if name == "" {
		return fmt.Errorf("missing environment name")
	}

	dbDir := filepath.Dir(resolveDBPath(db))
	cfg, _ := runlog.LoadConfig(dbDir)
	env := cfg.LookupEnvironment(name)
	if env == nil {
		return fmt.Errorf("environment %q not found", name)
	}
	if env.TestScript == "" {
		return fmt.Errorf("environment %q has no test_script configured", name)
	}

	// Set env vars from environment config so the script can read them
	setEnvFromConfig(env)
	serverURL := os.Getenv("MEMORY_TEST_SERVER")

	fmt.Printf("Running test_script for environment %q: %s\n", name, env.TestScript)
	return runScript(env.TestScript, serverURL)
}

func cmdEnvSetup(args []string) error {
	fs := flag.NewFlagSet("env setup", flag.ContinueOnError)
	var db string
	fs.StringVar(&db, "db", "", "")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: runlog env setup <name>

Run the environment's setup_script to prepare the environment.
`)
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	name := strings.Join(fs.Args(), " ")
	if name == "" {
		return fmt.Errorf("missing environment name")
	}

	dbDir := filepath.Dir(resolveDBPath(db))
	cfg, _ := runlog.LoadConfig(dbDir)
	env := cfg.LookupEnvironment(name)
	if env == nil {
		return fmt.Errorf("environment %q not found", name)
	}
	if env.SetupScript == "" {
		return fmt.Errorf("environment %q has no setup_script configured", name)
	}

	setEnvFromConfig(env)
	serverURL := os.Getenv("MEMORY_TEST_SERVER")

	fmt.Printf("Running setup_script for environment %q: %s\n", name, env.SetupScript)
	return runScript(env.SetupScript, serverURL)
}

func runScript(script, serverURL string) error {
	wd, _ := os.Getwd()
	cmd := exec.Command("sh", script, serverURL)
	if wd != "" {
		cmd.Dir = wd
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func setEnvFromConfig(env *runlog.EnvironmentConfig) {
	for k, v := range env.Env {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}
