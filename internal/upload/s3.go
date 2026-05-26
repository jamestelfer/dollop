package upload

import (
	"context"
	"fmt"
	"io"

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

func (u *S3Uploader) PutObject(ctx context.Context, bucket, key, contentType string, body io.Reader) error {
	_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
		Body:        body,
	})
	return err
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
