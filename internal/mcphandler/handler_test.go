package mcphandler_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamestelfer/dollop/internal/mcphandler"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateInput(t *testing.T) {
	tests := []struct {
		name      string
		fileName  string
		content   string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "path separator slash",
			fileName:  "dir/file.md",
			content:   "hello",
			wantErr:   true,
			errSubstr: "path separator",
		},
		{
			name:      "path separator backslash",
			fileName:  "dir\\file.md",
			content:   "hello",
			wantErr:   true,
			errSubstr: "path separator",
		},
		{
			name:      "dot filename",
			fileName:  ".",
			content:   "hello",
			wantErr:   true,
			errSubstr: "invalid filename",
		},
		{
			name:      "dotdot filename",
			fileName:  "..",
			content:   "hello",
			wantErr:   true,
			errSubstr: "invalid filename",
		},
		{
			name:      "parent traversal",
			fileName:  "../etc/passwd",
			content:   "hello",
			wantErr:   true,
			errSubstr: "path separator",
		},
		{
			name:      "unsupported extension png",
			fileName:  "photo.png",
			content:   "hello",
			wantErr:   true,
			errSubstr: ".md",
		},
		{
			name:      "unsupported extension png mentions html",
			fileName:  "photo.png",
			content:   "hello",
			wantErr:   true,
			errSubstr: ".html",
		},
		{
			name:      "no extension",
			fileName:  "readme",
			content:   "hello",
			wantErr:   true,
			errSubstr: "unsupported file extension",
		},
		{
			name:      "empty content",
			fileName:  "doc.md",
			content:   "",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:     "valid markdown",
			fileName: "doc.md",
			content:  "# Hello",
			wantErr:  false,
		},
		{
			name:     "valid html",
			fileName: "page.html",
			content:  "<html>",
			wantErr:  false,
		},
		{
			name:     "uppercase extension accepted",
			fileName: "DOC.MD",
			content:  "hello",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mcphandler.ValidateInput(tt.fileName, tt.content)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// makeToolRequest constructs a CallToolRequest with the given arguments.
func makeToolRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "create_upload",
			Arguments: args,
		},
	}
}

func TestHandler_InvalidExtension_ReturnsError(t *testing.T) {
	handler := mcphandler.Handler(nil)
	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "photo.png",
		"content": "some data",
	}))
	require.NoError(t, err) // transport-level error must be nil
	require.True(t, result.IsError, "should be an MCP error result")
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, ".md")
	assert.Contains(t, text, ".html")
}

func TestHandler_PathTraversal_ReturnsError(t *testing.T) {
	handler := mcphandler.Handler(nil)
	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "../etc/passwd",
		"content": "data",
	}))
	require.NoError(t, err)
	require.True(t, result.IsError)
}

func TestHandler_PathSeparator_ReturnsError(t *testing.T) {
	handler := mcphandler.Handler(nil)
	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "dir/file.md",
		"content": "data",
	}))
	require.NoError(t, err)
	require.True(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "path separator")
}

func TestHandler_EmptyContent_ReturnsError(t *testing.T) {
	handler := mcphandler.Handler(nil)
	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "doc.md",
		"content": "",
	}))
	require.NoError(t, err)
	require.True(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "empty")
}

func TestHandler_ValidMarkdown_ReturnsSuccess(t *testing.T) {
	handler := mcphandler.Handler(nil)
	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "doc.md",
		"content": "# Hello",
	}))
	require.NoError(t, err)
	assert.False(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "doc.md")
}

func TestHandler_ValidHTML_ReturnsSuccess(t *testing.T) {
	handler := mcphandler.Handler(nil)
	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "page.html",
		"content": "<html>",
	}))
	require.NoError(t, err)
	assert.False(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "page.html")
}

// fakeUploader records PutObject calls for verification.
type fakeUploader struct {
	keys []string
	err  error
}

func (f *fakeUploader) PutObject(_ context.Context, _, key, _ string, _ io.Reader, _ ...upload.PutOption) error {
	if f.err != nil {
		return f.err
	}
	f.keys = append(f.keys, key)
	return nil
}

