package mcphandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jamestelfer/dollop/internal/render"
	"github.com/jamestelfer/dollop/internal/upload"
)

// NewUploadFunc creates an UploadFunc that materialises the decoded content
// into a temp directory, calls upload.UploadFiles, and returns JSON with the
// public URL(s). The temp directory is cleaned up after every call.
func NewUploadFunc(
	uploader upload.Uploader,
	bucket string,
	baseURL string,
	newID func() (string, error),
) UploadFunc {
	return func(ctx context.Context, name string, decoded []byte) (string, error) {
		id, err := newID()
		if err != nil {
			return "", fmt.Errorf("generate id: %w", err)
		}
		prefix := upload.EphemeralPrefix(7, id)

		// Create temp directory and write the file.
		tmpDir, err := os.MkdirTemp("", "dollop-mcp-*")
		if err != nil {
			return "", fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir) //nolint:errcheck

		filePath := filepath.Join(tmpDir, name)
		if err := os.WriteFile(filePath, decoded, 0o600); err != nil {
			return "", fmt.Errorf("write temp file: %w", err)
		}

		// Choose upload options based on extension.
		ext := strings.ToLower(filepath.Ext(name))
		var opts []upload.UploadOption
		if ext == ".md" {
			opts = append(opts, upload.WithRenderer(render.NewMarkdownRenderer()))
		}

		result, err := upload.UploadFiles(ctx, uploader, bucket, prefix, filePath, false, io.Discard, opts...)
		if err != nil {
			return "", err
		}
		_ = result // result.SourceRelPaths used implicitly: URL is built from name directly

		// Build the response JSON.
		if ext == ".md" {
			// Markdown produces both .md and .html. Find the html path.
			htmlName := strings.TrimSuffix(name, filepath.Ext(name)) + ".html"
			htmlURL := upload.PublicURL(baseURL, prefix, htmlName)
			mdURL := upload.PublicURL(baseURL, prefix, name)
			return marshalJSON(map[string]string{
				"html_url":     htmlURL,
				"markdown_url": mdURL,
			})
		}

		// HTML: single URL.
		url := upload.PublicURL(baseURL, prefix, name)
		return marshalJSON(map[string]string{
			"url": url,
		})
	}
}

func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal response: %w", err)
	}
	return string(b), nil
}
