package upload

import (
	"mime"
	"path/filepath"
	"strings"
)

// ContentType returns the MIME type for filename based on its extension.
// Falls back to application/octet-stream for unknown extensions.
func ContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return "application/octet-stream"
	}
	// text/markdown is not rendered by browsers; use text/plain so content is
	// displayed inline rather than downloaded.
	if ext == ".md" || ext == ".markdown" {
		return "text/plain; charset=utf-8"
	}
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		return "application/octet-stream"
	}
	return ct
}