func (f *fakeUploader) ListObjects(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

var _ upload.ListingUploader = (*fakeUploader)(nil)

func TestHandler_HTMLUpload_ReturnsURL(t *testing.T) {
	up := &fakeUploader{}
	uploadFn := mcphandler.NewUploadFunc(
		up,
		"test-bucket",
		"https://cdn.example.com",
		func() (string, error) { return "testid", nil },
	)
	handler := mcphandler.Handler(uploadFn)

	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "page.html",
		"content": "<h1>Hello</h1>",
	}))
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success result")

	text := result.Content[0].(mcp.TextContent).Text
	var response map[string]string
	require.NoError(t, json.Unmarshal([]byte(text), &response))
	assert.Contains(t, response["url"], "https://cdn.example.com/flash/7/testid/page.html")
}

func TestHandler_MarkdownUpload_ReturnsBothURLs(t *testing.T) {
	up := &fakeUploader{}
	uploadFn := mcphandler.NewUploadFunc(
		up,
		"test-bucket",
		"https://cdn.example.com",
		func() (string, error) { return "testid", nil },
	)
	handler := mcphandler.Handler(uploadFn)

	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "doc.md",
		"content": "# Hello",
	}))
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success result")

	text := result.Content[0].(mcp.TextContent).Text
	var response map[string]string
	require.NoError(t, json.Unmarshal([]byte(text), &response))
	assert.Contains(t, response["html_url"], "https://cdn.example.com/flash/7/testid/doc.html")
	assert.Contains(t, response["markdown_url"], "https://cdn.example.com/flash/7/testid/doc.md")

	// Both .md and .html should have been uploaded
	assert.Contains(t, up.keys, "flash/7/testid/doc.md")
	assert.Contains(t, up.keys, "flash/7/testid/doc.html")
}

func TestHandler_UploadError_ReturnsMCPError(t *testing.T) {
	up := &fakeUploader{err: assert.AnError}
	uploadFn := mcphandler.NewUploadFunc(
		up,
		"test-bucket",
		"https://cdn.example.com",
		func() (string, error) { return "testid", nil },
	)
	handler := mcphandler.Handler(uploadFn)

	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "page.html",
		"content": "<html>",
	}))
	require.NoError(t, err) // transport error must be nil
	require.True(t, result.IsError, "upload failure should produce MCP error")
}

func TestHandler_HTMLUpload_UsesBaseURL(t *testing.T) {
	up := &fakeUploader{}
	uploadFn := mcphandler.NewUploadFunc(
		up,
		"test-bucket",
		"https://my-cdn.example.com",
		func() (string, error) { return "abc123", nil },
	)
	handler := mcphandler.Handler(uploadFn)

	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "report.html",
		"content": "<h1>Report</h1>",
	}))
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := result.Content[0].(mcp.TextContent).Text
	var response map[string]string
	require.NoError(t, json.Unmarshal([]byte(text), &response))
	assert.Equal(t, "https://my-cdn.example.com/flash/7/abc123/report.html", response["url"])
}

func TestHandler_HTMLUpload_DirUploader_WritesFile(t *testing.T) {
	outDir := t.TempDir()
	up := &upload.DirUploader{Root: outDir}
	uploadFn := mcphandler.NewUploadFunc(
		up,
		"ignored-bucket",
		"https://cdn.example.com",
		func() (string, error) { return "testid", nil },
	)
	handler := mcphandler.Handler(uploadFn)

	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "page.html",
		"content": "<h1>Hello</h1>",
	}))
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Verify the file was written to the expected path.
	got, err := os.ReadFile(filepath.Join(outDir, "flash", "7", "testid", "page.html"))
	require.NoError(t, err)
	assert.Equal(t, "<h1>Hello</h1>", string(got))
}

func TestHandler_MarkdownUpload_DirUploader_WritesBothFiles(t *testing.T) {
	outDir := t.TempDir()
	up := &upload.DirUploader{Root: outDir}
	uploadFn := mcphandler.NewUploadFunc(
		up,
		"ignored-bucket",
		"https://cdn.example.com",
		func() (string, error) { return "testid", nil },
	)
	handler := mcphandler.Handler(uploadFn)

	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "notes.md",
		"content": "# My Notes",
	}))
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Both .md and .html should exist on disk.
	mdPath := filepath.Join(outDir, "flash", "7", "testid", "notes.md")
	htmlPath := filepath.Join(outDir, "flash", "7", "testid", "notes.html")
	assert.FileExists(t, mdPath)
	assert.FileExists(t, htmlPath)

	// The HTML should contain rendered content.
	htmlContent, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	assert.Contains(t, string(htmlContent), "My Notes")
}
