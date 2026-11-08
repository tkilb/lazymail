package auth

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecrypt_TableDriven(t *testing.T) {
	tests := []struct {
		name          string
		plaintext     []byte
		encPassphrase string
		decPassphrase string
		wantErr       error
	}{
		{
			name:          "standard ascii plaintext",
			plaintext:     []byte("hello world token payload 12345"),
			encPassphrase: "correct-horse-battery-staple",
			decPassphrase: "correct-horse-battery-staple",
			wantErr:       nil,
		},
		{
			name:          "json credentials payload",
			plaintext:     []byte(`{"access_token":"ya29.a0...","refresh_token":"1//0e...","expiry":"2026-11-07T16:00:00Z"}`),
			encPassphrase: "super-secure-passphrase!99",
			decPassphrase: "super-secure-passphrase!99",
			wantErr:       nil,
		},
		{
			name:          "empty plaintext",
			plaintext:     []byte(""),
			encPassphrase: "passphrase-for-empty",
			decPassphrase: "passphrase-for-empty",
			wantErr:       nil,
		},
		{
			name:          "wrong passphrase fails decryption",
			plaintext:     []byte("confidential oauth token"),
			encPassphrase: "actual-passphrase",
			decPassphrase: "wrong-passphrase",
			wantErr:       ErrAuthenticationFailed,
		},
		{
			name:          "empty passphrase on encrypt",
			plaintext:     []byte("sample data"),
			encPassphrase: "",
			decPassphrase: "any",
			wantErr:       ErrEmptyPassphrase,
		},
		{
			name:          "empty passphrase on decrypt",
			plaintext:     []byte("sample data"),
			encPassphrase: "valid-passphrase",
			decPassphrase: "",
			wantErr:       ErrEmptyPassphrase,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := Encrypt(tt.plaintext, tt.encPassphrase)
			if tt.encPassphrase == "" {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Encrypt() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected Encrypt() error: %v", err)
			}

			dec, err := Decrypt(enc, tt.decPassphrase)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Decrypt() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected Decrypt() error: %v", err)
			}

			if !bytes.Equal(dec, tt.plaintext) {
				t.Errorf("Decrypt() got = %q, want = %q", string(dec), string(tt.plaintext))
			}
		})
	}
}

func TestDecrypt_CorruptCiphertext(t *testing.T) {
	tests := []struct {
		name       string
		ciphertext []byte
		wantErr    error
	}{
		{
			name:       "empty ciphertext",
			ciphertext: []byte{},
			wantErr:    ErrCorruptCiphertext,
		},
		{
			name:       "shorter than salt+nonce+tag minimum",
			ciphertext: make([]byte, minEncLength-1),
			wantErr:    ErrCorruptCiphertext,
		},
		{
			name:       "corrupted gcm payload",
			ciphertext: make([]byte, minEncLength+10),
			wantErr:    ErrAuthenticationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decrypt(tt.ciphertext, "valid-passphrase")
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Decrypt() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEncryptToFile_DecryptFromFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "nested", "token.json.enc")
	passphrase := "master-key-xyz"
	secretPayload := []byte(`{"refresh_token":"1//secret_token"}`)

	// Test writing
	err := EncryptToFile(filePath, secretPayload, passphrase)
	if err != nil {
		t.Fatalf("EncryptToFile() failed: %v", err)
	}

	// Verify file mode is 0600
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("os.Stat() failed: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	// Test reading
	readPayload, err := DecryptFromFile(filePath, passphrase)
	if err != nil {
		t.Fatalf("DecryptFromFile() failed: %v", err)
	}
	if !bytes.Equal(readPayload, secretPayload) {
		t.Errorf("got decrypted payload %s, want %s", string(readPayload), string(secretPayload))
	}

	// Test reading with wrong passphrase
	_, err = DecryptFromFile(filePath, "wrong-passphrase")
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Errorf("DecryptFromFile() wrong pass error = %v, want %v", err, ErrAuthenticationFailed)
	}
}

func TestDerivedKey_EncryptDecrypt(t *testing.T) {
	passphrase := "fast-key-pass"
	salt := []byte("1234567890123456")

	key, err := DeriveKey(passphrase, salt)
	if err != nil {
		t.Fatalf("DeriveKey failed: %v", err)
	}

	plaintext := []byte("super fast cache payload without pbkdf2 re-derivation")
	ciphertext, err := EncryptWithKey(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptWithKey failed: %v", err)
	}

	decrypted, err := DecryptWithKey(ciphertext, key)
	if err != nil {
		t.Fatalf("DecryptWithKey failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("got %s, want %s", string(decrypted), string(plaintext))
	}

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "fast.enc")
	if err := WriteEncryptedFileWithKey(filePath, plaintext, key); err != nil {
		t.Fatalf("WriteEncryptedFileWithKey failed: %v", err)
	}

	fileDecrypted, err := ReadEncryptedFileWithKey(filePath, key)
	if err != nil {
		t.Fatalf("ReadEncryptedFileWithKey failed: %v", err)
	}
	if !bytes.Equal(fileDecrypted, plaintext) {
		t.Fatalf("file decrypted mismatch")
	}
}
