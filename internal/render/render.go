package render

import (
	"io"
	"os"
	"path/filepath"
)

// Source represents a single file to be uploaded. Open is called lazily when
// the file is ready to be read. ContentType is empty when the upload layer
// should detect it from the file extension. Size is -1 when the final size is
// unknown until Open is called (e.g. dynamically rendered content).
type Source struct {
	RelPath     string
	ContentType string
	Size        int64
	Open        func() (io.ReadCloser, error)
}

// Renderer plans the set of Sources to upload from a list of relative paths
// and returns any shared assets that must be uploaded once at the prefix root.
type Renderer interface {
	Plan(relPaths []string, sourceDir string) ([]Source, []SharedAsset, error)
}

// NewDiskRenderer returns a Renderer that serves files directly from disk
// without any conversion.
func NewDiskRenderer() Renderer {
	return &diskRenderer{}
}

type diskRenderer struct{}

func (d *diskRenderer) Plan(relPaths []string, sourceDir string) ([]Source, []SharedAsset, error) {
	sources := make([]Source, 0, len(relPaths))
	for _, p := range relPaths {
		absPath := filepath.Join(sourceDir, filepath.FromSlash(p))
		info, err := os.Stat(absPath)
		var sz int64 = -1
		if err == nil {
			sz = info.Size()
		}
		sources = append(sources, Source{
			RelPath: p,
			Size:    sz,
			Open:    func() (io.ReadCloser, error) { return os.Open(absPath) }, //nolint:gosec
		})
	}
	return sources, nil, nil
}
