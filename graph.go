// Package e2eframework — graph.go
//
// Helpers for querying the Memory knowledge graph via the REST API.
package runlog

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ListByType fetches graph objects filtered by type via the REST API.
// If objectType is empty, all objects are returned.
// The server identifies the project via the X-Project-ID header (set by DoJSON).
func ListByType(t *testing.T, srv, token, projectID, objectType string) []map[string]any {
	t.Helper()
	apiURL := fmt.Sprintf("%s/api/graph/objects/search?limit=200", srv)
	if objectType != "" {
		apiURL += "&type=" + objectType
	}
	resp := DoJSON(t, "GET", apiURL, token, projectID, nil)
	if resp.StatusCode != 200 {
		body := ReadBody(t, resp)
		t.Logf("warn: list graph objects returned %d: %s", resp.StatusCode, body)
		return nil
	}
	body := ReadBody(t, resp)
	var result struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &result); err != nil {
		t.Logf("warn: decode graph objects response: %v\nraw: %s", err, Truncate(body, 200))
		return nil
	}
	return result.Items
}

// ListByLabel fetches graph objects filtered by label via the REST API.
func ListByLabel(t *testing.T, srv, token, projectID, label string) []map[string]any {
	t.Helper()
	apiURL := fmt.Sprintf("%s/api/graph/objects/search?limit=200&label=%s", srv, label)
	resp := DoJSON(t, "GET", apiURL, token, projectID, nil)
	if resp.StatusCode != 200 {
		body := ReadBody(t, resp)
		t.Logf("warn: list graph objects by label returned %d: %s", resp.StatusCode, body)
		return nil
	}
	body := ReadBody(t, resp)
	var result struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &result); err != nil {
		t.Logf("warn: decode graph objects response: %v\nraw: %s", err, Truncate(body, 200))
		return nil
	}
	return result.Items
}

// ListRelationships fetches relationships where dst_id matches targetID,
// optionally filtered by relationship type.
func ListRelationships(t *testing.T, srv, token, projectID, targetID, relType string) []map[string]any {
	t.Helper()
	apiURL := fmt.Sprintf("%s/api/graph/relationships/search?limit=200&dst_id=%s", srv, targetID)
	if relType != "" {
		apiURL += "&type=" + relType
	}
	resp := DoJSON(t, "GET", apiURL, token, projectID, nil)
	if resp.StatusCode != 200 {
		body := ReadBody(t, resp)
		t.Logf("warn: list relationships returned %d: %s", resp.StatusCode, body)
		return nil
	}
	body := ReadBody(t, resp)
	var result struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &result); err != nil {
		t.Logf("warn: decode relationships response: %v\nraw: %s", err, Truncate(body, 200))
		return nil
	}
	return result.Items
}

// ListRelationshipsFromCLI lists relationships where the source matches fromID,
// optionally filtered by relationship type.  Uses the CLI instead of HTTP.
// Returns the parsed items slice.  An empty slice is returned on any error.
func ListRelationshipsFromCLI(t *testing.T, home, projectID, fromID, relType string) []map[string]any {
	t.Helper()
	args := []string{
		"graph", "relationships", "list",
		"--from", fromID,
		"--limit", "200",
		"--project", projectID,
		"--output", "json",
	}
	if relType != "" {
		args = append(args, "--type", relType)
	}
	out := MustRunCLIInDirWithHome(t, "", home, args...)
	out = strings.TrimSpace(out)
	if out == "" || out == "null" || out == "[]" {
		return nil
	}

	// The CLI returns either a JSON array or an object with a "data" array.
	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err == nil {
		return items
	}
	var wrapper struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &wrapper); err != nil {
		t.Logf("warn: could not parse relationships CLI output: %v\nraw: %s", err, Truncate(out, 300))
		return nil
	}
	return wrapper.Data
}

// PropString extracts a string value from a properties map, trying keys in order.
func PropString(props map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := props[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
