package upload

import (
	"context"
	"io"
)

// Uploader puts a single object into a bucket.
type Uploader interface {
	PutObject(ctx context.Context, bucket, key, contentType string, body io.Reader) error
}

// BucketLister checks that a bucket is accessible.
type BucketLister interface {
	ListBucket(ctx context.Context, bucket string) error
}
