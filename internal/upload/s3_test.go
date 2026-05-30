package upload_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time assertions: *S3Uploader implements the listing interfaces, and
// *DirUploader provides an equivalent ObjectLister for integration use.
var (
	_ upload.BucketLister = (*upload.S3Uploader)(nil)
	_ upload.ObjectLister = (*upload.S3Uploader)(nil)
	_ upload.ObjectLister = (*upload.DirUploader)(nil)
	_ upload.ObjectCopier = (*upload.S3Uploader)(nil)
	_ upload.ObjectCopier = (*upload.DirUploader)(nil)
)

// fakeUploaderWithOptions records PutObject calls including any options.
type fakeUploaderWithOptions struct {
	calls []putCallWithOptions
	err   error
}

type putCallWithOptions struct {
	key          string
	cacheControl string // empty if not set
}

func (f *fakeUploaderWithOptions) PutObject(_ context.Context, _, key, _ string, _ interface{ Read([]byte) (int, error) }, opts ...upload.PutOption) error {
	if f.err != nil {
		return f.err
	}
	var o upload.PutOptions
	for _, opt := range opts {
		opt(&o)
	}
	f.calls = append(f.calls, putCallWithOptions{key: key, cacheControl: o.CacheControl})
	return nil
}

func TestWithCacheControl_SetsHeader(t *testing.T) {
	up := &fakeUploaderWithOptions{}
	opt := upload.WithCacheControl("max-age=604800")

	err := up.PutObject(context.Background(), "b", "k", "text/css", bytes.NewReader(nil), opt)
	require.NoError(t, err)
	require.Len(t, up.calls, 1)
	assert.Equal(t, "max-age=604800", up.calls[0].cacheControl)
}

func TestWithCacheControl_AbsentWhenNoOption(t *testing.T) {
	up := &fakeUploaderWithOptions{}

	err := up.PutObject(context.Background(), "b", "k", "text/plain", bytes.NewReader(nil))
	require.NoError(t, err)
	require.Len(t, up.calls, 1)
	assert.Empty(t, up.calls[0].cacheControl)
}
