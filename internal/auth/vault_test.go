package auth

import (
	"os"
	"testing"
)

func TestVault_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	_ = os.Setenv("HOME", tempDir)

	passphrase := "vault-test-passphrase"
	v := &Vault{
		Email:       "testuser@gmail.com",
		AppPassword: "abcd efgh ijkl mnop",
	}

	exists, err := VaultExists()
	if err != nil {
		t.Fatalf("VaultExists() error: %v", err)
	}
	if exists {
		t.Fatalf("expected vault to not exist yet")
	}

	err = SaveVault(v, passphrase)
	if err != nil {
		t.Fatalf("SaveVault() failed: %v", err)
	}

	exists, err = VaultExists()
	if err != nil || !exists {
		t.Fatalf("expected vault to exist now")
	}

	loaded, err := LoadVault(passphrase)
	if err != nil {
		t.Fatalf("LoadVault() failed: %v", err)
	}

	if loaded.Email != v.Email {
		t.Errorf("Email got %q, want %q", loaded.Email, v.Email)
	}
	if loaded.AppPassword != v.AppPassword {
		t.Errorf("AppPassword got %q, want %q", loaded.AppPassword, v.AppPassword)
	}

	// Loading with wrong passphrase must fail
	_, err = LoadVault("wrong-passphrase")
	if err == nil {
		t.Errorf("expected decryption error with wrong passphrase, got nil")
	}
}
