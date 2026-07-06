package depscmd_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamestelfer/dollop/internal/cli/depscmd"
	"github.com/jamestelfer/dollop/internal/deps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// fixtureTarball builds a minimal mermaid-shaped gzipped tarball and its
// npm-style integrity string.
func fixtureTarball(t *testing.T) (tgz []byte, integrity string) {
	t.Helper()
	entries := map[string]string{
		"package/dist/mermaid.esm.min.mjs":                    "import './chunks/mermaid.esm.min/chunk-A.mjs';\n",
		"package/dist/chunks/mermaid.esm.min/chunk-A.mjs":     "export const a = 1;\n",
		"package/dist/chunks/mermaid.esm.min/chunk-A.mjs.map": "MAP",
		"package/README.md":                                   "readme",
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	tgz = buf.Bytes()
	sum := sha512.Sum512(tgz)
	return tgz, "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

func fetchFixture(tgz []byte) deps.FetchFunc {
	return func(_ context.Context, _ string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(tgz)), nil
	}
}

func run(t *testing.T, integrity string, fetch deps.FetchFunc, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := depscmd.New(nil, nil, "test-bucket", "11.16.0", integrity, fetch)
	app := &cli.Command{
		Name:           "dollop",
		Writer:         &outBuf,
		ErrWriter:      &errBuf,
		Commands:       []*cli.Command{&cmd},
		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {},
	}
	err := app.Run(context.Background(), append([]string{"dollop"}, args...))
	if err != nil {
		var ec cli.ExitCoder
		if errors.As(err, &ec) {
			return outBuf.String(), errBuf.String(), ec.ExitCode()
		}
		return outBuf.String(), errBuf.String(), 1
	}
	return outBuf.String(), errBuf.String(), 0
}

func TestPublish_CopyDir_WritesESMTree(t *testing.T) {
	tgz, integrity := fixtureTarball(t)
	dst := t.TempDir()

	stdout, _, code := run(t, integrity, fetchFixture(tgz), "deps", "publish", "--copy-dir", dst)
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "published mermaid 11.16.0")

	entry, err := os.ReadFile(filepath.Join(dst, "deps", "mermaid", "11.16.0", "mermaid.esm.min.mjs"))
	require.NoError(t, err)
	assert.Equal(t, "import './chunks/mermaid.esm.min/chunk-A.mjs';\n", string(entry))

	_, err = os.ReadFile(filepath.Join(dst, "deps", "mermaid", "11.16.0", "chunks", "mermaid.esm.min", "chunk-A.mjs"))
	require.NoError(t, err)

	// source maps must not be uploaded
	_, err = os.Stat(filepath.Join(dst, "deps", "mermaid", "11.16.0", "chunks", "mermaid.esm.min", "chunk-A.mjs.map"))
	assert.True(t, os.IsNotExist(err))
}

func TestPublish_SecondRunSkips(t *testing.T) {
	tgz, integrity := fixtureTarball(t)
	dst := t.TempDir()

	_, _, code := run(t, integrity, fetchFixture(tgz), "deps", "publish", "--copy-dir", dst)
	require.Equal(t, 0, code)

	stdout, stderr, code := run(t, integrity, fetchFixture(tgz), "deps", "publish", "--copy-dir", dst)
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "already present")
	assert.Contains(t, stderr, "skipping")
}

func TestPublish_ForceReuploads(t *testing.T) {
	tgz, integrity := fixtureTarball(t)
	dst := t.TempDir()

	_, _, code := run(t, integrity, fetchFixture(tgz), "deps", "publish", "--copy-dir", dst)
	require.Equal(t, 0, code)

	stdout, _, code := run(t, integrity, fetchFixture(tgz), "deps", "publish", "--force", "--copy-dir", dst)
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "published mermaid 11.16.0")
}

func TestPublish_IntegrityMismatchFails(t *testing.T) {
	tgz, _ := fixtureTarball(t)
	dst := t.TempDir()
	bad := "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, 64))

	_, stderr, code := run(t, bad, fetchFixture(tgz), "deps", "publish", "--copy-dir", dst)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "error:")

	_, err := os.Stat(filepath.Join(dst, "deps"))
	assert.True(t, os.IsNotExist(err), "no files should be written on integrity mismatch")
}

func TestStatus_ReportsAbsentThenPresent(t *testing.T) {
	tgz, integrity := fixtureTarball(t)
	dst := t.TempDir()

	stdout, _, code := run(t, integrity, fetchFixture(tgz), "deps", "status", "--copy-dir", dst)
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "mermaid version: 11.16.0")
	assert.Contains(t, stdout, "absent")

	_, _, code = run(t, integrity, fetchFixture(tgz), "deps", "publish", "--copy-dir", dst)
	require.Equal(t, 0, code)

	stdout, _, code = run(t, integrity, fetchFixture(tgz), "deps", "status", "--copy-dir", dst)
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "present")
}
