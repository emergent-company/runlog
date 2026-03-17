// Package e2eframework — credentials.go
//
// Helpers for verifying CLI-written credential files in tests.
package runlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// CredentialsInfo holds the parsed fields from ~/.memory/credentials.json
// that are relevant for test assertions.  Token values are redacted.
type CredentialsInfo struct {
	// Path is the absolute path of the file that was read.
	Path string
	// HasToken is true when the file contains a non-empty token field
	// (access_token, token, or api_key — whichever the CLI writes).
	HasToken bool
	// HasServerURL is true when the file contains a non-empty server_url field.
	HasServerURL bool
	// ServerURL is the server URL found in the file, or "" if absent.
	ServerURL string
	// RawKeys lists every top-level key present in the file.
	RawKeys []string
}

// VerifyCredentialsWritten asserts that credentials.json exists under
// <home>/.memory/ and contains a non-empty token value.
//
// It reads and parses the file, runs the following assertions via t.Errorf
// (non-fatal):
//   - file exists
//   - file contains valid JSON
//   - at least one recognised token field is non-empty
//     (access_token, token, api_key)
//
// If rl is non-nil, a "credentials" event is emitted with the message
// "credentials.json written" (or an error variant), recording the path,
// which fields were present, and the server URL — but never the token value.
func VerifyCredentialsWritten(t *testing.T, rl *RunLog, home string) CredentialsInfo {
	t.Helper()

	credsPath := filepath.Join(home, ".memory", "credentials.json")

	data, err := os.ReadFile(credsPath)
	if err != nil {
		t.Errorf("credentials.json not found at %s: %v", credsPath, err)
		if rl != nil {
			rl.Event("credentials", "credentials.json missing", map[string]any{
				"path":  credsPath,
				"error": err.Error(),
			})
		}
		return CredentialsInfo{Path: credsPath}
	}

	// Parse into a generic map so we can inspect all keys without knowing the
	// exact schema upfront.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Errorf("credentials.json at %s contains invalid JSON: %v", credsPath, err)
		if rl != nil {
			rl.Event("credentials", "credentials.json invalid JSON", map[string]any{
				"path":  credsPath,
				"error": err.Error(),
			})
		}
		return CredentialsInfo{Path: credsPath}
	}

	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}

	// Check for any recognised token field.
	hasToken := false
	for _, field := range []string{"access_token", "token", "api_key"} {
		if v, ok := raw[field]; ok {
			if s, ok := v.(string); ok && s != "" {
				hasToken = true
				break
			}
		}
	}
	if !hasToken {
		t.Errorf("credentials.json at %s has no recognised token field (checked: access_token, token, api_key); keys present: %v", credsPath, keys)
	}

	serverURL, _ := raw["server_url"].(string)

	info := CredentialsInfo{
		Path:         credsPath,
		HasToken:     hasToken,
		HasServerURL: serverURL != "",
		ServerURL:    serverURL,
		RawKeys:      keys,
	}

	if rl != nil {
		rl.Event("credentials", "credentials.json written", map[string]any{
			"path":           credsPath,
			"has_token":      hasToken,
			"has_server_url": info.HasServerURL,
			"server_url":     serverURL,
			"keys":           keys,
		})
	}
	return info
}
