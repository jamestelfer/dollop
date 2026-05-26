package upload_test

import "github.com/jamestelfer/dollop/internal/upload"

// Compile-time assertion: *S3Uploader must implement BucketLister.
var _ upload.BucketLister = (*upload.S3Uploader)(nil)
