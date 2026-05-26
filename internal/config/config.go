package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const ServiceName = "dollop"

var AllowedFileKeys = []string{"bucket", "account-id", "base-url"}
var AllowedKeyringKeys = []string{"r2-key", "r2-secret"}

// Config holds the values persisted to the YAML config file.
type Config struct {
	Bucket    string `yaml:"bucket"`
	AccountID string `yaml:"account_id"`
	BaseURL   string `yaml:"base_url"`
}

// ErrMissingKey is returned when a required config value is empty.
type ErrMissingKey struct {
	Key string
}

func (e ErrMissingKey) Error() string {
	return fmt.Sprintf("required config key %q is not set", e.Key)
}

// Set assigns value to key. Returns an error for unknown keys.
func (c *Config) Set(key, value string) error {
	switch key {
	case "bucket":
		c.Bucket = value
	case "account-id":
		c.AccountID = value
	case "base-url":
		c.BaseURL = value
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

// Get returns the value for key. Returns an error for unknown keys.
func (c Config) Get(key string) (string, error) {
	switch key {
	case "bucket":
		return c.Bucket, nil
	case "account-id":
		return c.AccountID, nil
	case "base-url":
		return c.BaseURL, nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

// List returns all key/value pairs in AllowedFileKeys order.
func (c Config) List() [][2]string {
	pairs := make([][2]string, 0, len(AllowedFileKeys))
	for _, key := range AllowedFileKeys {
		val, _ := c.Get(key)
		pairs = append(pairs, [2]string{key, val})
	}
	return pairs
}

// Require returns the value for key or ErrMissingKey if empty.
func (c Config) Require(key string) (string, error) {
	val, err := c.Get(key)
	if err != nil {
		return "", err
	}
	if val == "" {
		return "", ErrMissingKey{Key: key}
	}
	return val, nil
}

// LoadFrom reads a Config from path. Returns a zero Config if the file does
// not exist.
func LoadFrom(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// SaveTo writes cfg as YAML to path, creating parent directories as needed.
func SaveTo(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Load reads the config file from the default XDG path.
func Load() (Config, error) {
	path, err := ConfigFilePath()
	if err != nil {
		return Config{}, err
	}
	return LoadFrom(path)
}

// Save writes cfg to the default XDG config file path.
func Save(cfg Config) error {
	path, err := ConfigFilePath()
	if err != nil {
		return err
	}
	return SaveTo(path, cfg)
}

// ConfigDirWith resolves the dollop config directory using the supplied env
// lookup function, enabling injection in tests.
func ConfigDirWith(envLookup func(string) (string, bool)) (string, error) {
	if xdg, ok := envLookup("XDG_CONFIG_HOME"); ok && xdg != "" {
		return filepath.Join(xdg, "dollop"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dollop"), nil
}

// ConfigFilePath returns the absolute path to the config file using the real
// environment.
func ConfigFilePath() (string, error) {
	dir, err := ConfigDirWith(os.LookupEnv)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// AuthFilePath returns the absolute path to the plain-text auth file using the
// real environment.
func AuthFilePath() (string, error) {
	dir, err := ConfigDirWith(os.LookupEnv)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.yaml"), nil
}
