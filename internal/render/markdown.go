package render

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
)

// NewMarkdownRenderer returns a Renderer that converts .md files to .html,
// discarding any warnings.
func NewMarkdownRenderer() Renderer {
	return &markdownRenderer{stderr: io.Discard}
}

// NewMarkdownRendererWithStderr returns a Renderer that writes collision
// warnings to stderr.
func NewMarkdownRendererWithStderr(stderr io.Writer) Renderer {
	return &markdownRenderer{stderr: stderr}
}

type markdownRenderer struct {
	stderr io.Writer
}

func (m *markdownRenderer) Render(relPaths []string, sourceDir string) ([]string, error) {
	// build a set of paths already in the list so we can detect collisions
	existing := make(map[string]bool, len(relPaths))
	for _, p := range relPaths {
		existing[p] = true
	}

	result := make([]string, 0, len(relPaths))
	for _, p := range relPaths {
		result = append(result, p)
		if !isMarkdown(p) {
			continue
		}
		stem := strings.TrimSuffix(p, filepath.Ext(p))
		htmlRel := stem + ".html"

		if existing[htmlRel] {
			fmt.Fprintf(m.stderr, "%s already present, skipping rendering of %s\n", htmlRel, p) //nolint:errcheck
			continue
		}

		generated, err := renderMarkdownFile(p, sourceDir)
		if err != nil {
			return nil, err
		}
		result = append(result, generated)
	}
	return result, nil
}

func isMarkdown(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".md" || ext == ".markdown"
}

func renderMarkdownFile(relPath, sourceDir string) (string, error) {
	src, err := os.ReadFile(filepath.Join(sourceDir, relPath)) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("read %s: %w", relPath, err)
	}

	var buf bytes.Buffer
	if err := goldmark.Convert(src, &buf); err != nil {
		return "", fmt.Errorf("render %s: %w", relPath, err)
	}

	stem := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	htmlRel := stem + ".html"
	if err := os.WriteFile(filepath.Join(sourceDir, htmlRel), buf.Bytes(), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", htmlRel, err)
	}
	return htmlRel, nil
}
