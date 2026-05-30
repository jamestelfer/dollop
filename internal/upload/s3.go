package upload

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Uploader implements Uploader against Cloudflare R2 via the S3-compatible API.
type S3Uploader struct {
	client *s3.Client
}

// NewS3Uploader returns an Uploader pointed at the R2 endpoint for accountID.
func NewS3Uploader(accountID, accessKey, secretKey string) (*S3Uploader, error) {
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       "auto",
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		UsePathStyle: true,
	})
	return &S3Uploader{client: client}, nil
}

func (u *S3Uploader) PutObject(ctx context.Context, bucket, key, contentType string, body io.Reader, opts ...PutOption) error {
	var o PutOptions
	for _, opt := range opts {
		opt(&o)
	}
	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
		Body:        body,
	}
	if o.CacheControl != "" {
		input.CacheControl = aws.String(o.CacheControl)
	}
	if _, err := u.client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("failed to upload %s to bucket %s: %w", key, bucket, err)
	}
	return nil
}

// ListObjects returns the full keys of every object under prefix, paginating
// through all ListObjectsV2 result pages. The prefix is scoped to a path
// segment by appending a trailing slash, so siblings sharing a name stem are
// not matched.
func (u *S3Uploader) ListObjects(ctx context.Context, bucket, prefix string) ([]string, error) {
	listPrefix := strings.TrimRight(prefix, "/") + "/"
	paginator := s3.NewListObjectsV2Paginator(u.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(listPrefix),
	})
	var keys []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects under %s in bucket %s: %w", listPrefix, bucket, err)
		}
		for _, obj := range page.Contents {
			keys = append(keys, aws.ToString(obj.Key))
		}
	}
	return keys, nil
}

// CopyObject touches key in bucket by self-copying it in place with
// MetadataDirective COPY, preserving its content and metadata while resetting
// its modification time (and thus the R2 lifecycle expiry clock). A successful
// CopyObject response is the authoritative signal that the object was rewritten;
// the Last-Modified timestamp is deliberately not re-read (R2 reports it at
// 1-second resolution, so two touches within the same second are
// indistinguishable).
func (u *S3Uploader) CopyObject(ctx context.Context, bucket, key string) error {
	input := &s3.CopyObjectInput{
		Bucket:            aws.String(bucket),
		Key:               aws.String(key),
		CopySource:        aws.String(copySource(bucket, key)),
		MetadataDirective: types.MetadataDirectiveCopy,
	}
	if _, err := u.client.CopyObject(ctx, input); err != nil {
		return fmt.Errorf("failed to touch %s in bucket %s: %w", key, bucket, err)
	}
	return nil
}

// copySource builds the x-amz-copy-source header value for a self-copy: the
// bucket and key joined by a slash, with the key URL-encoded so reserved and
// special characters survive transport. Path separators within the key are
// preserved.
func copySource(bucket, key string) string {
	return bucket + "/" + (&url.URL{Path: key}).EscapedPath()
}

// ListBucket verifies that bucket is accessible by listing at most one object.
func (u *S3Uploader) ListBucket(ctx context.Context, bucket string) error {
	maxKeys := int32(1)
	_, err := u.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: &maxKeys,
	})
	if err != nil {
		return fmt.Errorf("failed to list files in bucket %s: %w", bucket, err)
	}
	return nil
}
