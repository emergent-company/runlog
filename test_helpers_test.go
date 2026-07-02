package runlog

import (
	"sync"
	"testing"
)

// newTestDB creates a temp directory with an isolated runs.db and sets it as
// the shared global DB so NewRunLog / SharedDB find it.
//
// Deprecated: use StartTestDaemon instead for daemon-backed tests.
func newTestDB(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/runs.db"

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}

	globalDBMu.Lock()
	if globalDB != nil {
		globalDB.Close()
	}
	globalDB = db
	globalDBErr = nil
	globalDBOnce = sync.Once{}
	globalDBMu.Unlock()

	t.Cleanup(func() {
		globalDBMu.Lock()
		if globalDB == db {
			globalDB.Close()
			globalDB = nil
			globalDBOnce = sync.Once{}
		}
		globalDBMu.Unlock()
	})

	return tmpDir
}
