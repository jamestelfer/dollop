package config

import "github.com/zalando/go-keyring"

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
