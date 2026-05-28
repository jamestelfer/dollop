package doctorcmd_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/cli/doctorcmd"
	"github.com/jamestelfer/dollop/internal/config"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

type fakeLister struct{ err error }

func (f *fakeLister) ListBucket(_ context.Context, _ string) error { return f.err }

type fakeUploader struct {
	err   error
	calls []string
}

func (f *fakeUploader) PutObject(_ context.Context, _, key, _ string, _ io.Reader, _ ...upload.PutOption) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, key)
	return nil
}

func mockGet(code int, body string) func(context.Context, string) (*http.Response, error) {
	return func(_ context.Context, _ string) (*http.Response, error) {
		return &http.Response{
			StatusCode: code,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
}

func okGet() func(context.Context, string) (*http.Response, error) {
	return mockGet(http.StatusOK, "testid")
}

// compile-time check: fakeUploader and fakeLister satisfy their interfaces
var _ upload.Uploader = (*fakeUploader)(nil)
var _ upload.BucketLister = (*fakeLister)(nil)

func runDoctor(t *testing.T, cfg config.Config, hasKey, hasSecret bool, lister upload.BucketLister, up upload.Uploader, httpGet func(context.Context, string) (*http.Response, error)) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := doctorcmd.New(
		cfg, hasKey, hasSecret,
		up, lister,
		func() (string, error) { return "testid", nil },
		httpGet,
	)
	app := &cli.Command{
		Name:           "dollop",
		Writer:         &outBuf,
		ErrWriter:      &errBuf,
		Commands:       []*cli.Command{&cmd},
		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {},
	}
	err := app.Run(context.Background(), []string{"dollop", "doctor"})
	if err != nil {
		var ec cli.ExitCoder
		if errors.As(err, &ec) {
			return outBuf.String(), errBuf.String(), ec.ExitCode()
		}
		return outBuf.String(), errBuf.String(), 1
	}
	return outBuf.String(), errBuf.String(), 0
}

func fullConfig() config.Config {
	return config.Config{
		Bucket:    "test-bucket",
		AccountID: "test-account",
		BaseURL:   "https://example.com",
	}
}

func TestDoctor_AllOK(t *testing.T) {
	stdout, _, code := runDoctor(t,
		fullConfig(), true, true,
		&fakeLister{},
		&fakeUploader{},
		okGet(),
	)
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "all checks passed")
	assert.Contains(t, stdout, "✓ bucket: test-bucket")
	assert.Contains(t, stdout, "✓ r2-key: set")
	assert.Contains(t, stdout, "✓ bucket accessible")
	assert.Contains(t, stdout, "✓ uploaded flash/1/testid/check.txt")
	assert.Contains(t, stdout, "✓ downloaded https://example.com/flash/1/testid/check.txt")
}

func TestDoctor_MissingBucket(t *testing.T) {
	cfg := fullConfig()
	cfg.Bucket = ""
	stdout, _, code := runDoctor(t, cfg, true, true, &fakeLister{}, &fakeUploader{}, okGet())
	require.NotEqual(t, 0, code)
	assert.Contains(t, stdout, "✗ bucket: not set")
}

func TestDoctor_MissingAccountID(t *testing.T) {
	cfg := fullConfig()
	cfg.AccountID = ""
	stdout, _, code := runDoctor(t, cfg, true, true, &fakeLister{}, &fakeUploader{}, okGet())
	require.NotEqual(t, 0, code)
	assert.Contains(t, stdout, "✗ account-id: not set")
}

func TestDoctor_MissingBaseURL(t *testing.T) {
	cfg := fullConfig()
	cfg.BaseURL = ""
	stdout, _, code := runDoctor(t, cfg, true, true, &fakeLister{}, &fakeUploader{}, okGet())
	require.NotEqual(t, 0, code)
	assert.Contains(t, stdout, "✗ base-url: not set")
}

func TestDoctor_MissingKey(t *testing.T) {
	stdout, _, code := runDoctor(t, fullConfig(), false, true, &fakeLister{}, &fakeUploader{}, okGet())
	require.NotEqual(t, 0, code)
	assert.Contains(t, stdout, "✗ r2-key: not set")
}

func TestDoctor_MissingSecret(t *testing.T) {
	stdout, _, code := runDoctor(t, fullConfig(), true, false, &fakeLister{}, &fakeUploader{}, okGet())
	require.NotEqual(t, 0, code)
	assert.Contains(t, stdout, "✗ r2-secret: not set")
}

func TestDoctor_MissingConfig_ShowsAllFailures(t *testing.T) {
	stdout, _, code := runDoctor(t, config.Config{}, false, false, &fakeLister{}, &fakeUploader{}, okGet())
	require.NotEqual(t, 0, code)
	assert.Contains(t, stdout, "✗ bucket: not set")
	assert.Contains(t, stdout, "✗ account-id: not set")
	assert.Contains(t, stdout, "✗ base-url: not set")
	assert.Contains(t, stdout, "✗ r2-key: not set")
	assert.Contains(t, stdout, "✗ r2-secret: not set")
}

func TestDoctor_BucketListError(t *testing.T) {
	stdout, _, code := runDoctor(t,
		fullConfig(), true, true,
		&fakeLister{err: errors.New("connection refused")},
		&fakeUploader{},
		okGet(),
	)
	require.NotEqual(t, 0, code)
	assert.Contains(t, stdout, "✗")
	assert.Contains(t, stdout, "bucket")
}

func TestDoctor_UploadError(t *testing.T) {
	stdout, _, code := runDoctor(t,
		fullConfig(), true, true,
		&fakeLister{},
		&fakeUploader{err: errors.New("put failed")},
		okGet(),
	)
	require.NotEqual(t, 0, code)
	assert.Contains(t, stdout, "✗")
}

func TestDoctor_DownloadNon200(t *testing.T) {
	stdout, _, code := runDoctor(t,
		fullConfig(), true, true,
		&fakeLister{},
		&fakeUploader{},
		mockGet(http.StatusNotFound, ""),
	)
	require.NotEqual(t, 0, code)
	assert.Contains(t, stdout, "✗")
}

func TestDoctor_DownloadContentMismatch(t *testing.T) {
	stdout, _, code := runDoctor(t,
		fullConfig(), true, true,
		&fakeLister{},
		&fakeUploader{},
		mockGet(http.StatusOK, "wrong-content"),
	)
	require.NotEqual(t, 0, code)
	assert.Contains(t, stdout, "✗")
}
