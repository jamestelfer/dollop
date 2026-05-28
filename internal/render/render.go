package render

// Renderer converts .md files to .html within a source directory and returns
// the augmented path list (original paths plus generated .html paths).
type Renderer interface {
	Render(relPaths []string, sourceDir string) ([]string, error)
}

// NewNoopRenderer returns a Renderer that performs no conversion.
func NewNoopRenderer() Renderer {
	return &noopRenderer{}
}

type noopRenderer struct{}

func (n *noopRenderer) Render(relPaths []string, sourceDir string) ([]string, error) {
	return relPaths, nil
}
