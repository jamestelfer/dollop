package servecmd_test

import (
	"context"
	"encoding/json"
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

// startServer runs the server in the background and returns its address and
// a stop function that cancels it and waits for a clean shutdown.
func startServer(t *testing.T, version string) (addr string, stop func()) {
	t.Helper()
	addr = getFreePort(t)

	ctx, cancel := context.WithCancel(t.Context())
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

	require.Eventually(t, func() bool {
		return get(t, "http://"+addr+"/status") == http.StatusOK
	}, 2*time.Second, 10*time.Millisecond)

	return addr, func() {
		cancel()
		select {
		case err := <-errCh:
			assert.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Fatal("server did not shut down in time")
		}
	}
}

// get issues a GET and returns the status code, or 0 on transport error.
func get(t *testing.T, url string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestServe_MCPInitialize_StreamableHTTP_ReportsVersion(t *testing.T) {
	version := "1.2.3-test"
	addr, stop := startServer(t, version)
	defer stop()

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(initBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

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
	assert.Equal(t, version, result.Result.ServerInfo.Version)
}

func TestServe_SSEEndpointsRemoved(t *testing.T) {
	addr, stop := startServer(t, "test")
	defer stop()

	assert.Equal(t, http.StatusNotFound, get(t, "http://"+addr+"/sse"))
	assert.Equal(t, http.StatusNotFound, get(t, "http://"+addr+"/message"))
}
