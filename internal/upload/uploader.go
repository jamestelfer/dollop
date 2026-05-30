package upload

import (
	"context"
	"io"
)

// PutOptions holds optional parameters for a PutObject call.
type PutOptions struct {
	CacheControl string // empty means no Cache-Control header is set
}

// PutOption configures optional fields on a PutObject call.
type PutOption func(*PutOptions)

// WithCacheControl sets the Cache-Control header on the uploaded object.
func WithCacheControl(value string) PutOption {
	return func(o *PutOptions) {
		o.CacheControl = value
	}
}

// Uploader puts a single object into a bucket.
type Uploader interface {
	PutObject(ctx context.Context, bucket, key, contentType string, body io.Reader, opts ...PutOption) error
}

// BucketLister checks that a bucket is accessible.
type BucketLister interface {
	ListBucket(ctx context.Context, bucket string) error
}

// ObjectLister lists the full keys of every object stored under a prefix.
// The prefix is matched as a path segment (a trailing slash is implied), so
// listing "flash/7/abc" returns keys under "flash/7/abc/" and never matches a
// sibling such as "flash/7/abcdef/...". Keys are returned in lexical order.
type ObjectLister interface {
	ListObjects(ctx context.Context, bucket, prefix string) ([]string, error)
}

// ObjectCopier touches a single object by self-copying it in place: the source
// and destination key are the same, content and metadata are preserved, and the
// object's modification time is reset. On R2 this resets the lifecycle expiry
// clock. Success is determined from the copy response, not a Last-Modified diff
// (R2's Last-Modified has 1-second resolution, so two touches within the same
// second are indistinguishable).
type ObjectCopier interface {
	CopyObject(ctx context.Context, bucket, key string) error
}
