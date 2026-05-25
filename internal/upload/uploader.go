package upload

import (
	"context"
	"io"
)

// Uploader puts a single object into a bucket.
type Uploader interface {
	PutObject(ctx context.Context, bucket, key, contentType string, body io.Reader) error
}
