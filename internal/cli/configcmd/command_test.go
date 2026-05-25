package configcmd_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/cli/configcmd"
	"github.com/jamestelfer/dollop/internal/config"
	"github.com/urfave/cli/v3"
)

// fakeKeyring is an in-memory KeyringStore for testing.
type fakeKeyring struct {
	data map[string]string
	err  error // if non-nil, all operations return this error
}

func newFakeKeyring() *fakeKeyring { return &fakeKeyring{data: map[string]string{}} }

func (f *fakeKeyring) Get(service, user string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	v, ok := f.data[service+"/"+user]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (f *fakeKeyring) Set(service, user, password string) error {
	if f.err != nil {
		return f.err
	}
	f.data[service+"/"+user] = password
	return nil
}

func (f *fakeKeyring) Delete(service, user string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.data, service+"/"+user)
	return nil
}

// run builds the app wired to a temp config file and runs args.
// Returns stdout, stderr, and the exit code (0 if no error / cli.ExitCoder).
func run(t *testing.T, cfgPath string, kr config.KeyringStore, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	sub := configcmd.New(kr, cfgPath)
	app := &cli.Command{
		Name:      "dollop",
		Writer:    &outBuf,
		ErrWriter: &errBuf,
		Commands:  []*cli.Command{&sub},
		// Prevent os.Exit from being called in tests; errors are returned instead.
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

func TestConfigHelp(t *testing.T) {
	dir := t.TempDir()
	out, _, code := run(t, filepath.Join(dir, "config.yaml"), newFakeKeyring(), "config", "--help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	for _, want := range []string{"set", "get", "list", "auth"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in help output, got:\n%s", want, out)
		}
	}
}

func TestConfigSet_ValidKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	_, _, code := run(t, cfgPath, newFakeKeyring(), "config", "set", "bucket", "my-bucket")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Bucket != "my-bucket" {
		t.Errorf("bucket = %q, want %q", cfg.Bucket, "my-bucket")
	}
}

func TestConfigSet_PrintsFilePath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	stdout, _, code := run(t, cfgPath, newFakeKeyring(), "config", "set", "bucket", "my-bucket")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout, cfgPath) {
		t.Errorf("expected config file path %q in output, got: %q", cfgPath, stdout)
	}
}

func TestConfigSet_InvalidKey(t *testing.T) {
	dir := t.TempDir()
	_, _, code := run(t, filepath.Join(dir, "c.yaml"), newFakeKeyring(), "config", "set", "unknown", "val")
	if code == 0 {
		t.Error("expected non-zero exit for unknown key")
	}
}

func TestConfigSet_WrongArgCount(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"config", "set"},
		{"config", "set", "bucket"},
	} {
		_, _, code := run(t, filepath.Join(dir, "c.yaml"), newFakeKeyring(), args...)
		if code == 0 {
			t.Errorf("args %v: expected non-zero exit", args)
		}
	}
}

func TestConfigGet_ValidKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	run(t, cfgPath, newFakeKeyring(), "config", "set", "bucket", "b1") //nolint

	out, _, code := run(t, cfgPath, newFakeKeyring(), "config", "get", "bucket")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.TrimSpace(out) != "b1" {
		t.Errorf("stdout = %q, want %q", out, "b1")
	}
}

func TestConfigGet_InvalidKey(t *testing.T) {
	dir := t.TempDir()
	_, _, code := run(t, filepath.Join(dir, "c.yaml"), newFakeKeyring(), "config", "get", "unknown")
	if code == 0 {
		t.Error("expected non-zero exit for unknown key")
	}
}

func TestConfigList(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	run(t, cfgPath, newFakeKeyring(), "config", "set", "bucket", "bkt")     //nolint
	run(t, cfgPath, newFakeKeyring(), "config", "set", "account-id", "acc") //nolint

	out, _, code := run(t, cfgPath, newFakeKeyring(), "config", "list")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	for _, want := range []string{"bucket", "account-id", "base-url"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected key %q in list output, got:\n%s", want, out)
		}
	}
}

func TestConfigAuth_ValidKey(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()

	_, _, code := run(t, filepath.Join(dir, "c.yaml"), kr, "config", "auth", "r2-key", "mykey")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	val, err := kr.Get(config.ServiceName, "r2-key")
	if err != nil {
		t.Fatalf("keyring.Get: %v", err)
	}
	if val != "mykey" {
		t.Errorf("keyring value = %q, want %q", val, "mykey")
	}
}

func TestConfigAuth_InvalidKey(t *testing.T) {
	dir := t.TempDir()
	_, _, code := run(t, filepath.Join(dir, "c.yaml"), newFakeKeyring(), "config", "auth", "unknown", "val")
	if code == 0 {
		t.Error("expected non-zero exit for unknown keyring key")
	}
}

func TestConfigAuth_KeyringError(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()
	kr.err = errors.New("keyring unavailable")

	_, stderr, code := run(t, filepath.Join(dir, "c.yaml"), kr, "config", "auth", "r2-key", "v")
	if code == 0 {
		t.Error("expected non-zero exit on keyring error")
	}
	if !strings.Contains(stderr, "keyring") {
		t.Errorf("expected keyring error in stderr, got: %q", stderr)
	}
}
