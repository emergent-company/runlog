// cmd/runlog/skills.go — "runlog skills" subcommand implementation.
//
// Skills are embedded in the binary via go:embed. The install command copies
// them from the embedded filesystem to each target tool's install directory.
// Only skills with the "runlog-" prefix are included.
package main

import (
	"bufio"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed skills/*
var embeddedSkills embed.FS

// toolEntry describes one registered AI agent tool.
type toolEntry struct {
	// MarkerPath is a path relative to project root whose existence signals
	// that this tool is configured in the project.
	MarkerPath string
	// InstallDir is the skill install directory relative to project root.
	InstallDir string
}

// toolRegistry is the static map of tool ID → detection marker + install path.
var toolRegistry = map[string]toolEntry{
	"opencode": {MarkerPath: ".opencode", InstallDir: ".opencode/skills"},
	"claude":   {MarkerPath: ".claude", InstallDir: ".claude/skills"},
	"cursor":   {MarkerPath: ".cursor", InstallDir: ".cursor/skills"},
	"agents":   {MarkerPath: ".agents", InstallDir: ".agents/skills"},
	"windsurf": {MarkerPath: ".windsurf", InstallDir: ".windsurf/skills"},
	"cline":    {MarkerPath: ".cline", InstallDir: ".cline/skills"},
	"aider":    {MarkerPath: ".aider", InstallDir: ".aider/skills"},
	"continue": {MarkerPath: ".continue", InstallDir: ".continue/skills"},
	"zed":      {MarkerPath: ".zed", InstallDir: ".zed/skills"},
	"copilot":  {MarkerPath: ".github/copilot-instructions.md", InstallDir: ".github/skills"},
}

// detectTools iterates toolRegistry, checks each marker path, and returns the
// IDs of tools whose marker exists under projectRoot.
func detectTools(projectRoot string) []string {
	var detected []string
	for id, entry := range toolRegistry {
		marker := filepath.Join(projectRoot, filepath.FromSlash(entry.MarkerPath))
		if _, err := os.Stat(marker); err == nil {
			detected = append(detected, id)
		}
	}
	sort.Strings(detected)
	return detected
}

// skillEntry represents one installable skill from the embedded filesystem.
type skillEntry struct {
	Name string
}

// discoverEmbeddedSkills reads the embedded skills/ directory and returns one
// skillEntry per subdirectory that has the "runlog-" prefix.
func discoverEmbeddedSkills() ([]skillEntry, error) {
	entries, err := fs.ReadDir(embeddedSkills, "skills")
	if err != nil {
		return nil, fmt.Errorf("reading embedded skills: %w", err)
	}

	var skills []skillEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "runlog-") {
			continue
		}
		skills = append(skills, skillEntry{Name: name})
	}

	if len(skills) == 0 {
		return nil, fmt.Errorf("no embedded runlog skills found")
	}
	return skills, nil
}

// selectTools presents a numbered list of detected tools and reads a
// comma-separated selection from stdin. Returns nil, nil if the user cancels.
func selectTools(detected []string) ([]string, error) {
	if len(detected) == 0 {
		return nil, nil
	}
	fmt.Println("Detected agent tools:")
	for i, id := range detected {
		fmt.Printf("  %d) %s\n", i+1, id)
	}
	fmt.Print("Select tools (comma-separated numbers, \"all\", or Enter to cancel): ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return nil, nil
	}
	line := strings.TrimSpace(scanner.Text())

	switch strings.ToLower(line) {
	case "", "none":
		return nil, nil
	case "all":
		return detected, nil
	}

	var selected []string
	for _, tok := range strings.Split(line, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		n, err := strconv.Atoi(tok)
		if err != nil || n < 1 || n > len(detected) {
			return nil, fmt.Errorf("invalid selection %q", tok)
		}
		selected = append(selected, detected[n-1])
	}
	return selected, nil
}

// installEmbeddedSkill copies an embedded skill directory to dstDir on disk.
// Returns true if the install was performed (false means skipped due to
// existing install without --force).
func installEmbeddedSkill(skillName, dstDir string, force bool, dryRun bool) (bool, error) {
	if _, err := os.Stat(dstDir); err == nil {
		if !force {
			fmt.Printf("skipped: %s (already exists; use --force to overwrite)\n", skillName)
			return false, nil
		}
		if !dryRun {
			if err := os.RemoveAll(dstDir); err != nil {
				return false, fmt.Errorf("removing %s: %w", dstDir, err)
			}
		}
	}

	if dryRun {
		fmt.Printf("would install: %s → %s\n", skillName, dstDir)
		return true, nil
	}

	// Walk the embedded skill directory and write each file to disk.
	srcRoot := "skills/" + skillName
	err := fs.WalkDir(embeddedSkills, srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Compute relative path from the skill root.
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dstDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

		// Read from embedded FS.
		data, err := fs.ReadFile(embeddedSkills, path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}

		// Write to disk.
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, 0644)
	})
	if err != nil {
		return false, fmt.Errorf("installing %s: %w", skillName, err)
	}
	return true, nil
}

// cmdSkills dispatches the "skills" subcommand.
func cmdSkills(args []string) error {
	if len(args) == 0 {
		skillsUsage()
		return nil
	}
	switch args[0] {
	case "install":
		return cmdSkillsInstall(args[1:])
	case "list":
		return cmdSkillsList()
	case "help", "--help", "-h":
		skillsUsage()
		return nil
	default:
		return fmt.Errorf("unknown skills action %q; try \"runlog skills --help\"", args[0])
	}
}

// cmdSkillsList prints all embedded skills.
func cmdSkillsList() error {
	skills, err := discoverEmbeddedSkills()
	if err != nil {
		return err
	}
	fmt.Printf("Embedded skills (%d):\n", len(skills))
	for _, s := range skills {
		fmt.Printf("  %s\n", s.Name)
	}
	return nil
}

// cmdSkillsInstall implements "runlog skills install".
func cmdSkillsInstall(args []string) error {
	installFS := flag.NewFlagSet("skills install", flag.ContinueOnError)
	all := installFS.Bool("all", false, "install for all detected agent tools without prompting")
	tools := installFS.String("tools", "", "comma-separated tool IDs to target (e.g. opencode,claude)")
	force := installFS.Bool("force", false, "overwrite existing skill installs")
	dryRun := installFS.Bool("dry-run", false, "print planned actions without writing")
	installFS.Usage = skillsInstallUsage
	if err := installFS.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Discover embedded skills.
	skills, err := discoverEmbeddedSkills()
	if err != nil {
		return err
	}

	// Determine target tools.
	var targetTools []string
	if *tools != "" {
		for _, id := range strings.Split(*tools, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := toolRegistry[id]; !ok {
				valid := make([]string, 0, len(toolRegistry))
				for k := range toolRegistry {
					valid = append(valid, k)
				}
				sort.Strings(valid)
				return fmt.Errorf("unknown tool ID %q; valid IDs: %s", id, strings.Join(valid, ", "))
			}
			targetTools = append(targetTools, id)
		}
	} else {
		detected := detectTools(projectRoot)
		if len(detected) == 0 {
			fmt.Println("no agent tools detected; use --tools to specify targets")
			return nil
		}
		if *all {
			targetTools = detected
		} else {
			targetTools, err = selectTools(detected)
			if err != nil {
				return err
			}
			if len(targetTools) == 0 {
				fmt.Println("no tools selected; nothing to install")
				return nil
			}
		}
	}

	// Install each embedded skill for each selected tool.
	totalInstalled := 0
	for _, toolID := range targetTools {
		entry := toolRegistry[toolID]
		installBase := filepath.Join(projectRoot, filepath.FromSlash(entry.InstallDir))

		for _, skill := range skills {
			dstDir := filepath.Join(installBase, skill.Name)
			ok, err := installEmbeddedSkill(skill.Name, dstDir, *force, *dryRun)
			if err != nil {
				return fmt.Errorf("installing %s for %s: %w", skill.Name, toolID, err)
			}
			if ok {
				totalInstalled++
			}
		}
	}

	if *dryRun {
		fmt.Printf("dry-run: would install %d skills for %d tools\n", len(skills)*len(targetTools), len(targetTools))
	} else {
		fmt.Printf("installed %d skills for %d tools\n", totalInstalled, len(targetTools))
	}
	return nil
}

func skillsUsage() {
	fmt.Fprint(os.Stderr, `runlog skills — manage AI agent skills

USAGE
  runlog skills install [flags]   install embedded skills into agent tool directories
  runlog skills list              list all embedded skills

Run "runlog skills install --help" for details.
`)
}

func skillsInstallUsage() {
	fmt.Fprint(os.Stderr, `runlog skills install — copy embedded skills into agent tool install paths

Skills are bundled inside the runlog binary. Only runlog-* prefixed skills
are included — no external skill source directories are needed.

USAGE
  runlog skills install [flags]

FLAGS
  --all            install for all detected agent tools without prompting
  --tools <ids>    comma-separated tool IDs to target (e.g. opencode,claude)
  --force          overwrite existing skill installs
  --dry-run        print planned actions without modifying the filesystem

EXAMPLES
  runlog skills install                           # interactive tool selection
  runlog skills install --all                     # install for all detected tools
  runlog skills install --tools opencode          # install for opencode only
  runlog skills install --dry-run --all           # preview without writing
  runlog skills install --tools opencode --force  # overwrite existing installs
`)
}
