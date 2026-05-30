package upload

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
