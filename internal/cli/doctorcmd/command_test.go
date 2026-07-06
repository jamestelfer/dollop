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
	"github.com/jamestelfer/dollop/internal/render"
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

// fakeObjLister reports the object keys present under a prefix, used for the
// shared mermaid deps presence check.
type fakeObjLister struct {
	keys []string
	err  error
}

func (f *fakeObjLister) ListObjects(_ context.Context, _, _ string) ([]string, error) {
	return f.keys, f.err
}

// presentDepsLister reports the shipped mermaid version as published.
func presentDepsLister() *fakeObjLister {
	return &fakeObjLister{keys: []string{"deps/mermaid/" + render.MermaidVersion + "/mermaid.esm.min.mjs"}}
}

// compile-time check: fakeUploader and fakeLister satisfy their interfaces
var _ upload.Uploader = (*fakeUploader)(nil)
var _ upload.BucketLister = (*fakeLister)(nil)
var _ upload.ObjectLister = (*fakeObjLister)(nil)

// doctorOpts captures the configuration inputs that most tests leave at their
// defaults. Tests that care about a field override it before calling runDoctor.
type doctorOpts struct {
	configPath    string
	authPath      string
	secureStorage bool
}

func defaultOpts() doctorOpts {
	return doctorOpts{
		configPath:    "/etc/dollop/config.yaml",
		authPath:      "/etc/dollop/auth.yaml",
		secureStorage: true,
	}
}

func runDoctor(t *testing.T, cfg config.Config, hasKey, hasSecret bool, lister upload.BucketLister, up upload.Uploader, httpGet func(context.Context, string) (*http.Response, error)) (stdout, stderr string, code int) {
	t.Helper()
	return runDoctorDeps(t, defaultOpts(), cfg, hasKey, hasSecret, lister, up, presentDepsLister(), httpGet)
}

func runDoctorOpts(t *testing.T, opts doctorOpts, cfg config.Config, hasKey, hasSecret bool, lister upload.BucketLister, up upload.Uploader, httpGet func(context.Context, string) (*http.Response, error)) (stdout, stderr string, code int) {
	t.Helper()
	return runDoctorDeps(t, opts, cfg, hasKey, hasSecret, lister, up, presentDepsLister(), httpGet)
}

func runDoctorDeps(t *testing.T, opts doctorOpts, cfg config.Config, hasKey, hasSecret bool, lister upload.BucketLister, up upload.Uploader, objLister upload.ObjectLister, httpGet func(context.Context, string) (*http.Response, error)) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := doctorcmd.New(
		cfg, opts.configPath, opts.authPath, opts.secureStorage,
		hasKey, hasSecret,
		up, lister, objLister, render.MermaidVersion,
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

func TestDoctor_MermaidDepsPublished(t *testing.T) {
	stdout, _, code := runDoctor(t, fullConfig(), true, true, &fakeLister{}, &fakeUploader{}, okGet())
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "✓ mermaid "+render.MermaidVersion+": published")
}

func TestDoctor_MermaidDepsNotPublished(t *testing.T) {
	stdout, _, code := runDoctorDeps(t, defaultOpts(), fullConfig(), true, true,
		&fakeLister{}, &fakeUploader{}, &fakeObjLister{}, okGet())
	// missing deps are informational, not a failure
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "mermaid "+render.MermaidVersion+": not published")
	assert.Contains(t, stdout, "dollop deps publish")
	assert.Contains(t, stdout, "all checks passed")
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

func TestDoctor_GroupHeaders(t *testing.T) {
	stdout, _, code := runDoctor(t,
		fullConfig(), true, true,
		&fakeLister{},
		&fakeUploader{},
		okGet(),
	)
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "config:")
	assert.Contains(t, stdout, "auth:")
	assert.Contains(t, stdout, "roundtrip:")

	// groups must appear in order: config, then auth, then roundtrip
	assert.Less(t, strings.Index(stdout, "config:"), strings.Index(stdout, "auth:"))
	assert.Less(t, strings.Index(stdout, "auth:"), strings.Index(stdout, "roundtrip:"))
}

func TestDoctor_ConfigGroupShowsConfigPath(t *testing.T) {
	opts := defaultOpts()
	opts.configPath = "/etc/dollop/config.yaml"
	stdout, _, code := runDoctorOpts(t, opts, fullConfig(), true, true, &fakeLister{}, &fakeUploader{}, okGet())
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "ℹ config file: /etc/dollop/config.yaml")
}

func TestDoctor_SecureStorage_ShowsOSSecretsStorage(t *testing.T) {
	opts := defaultOpts()
	opts.secureStorage = true
	stdout, _, code := runDoctorOpts(t, opts, fullConfig(), true, true, &fakeLister{}, &fakeUploader{}, okGet())
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "ℹ stored in OS secrets storage")
	assert.NotContains(t, stdout, "no secure secret storage available")
}

func TestDoctor_PlaintextStorage_ShowsWarningWithPath(t *testing.T) {
	opts := defaultOpts()
	opts.secureStorage = false
	opts.authPath = "/etc/dollop/auth.yaml"
	stdout, _, code := runDoctorOpts(t, opts, fullConfig(), true, true, &fakeLister{}, &fakeUploader{}, okGet())
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "⚠ no secure secret storage available, stored in /etc/dollop/auth.yaml")
	assert.NotContains(t, stdout, "stored in OS secrets storage")
}

func TestDoctor_HomeDirPathsCollapseToTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	opts := defaultOpts()
	opts.configPath = home + "/.config/dollop/config.yaml"
	opts.authPath = home + "/.config/dollop/auth.yaml"
	opts.secureStorage = false

	stdout, _, code := runDoctorOpts(t, opts, fullConfig(), true, true, &fakeLister{}, &fakeUploader{}, okGet())
	require.Equal(t, 0, code)

	assert.Contains(t, stdout, "config file: ~/.config/dollop/config.yaml")
	assert.Contains(t, stdout, "stored in ~/.config/dollop/auth.yaml")
	// the expanded home directory must not leak into the output
	assert.NotContains(t, stdout, home)
}

func TestDoctor_PlaintextWarningShownEvenWhenConfigIncomplete(t *testing.T) {
	// Informational/auth items are emitted before the incomplete-config exit.
	opts := defaultOpts()
	opts.secureStorage = false
	opts.authPath = "/etc/dollop/auth.yaml"
	stdout, _, code := runDoctorOpts(t, opts, config.Config{}, false, false, &fakeLister{}, &fakeUploader{}, okGet())
	require.NotEqual(t, 0, code)
	assert.Contains(t, stdout, "config file:")
	assert.Contains(t, stdout, "⚠ no secure secret storage available, stored in /etc/dollop/auth.yaml")
	// roundtrip must not run when config/auth are incomplete
	assert.NotContains(t, stdout, "roundtrip:")
}
