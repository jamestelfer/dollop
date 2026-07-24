package servecmd_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/jamestelfer/dollop/internal/cli/servecmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestReadEnv_AllPresent(t *testing.T) {
	t.Setenv("DOLLOP_ACCOUNT_ID", "acct")
	t.Setenv("DOLLOP_BUCKET", "bucket")
	t.Setenv("DOLLOP_BASE_URL", "https://example.com")
	t.Setenv("DOLLOP_R2_KEY", "key")
	t.Setenv("DOLLOP_R2_SECRET", "secret")
	t.Setenv("DOLLOP_ADDR", ":9090")

	cfg, missing := servecmd.ReadEnv()
	require.Empty(t, missing)
	assert.Equal(t, "acct", cfg.AccountID)
	assert.Equal(t, "bucket", cfg.Bucket)
	assert.Equal(t, "https://example.com", cfg.BaseURL)
	assert.Equal(t, "key", cfg.R2Key)
	assert.Equal(t, "secret", cfg.R2Secret)
	assert.Equal(t, ":9090", cfg.Addr)
}

func TestReadEnv_DefaultAddr(t *testing.T) {
	t.Setenv("DOLLOP_ACCOUNT_ID", "acct")
	t.Setenv("DOLLOP_BUCKET", "bucket")
	t.Setenv("DOLLOP_BASE_URL", "https://example.com")
	t.Setenv("DOLLOP_R2_KEY", "key")
	t.Setenv("DOLLOP_R2_SECRET", "secret")
	// DOLLOP_ADDR intentionally unset

	cfg, missing := servecmd.ReadEnv()
	require.Empty(t, missing)
	assert.Equal(t, ":8080", cfg.Addr)
}

func TestReadEnv_MissingAll(t *testing.T) {
	// Clear all env vars in case they're set in the environment
	t.Setenv("DOLLOP_ACCOUNT_ID", "")
	t.Setenv("DOLLOP_BUCKET", "")
	t.Setenv("DOLLOP_BASE_URL", "")
	t.Setenv("DOLLOP_R2_KEY", "")
	t.Setenv("DOLLOP_R2_SECRET", "")

	_, missing := servecmd.ReadEnv()
	assert.ElementsMatch(t, []string{
		"DOLLOP_ACCOUNT_ID",
		"DOLLOP_BUCKET",
		"DOLLOP_BASE_URL",
		"DOLLOP_R2_KEY",
		"DOLLOP_R2_SECRET",
	}, missing)
}

func TestReadEnv_MissingSome(t *testing.T) {
	t.Setenv("DOLLOP_ACCOUNT_ID", "acct")
	t.Setenv("DOLLOP_BUCKET", "")
	t.Setenv("DOLLOP_BASE_URL", "https://example.com")
	t.Setenv("DOLLOP_R2_KEY", "")
	t.Setenv("DOLLOP_R2_SECRET", "secret")

	_, missing := servecmd.ReadEnv()
	assert.ElementsMatch(t, []string{"DOLLOP_BUCKET", "DOLLOP_R2_KEY"}, missing)
}

// runServe runs the serve command with a custom readEnv and returns stdout, stderr, exit code, and the error message.
func runServe(t *testing.T, readEnv func() (servecmd.EnvConfig, []string), args ...string) (stdout, stderr, errMsg string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := servecmd.New(readEnv, "test")
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
			return outBuf.String(), errBuf.String(), err.Error(), ec.ExitCode()
		}
		return outBuf.String(), errBuf.String(), err.Error(), 1
	}
	return outBuf.String(), errBuf.String(), "", 0
}

func TestServe_MissingEnvVars_ExitsNonZero(t *testing.T) {
	readEnv := func() (servecmd.EnvConfig, []string) {
		return servecmd.EnvConfig{}, []string{"DOLLOP_BUCKET", "DOLLOP_R2_KEY"}
	}

	_, _, errMsg, code := runServe(t, readEnv, "serve")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, errMsg, "DOLLOP_BUCKET")
	assert.Contains(t, errMsg, "DOLLOP_R2_KEY")
}

func TestServe_AllMissing_ListsAllVars(t *testing.T) {
	readEnv := func() (servecmd.EnvConfig, []string) {
		return servecmd.EnvConfig{}, []string{
			"DOLLOP_ACCOUNT_ID", "DOLLOP_BUCKET", "DOLLOP_BASE_URL",
			"DOLLOP_R2_KEY", "DOLLOP_R2_SECRET",
		}
	}

	_, _, errMsg, code := runServe(t, readEnv, "serve")
	assert.NotEqual(t, 0, code)
	for _, v := range []string{"DOLLOP_ACCOUNT_ID", "DOLLOP_BUCKET", "DOLLOP_BASE_URL", "DOLLOP_R2_KEY", "DOLLOP_R2_SECRET"} {
		assert.Contains(t, errMsg, v)
	}
}
