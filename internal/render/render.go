package render

import (
	"io"
	"io/fs"
)

// Source represents a single file to be uploaded. Open is called lazily when
// the file is ready to be read. ContentType is empty when the upload layer
// should detect it from the file extension. Size is -1 when the final size is
// unknown until Open is called (e.g. dynamically rendered content).
//
// Open returns an io.ReadSeekCloser so the S3 SDK can seek to determine
// Content-Length without buffering the entire body.
type Source struct {
	RelPath     string
	ContentType string
	Size        int64
	Open        func() (io.ReadSeekCloser, error)
}

// Renderer plans the set of Sources to upload from a list of relative paths
// within fsys and returns any shared assets that must be uploaded once at the
// prefix root. Sources read from fsys lazily, so it must remain usable until
// every Source has been opened.
// prefix is the R2 key prefix the files will be published under (e.g.
// flash/1/<id> or keep/<name>); it is used to compute relative references that
// climb out of the prefix to shared, bucket-rooted deps.
type Renderer interface {
	Plan(relPaths []string, fsys fs.FS, prefix string) ([]Source, []SharedAsset, error)
}

// NewDiskRenderer returns a Renderer that serves files directly from disk
// without any conversion.
func NewDiskRenderer() Renderer {
	return &diskRenderer{}
}

type diskRenderer struct{}

func (d *diskRenderer) Plan(relPaths []string, fsys fs.FS, _ string) ([]Source, []SharedAsset, error) {
	sources := make([]Source, 0, len(relPaths))
	for _, p := range relPaths {
		sources = append(sources, diskSource(p, fsys))
	}
	return sources, nil, nil
}
