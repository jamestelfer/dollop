package mcphandler_test

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/jamestelfer/dollop/internal/mcphandler"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateInput(t *testing.T) {
	validContent := base64.StdEncoding.EncodeToString([]byte("hello"))

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
			content:   validContent,
			wantErr:   true,
			errSubstr: "path separator",
		},
		{
			name:      "path separator backslash",
			fileName:  "dir\\file.md",
			content:   validContent,
			wantErr:   true,
			errSubstr: "path separator",
		},
		{
			name:      "dot filename",
			fileName:  ".",
			content:   validContent,
			wantErr:   true,
			errSubstr: "invalid filename",
		},
		{
			name:      "dotdot filename",
			fileName:  "..",
			content:   validContent,
			wantErr:   true,
			errSubstr: "invalid filename",
		},
		{
			name:      "parent traversal",
			fileName:  "../etc/passwd",
			content:   validContent,
			wantErr:   true,
			errSubstr: "path separator",
		},
		{
			name:      "unsupported extension png",
			fileName:  "photo.png",
			content:   validContent,
			wantErr:   true,
			errSubstr: ".md",
		},
		{
			name:      "unsupported extension png mentions html",
			fileName:  "photo.png",
			content:   validContent,
			wantErr:   true,
			errSubstr: ".html",
		},
		{
			name:      "no extension",
			fileName:  "readme",
			content:   validContent,
			wantErr:   true,
			errSubstr: "unsupported file extension",
		},
		{
			name:      "invalid base64",
			fileName:  "doc.md",
			content:   "not-valid-base64!!!",
			wantErr:   true,
			errSubstr: "base64",
		},
		{
			name:      "empty decoded content",
			fileName:  "doc.md",
			content:   base64.StdEncoding.EncodeToString([]byte{}),
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:     "valid markdown",
			fileName: "doc.md",
			content:  validContent,
			wantErr:  false,
		},
		{
			name:     "valid html",
			fileName: "page.html",
			content:  validContent,
			wantErr:  false,
		},
		{
			name:     "uppercase extension accepted",
			fileName: "DOC.MD",
			content:  validContent,
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
		"content": base64.StdEncoding.EncodeToString([]byte("data")),
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
		"content": base64.StdEncoding.EncodeToString([]byte("data")),
	}))
	require.NoError(t, err)
	require.True(t, result.IsError)
}

func TestHandler_PathSeparator_ReturnsError(t *testing.T) {
	handler := mcphandler.Handler(nil)
	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "dir/file.md",
		"content": base64.StdEncoding.EncodeToString([]byte("data")),
	}))
	require.NoError(t, err)
	require.True(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "path separator")
}

func TestHandler_InvalidBase64_ReturnsError(t *testing.T) {
	handler := mcphandler.Handler(nil)
	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "doc.md",
		"content": "not-valid!!!",
	}))
	require.NoError(t, err)
	require.True(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "base64")
}

func TestHandler_EmptyContent_ReturnsError(t *testing.T) {
	handler := mcphandler.Handler(nil)
	result, err := handler(context.Background(), makeToolRequest(map[string]any{
		"name":    "doc.md",
		"content": base64.StdEncoding.EncodeToString([]byte{}),
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
		"content": base64.StdEncoding.EncodeToString([]byte("# Hello")),
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
		"content": base64.StdEncoding.EncodeToString([]byte("<html>")),
	}))
	require.NoError(t, err)
	assert.False(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "page.html")
}
