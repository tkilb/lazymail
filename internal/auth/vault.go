package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	vaultFile = "vault.json.enc"
)

// Vault holds sensitive credentials encrypted together in a single file on disk.
// No plaintext passwords or tokens ever touch the filesystem.
type Vault struct {
	Email       string `json:"email"`
	AppPassword string `json:"app_password"`
}

// VaultPath returns the absolute path to the encrypted vault file (~/.config/lazymail/vault.json.enc).
func VaultPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, vaultFile), nil
}

// VaultExists returns true if the encrypted vault file exists.
func VaultExists() (bool, error) {
	path, err := VaultPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// SaveVault encrypts and writes the complete vault to disk using the master passphrase.
func SaveVault(v *Vault, passphrase string) error {
	if v == nil {
		return errors.New("cannot save nil vault")
	}
	if passphrase == "" {
		return ErrEmptyPassphrase
	}

	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal vault: %w", err)
	}

	path, err := VaultPath()
	if err != nil {
		return err
	}

	return EncryptToFile(path, data, passphrase)
}

// LoadVault reads and decrypts the vault from disk using the master passphrase.
func LoadVault(passphrase string) (*Vault, error) {
	if passphrase == "" {
		return nil, ErrEmptyPassphrase
	}

	path, err := VaultPath()
	if err != nil {
		return nil, err
	}

	data, err := DecryptFromFile(path, passphrase)
	if err != nil {
		return nil, err
	}

	var v Vault
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("unmarshal vault: %w", err)
	}

	return &v, nil
}
