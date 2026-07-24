package servecmd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jamestelfer/dollop/internal/cli/servecmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getFreePort asks the OS for an available port.
func getFreePort(t *testing.T) string {
	t.Helper()
	lc := net.ListenConfig{}
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

func TestServe_MCPInitialize_ReportsVersion(t *testing.T) {
	addr := getFreePort(t)
	version := "1.2.3-test"

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- servecmd.RunServer(ctx, servecmd.EnvConfig{
			AccountID: "test-acct",
			Bucket:    "test-bucket",
			BaseURL:   "https://example.com",
			R2Key:     "test-key",
			R2Secret:  "test-secret",
			Addr:      addr,
		}, version, io.Discard)
	}()

	// Wait for server to be ready.
	require.Eventually(t, func() bool {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/healthz", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == 200
	}, 2*time.Second, 10*time.Millisecond)

	// Send an MCP initialize request.
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(initBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Parse the JSON-RPC response to check the version.
	var result struct {
		Result struct {
			ServerInfo struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &result), "response: %s", string(body))
	assert.Equal(t, "dollop", result.Result.ServerInfo.Name)
	assert.Equal(t, version, result.Result.ServerInfo.Version,
		fmt.Sprintf("MCP server should report injected version, got response: %s", string(body)))

	// Shutdown.
	cancel()
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}
