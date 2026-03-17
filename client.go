// Package e2eframework — client.go
//
// HTTP helpers for making authenticated JSON requests to the Memory server.
package runlog

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// SetAuthHeader adds the correct authentication header to req.
// Tokens with an "emt_" prefix are project-scoped API tokens sent as
// "Authorization: Bearer". All other values are standalone API keys sent
// as "X-API-Key".
func SetAuthHeader(req *http.Request, token string) {
	if strings.HasPrefix(token, "emt_") {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("X-API-Key", token)
	}
}

// DoJSON performs an HTTP request with JSON body and the correct auth header.
// An optional X-Project-ID header is set when projectID is non-empty.
func DoJSON(t *testing.T, method, url, token, projectID string, body []byte) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	// Note: cancel is intentionally not deferred here so the caller can read the body.
	_ = cancel

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request %s %s: %v", method, url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	SetAuthHeader(req, token)
	if projectID != "" {
		req.Header.Set("X-Project-ID", projectID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	return resp
}

// ReadBody reads and closes the response body, returning it as a string.
func ReadBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(b)
}

// DoMCPJSON performs an HTTP POST to the MCP RPC endpoint with the correct
// MCP protocol headers. sessionID may be empty for the initialize call.
func DoMCPJSON(t *testing.T, url, token, projectID, sessionID, protocolVersion string, body []byte) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_ = cancel

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build MCP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	SetAuthHeader(req, token)
	if projectID != "" {
		req.Header.Set("X-Project-ID", projectID)
	}
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do MCP request: %v", err)
	}
	return resp
}

// Truncate returns s truncated to at most n runes with "…" appended if trimmed.
func Truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
