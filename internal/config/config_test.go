package config_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/config"
)

func TestLoadSave_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := config.Config{Bucket: "b", AccountID: "a", BaseURL: "u"}
	if err := config.SaveTo(path, original); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	got, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got != original {
		t.Errorf("got %+v, want %+v", got, original)
	}
}

func TestLoad_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	got, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (config.Config{}) {
		t.Errorf("expected zero Config, got %+v", got)
	}
}

func TestConfigDir_XDGSet(t *testing.T) {
	got, err := config.ConfigDirWith(func(key string) (string, bool) {
		if key == "XDG_CONFIG_HOME" {
			return "/custom/xdg", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/custom/xdg/dollop" {
		t.Errorf("got %q, want %q", got, "/custom/xdg/dollop")
	}
}

func TestConfigDir_XDGUnset(t *testing.T) {
	got, err := config.ConfigDirWith(func(_ string) (string, bool) {
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "/.config/dollop") {
		t.Errorf("got %q, want suffix %q", got, "/.config/dollop")
	}
}

func TestConfig_SetGet_ValidKeys(t *testing.T) {
	cases := []struct {
		key   string
		value string
	}{
		{"bucket", "my-bucket"},
		{"account-id", "abc123"},
		{"base-url", "https://drop.example.com"},
	}
	for _, tc := range cases {
		var c config.Config
		if err := c.Set(tc.key, tc.value); err != nil {
			t.Fatalf("Set(%q): %v", tc.key, err)
		}
		got, err := c.Get(tc.key)
		if err != nil {
			t.Fatalf("Get(%q): %v", tc.key, err)
		}
		if got != tc.value {
			t.Errorf("Get(%q) = %q, want %q", tc.key, got, tc.value)
		}
	}
}

func TestConfig_Set_InvalidKey(t *testing.T) {
	var c config.Config
	if err := c.Set("unknown", "val"); err == nil {
		t.Error("expected error for unknown key, got nil")
	}
}

func TestConfig_List(t *testing.T) {
	var c config.Config
	_ = c.Set("bucket", "b")
	_ = c.Set("account-id", "a")
	_ = c.Set("base-url", "u")

	pairs := c.List()
	if len(pairs) != 3 {
		t.Fatalf("List() returned %d pairs, want 3", len(pairs))
	}
	// must follow AllowedFileKeys order
	want := [][2]string{{"bucket", "b"}, {"account-id", "a"}, {"base-url", "u"}}
	for i, p := range pairs {
		if p != want[i] {
			t.Errorf("pairs[%d] = %v, want %v", i, p, want[i])
		}
	}
}

func TestConfig_Require_Missing(t *testing.T) {
	var c config.Config
	_, err := c.Require("bucket")
	if err == nil {
		t.Fatal("expected ErrMissingKey, got nil")
	}
	var mk config.ErrMissingKey
	if !errors.As(err, &mk) {
		t.Fatalf("expected ErrMissingKey, got %T: %v", err, err)
	}
	if mk.Key != "bucket" {
		t.Errorf("ErrMissingKey.Key = %q, want %q", mk.Key, "bucket")
	}
}

func TestConfig_Require_Present(t *testing.T) {
	var c config.Config
	_ = c.Set("bucket", "my-bucket")
	got, err := c.Require("bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-bucket" {
		t.Errorf("Require = %q, want %q", got, "my-bucket")
	}
}

func TestConfigDir_XDGEmpty(t *testing.T) {
	got, err := config.ConfigDirWith(func(key string) (string, bool) {
		if key == "XDG_CONFIG_HOME" {
			return "", true // set but empty
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "/.config/dollop") {
		t.Errorf("got %q, want suffix %q", got, "/.config/dollop")
	}
}
