package runlog

import (
	"os"
	"path/filepath"
)

// RunlogDir finds the appropriate .runlog directory for the current project.
// It walks up the directory tree looking for an existing .runlog directory,
// or a .git directory. If neither is found, it defaults to .runlog in the
// current working directory.
func RunlogDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ".runlog"
	}

	// 1. Walk up to find an existing .runlog directory
	dir := wd
	for {
		candidate := filepath.Join(dir, ".runlog")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// 2. Walk up to find .git directory to establish project root
	dir = wd
	for {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			return filepath.Join(dir, ".runlog")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// 3. Fallback to current working directory
	return filepath.Join(wd, ".runlog")
}

// DaemonPidFile returns the path to the daemon PID file.
// The file lives in the .runlog directory of the current project.
func DaemonPidFile() string {
	return filepath.Join(RunlogDir(), "daemon.pid")
}
