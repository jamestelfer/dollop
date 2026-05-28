package render_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamestelfer/dollop/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoopRenderer verifies that the no-op renderer returns its input unchanged
// and never errors.
func TestNoopRenderer_PassesThrough(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))

	r := render.NewNoopRenderer()
	got, err := r.Render([]string{"notes.md"}, dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"notes.md"}, got)
}

// TestNoopRenderer_EmptySlice verifies no-op handles an empty input.
func TestNoopRenderer_EmptySlice(t *testing.T) {
	r := render.NewNoopRenderer()
	got, err := r.Render([]string{}, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, []string{}, got)
}

// TestMarkdownRenderer_RendersHTML verifies that a .md file produces a
// corresponding .html file on disk and that both paths are in the result.
func TestMarkdownRenderer_RendersHTML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello\n\nWorld"), 0600))

	r := render.NewMarkdownRenderer()
	got, err := r.Render([]string{"notes.md"}, dir)
	require.NoError(t, err)

	assert.Contains(t, got, "notes.md")
	assert.Contains(t, got, "notes.html")

	// .html file must exist on disk
	htmlPath := filepath.Join(dir, "notes.html")
	_, statErr := os.Stat(htmlPath)
	require.NoError(t, statErr, "notes.html should have been written to disk")

	content, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "<h1>")
}

// TestMarkdownRenderer_NonMarkdownPassedThrough verifies non-.md files are
// included in the result without any additional paths.
func TestMarkdownRenderer_NonMarkdownPassedThrough(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("data"), 0600))

	r := render.NewMarkdownRenderer()
	got, err := r.Render([]string{"photo.jpg"}, dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"photo.jpg"}, got)
}

// TestMarkdownRenderer_CollisionSkipsAndWarns verifies that when a .html file
// with the same stem already exists on disk, the .md file is not rendered and
// a warning is written to stderr.
func TestMarkdownRenderer_CollisionSkipsAndWarns(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.html"), []byte("<h1>existing</h1>"), 0600))

	var stderr bytes.Buffer
	r := render.NewMarkdownRendererWithStderr(&stderr)
	got, err := r.Render([]string{"notes.md", "notes.html"}, dir)
	require.NoError(t, err)

	// both paths still appear (existing html is kept)
	assert.Contains(t, got, "notes.md")
	assert.Contains(t, got, "notes.html")
	// but it was not re-rendered — the existing content is intact
	htmlContent, err := os.ReadFile(filepath.Join(dir, "notes.html"))
	require.NoError(t, err)
	assert.Equal(t, "<h1>existing</h1>", string(htmlContent))

	assert.Contains(t, stderr.String(), "notes.html already present, skipping rendering of notes.md")
}
