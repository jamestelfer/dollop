package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type authFileData struct {
	Credentials map[string]string `yaml:"credentials"`
}

// PlaintextStore implements KeyringStore using a YAML file at path.
// Credentials are stored unencrypted; file permissions are 0600 (user only).
type PlaintextStore struct {
	path string
}

// NewPlaintextStore returns a PlaintextStore that reads and writes path.
func NewPlaintextStore(path string) *PlaintextStore {
	return &PlaintextStore{path: path}
}

// Path returns the file path used by this store.
func (p *PlaintextStore) Path() string { return p.path }

// Stat returns the FileInfo for the auth file, or an error if it does not exist.
func (p *PlaintextStore) Stat() (os.FileInfo, error) {
	return os.Stat(p.path) //nolint:gosec
}

func (p *PlaintextStore) credKey(service, user string) string {
	return service + "/" + user
}

func (p *PlaintextStore) load() (authFileData, error) {
	data, err := os.ReadFile(p.path) //nolint:gosec
	if errors.Is(err, os.ErrNotExist) {
		return authFileData{Credentials: map[string]string{}}, nil
	}
	if err != nil {
		return authFileData{}, err
	}
	var af authFileData
	if err := yaml.Unmarshal(data, &af); err != nil {
		return authFileData{}, err
	}
	if af.Credentials == nil {
		af.Credentials = map[string]string{}
	}
	return af, nil
}

func (p *PlaintextStore) save(af authFileData) error {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(af)
	if err != nil {
		return err
	}
	return os.WriteFile(p.path, data, 0o600)
}

func (p *PlaintextStore) Get(service, user string) (string, error) {
	af, err := p.load()
	if err != nil {
		return "", err
	}
	val, ok := af.Credentials[p.credKey(service, user)]
	if !ok {
		return "", fmt.Errorf("credential %q not found in plain-text store", p.credKey(service, user))
	}
	return val, nil
}

func (p *PlaintextStore) Set(service, user, password string) error {
	af, err := p.load()
	if err != nil {
		return err
	}
	af.Credentials[p.credKey(service, user)] = password
	return p.save(af)
}

func (p *PlaintextStore) Delete(service, user string) error {
	af, err := p.load()
	if err != nil {
		return err
	}
	delete(af.Credentials, p.credKey(service, user))
	return p.save(af)
}
