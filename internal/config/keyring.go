package config

import (
	"os"

	"github.com/zalando/go-keyring"
)

// KeyringStore is the interface for reading and writing secrets.
type KeyringStore interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

// SystemKeyring delegates to the OS keyring.
type SystemKeyring struct{}

func (SystemKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (SystemKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (SystemKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

// NewSystemKeyring returns a KeyringStore backed by the OS keyring.
func NewSystemKeyring() KeyringStore { return SystemKeyring{} }

// FallbackStore tries primary on Get; if it fails and the plaintext store's
// file exists, retries against the plaintext store. Set and Delete always use
// primary.
type FallbackStore struct {
	primary  KeyringStore
	fallback *PlaintextStore
}

// NewFallbackStore returns a KeyringStore that reads from primary and falls
// back to pt when its file exists.
func NewFallbackStore(primary KeyringStore, pt *PlaintextStore) KeyringStore {
	return &FallbackStore{primary: primary, fallback: pt}
}

func (f *FallbackStore) Get(service, user string) (string, error) {
	val, err := f.primary.Get(service, user)
	if err == nil {
		return val, nil
	}
	if _, ferr := os.Stat(f.fallback.Path()); ferr == nil {
		return f.fallback.Get(service, user)
	}
	return "", err
}

func (f *FallbackStore) Set(service, user, password string) error {
	return f.primary.Set(service, user, password)
}

func (f *FallbackStore) Delete(service, user string) error {
	return f.primary.Delete(service, user)
}
