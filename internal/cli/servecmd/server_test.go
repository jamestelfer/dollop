package servecmd_test

import (
	"bufio"
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

// sseEvent holds a parsed SSE event.
type sseEvent struct {
	Event string
	Data  string
}

// readSSEEvents reads SSE events from the body in a goroutine.
func readSSEEvents(body io.Reader) <-chan sseEvent {
	ch := make(chan sseEvent, 10)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(body)
		var eventType string
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				eventType = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ch <- sseEvent{Event: eventType, Data: strings.TrimPrefix(line, "data: ")}
				eventType = ""
			case line == "":
				// empty line separates events
			}
		}
	}()
	return ch
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
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/status", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == 200
	}, 2*time.Second, 10*time.Millisecond)

	// Step 1: Connect to SSE endpoint.
	sseReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/sse", nil)
	require.NoError(t, err)
	sseResp, err := http.DefaultClient.Do(sseReq)
	require.NoError(t, err)
	defer func() { _ = sseResp.Body.Close() }()
	require.Equal(t, http.StatusOK, sseResp.StatusCode)

	events := readSSEEvents(sseResp.Body)

	// Read the endpoint event.
	var messageEndpoint string
	select {
	case ev := <-events:
		require.Equal(t, "endpoint", ev.Event)
		messageEndpoint = ev.Data
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for endpoint event")
	}
	require.NotEmpty(t, messageEndpoint)

	// Step 2: POST an MCP initialize request to the message endpoint.
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}`
	msgReq, err := http.NewRequestWithContext(ctx, http.MethodPost, messageEndpoint, strings.NewReader(initBody))
	require.NoError(t, err)
	msgReq.Header.Set("Content-Type", "application/json")

	msgResp, err := http.DefaultClient.Do(msgReq)
	require.NoError(t, err)
	_ = msgResp.Body.Close()
	require.Equal(t, http.StatusAccepted, msgResp.StatusCode)

	// Step 3: Read the initialize response from the SSE stream.
	var responseLine string
	select {
	case ev := <-events:
		require.Equal(t, "message", ev.Event)
		responseLine = ev.Data
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for initialize response")
	}

	var result struct {
		Result struct {
			ServerInfo struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(responseLine), &result),
		"response: %s", responseLine)
	assert.Equal(t, "dollop", result.Result.ServerInfo.Name)
	assert.Equal(t, version, result.Result.ServerInfo.Version)

	// Shutdown.
	cancel()
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}
