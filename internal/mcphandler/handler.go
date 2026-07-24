package mcphandler

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// UploadFunc is the signature for the function that performs the actual upload.
// It receives the decoded file content, filename, and returns the tool result
// text (JSON) or an error.
type UploadFunc func(ctx context.Context, name string, decoded []byte) (string, error)

// Tool returns the MCP tool definition for create_upload.
func Tool() mcp.Tool {
	return mcp.NewTool("create_upload",
		mcp.WithDescription("Upload a file and return its public URL. Accepts HTML or Markdown files as base64-encoded content."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Bare filename (no path separators). Must have .md or .html extension."),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Base64-encoded file content."),
		),
	)
}

// Handler returns an MCP tool handler that validates input and delegates to
// uploadFn for the actual upload. If uploadFn is nil, a stub success is returned.
func Handler(uploadFn UploadFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nameVal, ok := args["name"]
		if !ok {
			return mcp.NewToolResultError("missing required parameter: name"), nil
		}
		name, ok := nameVal.(string)
		if !ok {
			return mcp.NewToolResultError("parameter 'name' must be a string"), nil
		}
		contentVal, ok := args["content"]
		if !ok {
			return mcp.NewToolResultError("missing required parameter: content"), nil
		}
		content, ok := contentVal.(string)
		if !ok {
			return mcp.NewToolResultError("parameter 'content' must be a string"), nil
		}

		if err := ValidateInput(name, content); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		decoded, _ := base64.StdEncoding.DecodeString(content) // safe: ValidateInput passed

		if uploadFn != nil {
			result, err := uploadFn(ctx, name, decoded)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(result), nil
		}

		// Stub success for Phase 2 (no upload wired yet).
		return mcp.NewToolResultText(fmt.Sprintf(`{"status":"ok","name":%q}`, name)), nil
	}
}

// RegisterTool adds the create_upload tool to the MCP server.
func RegisterTool(s *server.MCPServer, uploadFn UploadFunc) {
	s.AddTool(Tool(), Handler(uploadFn))
}

// ValidateInput checks the name and content parameters for a create_upload
// tool call. It validates in order: filename → extension → base64 decode →
// empty check. The first failure wins.
func ValidateInput(name, content string) error {
	// Reject path separators.
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid filename %q: must not contain path separators", name)
	}

	// Reject . and ..
	if name == "." || name == ".." {
		return fmt.Errorf("invalid filename %q", name)
	}

	// Reject unsupported extensions.
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".md" && ext != ".html" {
		return fmt.Errorf("unsupported file extension %q: allowed extensions are .md and .html", ext)
	}

	// Decode base64.
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return fmt.Errorf("invalid base64 content: %w", err)
	}

	// Reject empty content.
	if len(decoded) == 0 {
		return fmt.Errorf("content must not be empty after decoding")
	}

	return nil
}
