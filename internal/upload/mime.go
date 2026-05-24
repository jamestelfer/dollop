package upload

import (
	"mime"
	"path/filepath"
)

// ContentType returns the MIME type for filename based on its extension.
// Falls back to application/octet-stream for unknown extensions.
func ContentType(filename string) string {
	ext := filepath.Ext(filename)
	if ext == "" {
		return "application/octet-stream"
	}
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		return "application/octet-stream"
	}
	return ct
}
