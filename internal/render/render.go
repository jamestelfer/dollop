package render

// Renderer converts .md files to .html within a source directory and returns
// the augmented path list (original paths plus generated .html paths).
// SharedAssets returns the CSS/JS assets that must be uploaded at the prefix
// root, deduped across a batch.
type Renderer interface {
	Render(relPaths []string, sourceDir string) ([]string, error)
	SharedAssets() []SharedAsset
}

// NewNoopRenderer returns a Renderer that performs no conversion.
func NewNoopRenderer() Renderer {
	return &noopRenderer{}
}

type noopRenderer struct{}

func (n *noopRenderer) Render(relPaths []string, _ string) ([]string, error) {
	return relPaths, nil
}

func (n *noopRenderer) SharedAssets() []SharedAsset { return nil }
