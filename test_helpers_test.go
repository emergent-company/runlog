package runlog

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestDB creates a temp directory with TEST_RUNS_DB and TEST_LOG_DIR
// pointing into it, resets the global DB singleton, and registers cleanup
// on t.Cleanup.  Tests that need an isolated RunLog call this first.
// Returns the temp dir path so callers can open the DB directly for
// post-run assertions.
func newTestDB(t *testing.T) string {
	t.Helper()

	// If TEST_RUNS_DB is already set (dogfood mode), don't override.
	// Tests write directly to the dogfood DB — no isolation, but that's
	// expected when running in dogfood mode.
	if existing := os.Getenv("TEST_RUNS_DB"); existing != "" {
		// Dogfood mode: create a temp dir with runs.db → dogfood DB symlink
		// so callers can still do filepath.Join(tmpDir, "runs.db") and get the right path.
		tmpDir := t.TempDir()
		os.Symlink(existing, filepath.Join(tmpDir, "runs.db"))
		resetSharedDB()
		t.Cleanup(resetSharedDB)
		return tmpDir
	}

	tmpDir := t.TempDir()

	dbPath := filepath.Join(tmpDir, "runs.db")
	os.Setenv("TEST_RUNS_DB", dbPath)
	t.Cleanup(func() { os.Unsetenv("TEST_RUNS_DB") })

	os.Setenv("TEST_LOG_DIR", tmpDir)
	t.Cleanup(func() { os.Unsetenv("TEST_LOG_DIR") })

	resetSharedDB()
	t.Cleanup(resetSharedDB)

	return tmpDir
}
