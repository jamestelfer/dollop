package upload

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// s3Against returns an S3Uploader whose requests go to handler.
func s3Against(t *testing.T, handler http.HandlerFunc) *S3Uploader {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &S3Uploader{client: s3.New(s3.Options{
		BaseEndpoint: aws.String(srv.URL),
		Region:       "auto",
		Credentials:  credentials.NewStaticCredentialsProvider("k", "s", ""),
		UsePathStyle: true,
	})}
}

func TestS3Uploader_ReadObjectHead_SendsRange(t *testing.T) {
	var gotPath, gotRange string
	up := s3Against(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotRange = r.URL.Path, r.Header.Get("Range")
		w.Header().Set("Content-Range", "bytes 0-4/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("<!DOC"))
	})

	got, err := up.ReadObjectHead(context.Background(), "bkt", "keep/x/index.html", 512)
	require.NoError(t, err)
	assert.Equal(t, "<!DOC", string(got))
	assert.Equal(t, "/bkt/keep/x/index.html", gotPath)
	assert.Equal(t, "bytes=0-511", gotRange)
}

func TestS3Uploader_ReadObjectHead_CapsOversizedBody(t *testing.T) {
	up := s3Against(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("0123456789")) // a server ignoring Range
	})

	got, err := up.ReadObjectHead(context.Background(), "bkt", "k", 4)
	require.NoError(t, err)
	assert.Equal(t, "0123", string(got))
}

func TestS3Uploader_ReadObjectHead_MissingKey(t *testing.T) {
	up := s3Against(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`))
	})

	_, err := up.ReadObjectHead(context.Background(), "bkt", "k", 512)
	assert.ErrorIs(t, err, ErrObjectNotFound)
}
