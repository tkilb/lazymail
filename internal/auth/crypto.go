package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	saltSize     = 16
	nonceSize    = 12
	keySize      = 32
	pbkdf2Iters  = 100_000
	minEncLength = saltSize + nonceSize + 16 // salt + nonce + GCM tag
)

var (
	// ErrEmptyPassphrase is returned when attempting encryption or decryption with an empty passphrase.
	ErrEmptyPassphrase = errors.New("passphrase cannot be empty")

	// ErrCorruptCiphertext is returned when ciphertext is too short or malformed.
	ErrCorruptCiphertext = errors.New("ciphertext is malformed or too short")

	// ErrAuthenticationFailed is returned when decryption fails due to invalid passphrase or corrupted payload.
	ErrAuthenticationFailed = errors.New("decryption failed: incorrect passphrase or data corrupted")
)

// Encrypt encrypts plaintext using AES-256-GCM with a key derived from passphrase via PBKDF2-SHA256.
// Output format: [16 bytes salt][12 bytes GCM nonce][ciphertext + GCM tag].
func Encrypt(plaintext []byte, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, ErrEmptyPassphrase
	}

	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	key, err := pbkdf2.Key(sha256.New, passphrase, salt, pbkdf2Iters, keySize)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	sealed := gcm.Seal(nil, nonce, plaintext, nil)

	// Combine [salt][nonce][sealed]
	result := make([]byte, 0, len(salt)+len(nonce)+len(sealed))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, sealed...)

	return result, nil
}

// Decrypt decrypts ciphertext produced by Encrypt using the supplied passphrase.
func Decrypt(encrypted []byte, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, ErrEmptyPassphrase
	}

	if len(encrypted) < minEncLength {
		return nil, ErrCorruptCiphertext
	}

	salt := encrypted[:saltSize]
	nonce := encrypted[saltSize : saltSize+nonceSize]
	ciphertext := encrypted[saltSize+nonceSize:]

	key, err := pbkdf2.Key(sha256.New, passphrase, salt, pbkdf2Iters, keySize)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrAuthenticationFailed
	}

	return plaintext, nil
}

// EncryptToFile encrypts plaintext and writes it securely to targetPath with 0600 permissions.
func EncryptToFile(targetPath string, plaintext []byte, passphrase string) error {
	encrypted, err := Encrypt(plaintext, passphrase)
	if err != nil {
		return err
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Write atomically using a temp file in the same directory
	tempFile, err := os.CreateTemp(dir, "lazymail-auth-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tempFile.Name())
	}()

	if err := tempFile.Chmod(0600); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if _, err := tempFile.Write(encrypted); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tempFile.Name(), targetPath); err != nil {
		return fmt.Errorf("rename temp file to %s: %w", targetPath, err)
	}

	return nil
}

// DecryptFromFile reads and decrypts the file at sourcePath with passphrase.
func DecryptFromFile(sourcePath string, passphrase string) ([]byte, error) {
	encrypted, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", sourcePath, err)
	}

	return Decrypt(encrypted, passphrase)
}

// DeriveKey derives a 32-byte AES key from a passphrase and salt using PBKDF2-SHA256.
func DeriveKey(passphrase string, salt []byte) ([]byte, error) {
	if passphrase == "" {
		return nil, ErrEmptyPassphrase
	}
	return pbkdf2.Key(sha256.New, passphrase, salt, pbkdf2Iters, keySize)
}

// EncryptWithKey encrypts plaintext using an already derived 32-byte key via AES-256-GCM.
// Output format: [12 bytes GCM nonce][ciphertext + GCM tag].
func EncryptWithKey(plaintext, key []byte) ([]byte, error) {
	if len(key) != keySize {
		return nil, errors.New("invalid key size")
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	result := make([]byte, 0, len(nonce)+len(sealed))
	result = append(result, nonce...)
	result = append(result, sealed...)
	return result, nil
}

// DecryptWithKey decrypts ciphertext using an already derived 32-byte key via AES-256-GCM.
func DecryptWithKey(encrypted, key []byte) ([]byte, error) {
	if len(key) != keySize {
		return nil, errors.New("invalid key size")
	}

	if len(encrypted) < nonceSize+16 {
		return nil, ErrCorruptCiphertext
	}

	nonce := encrypted[:nonceSize]
	ciphertext := encrypted[nonceSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrAuthenticationFailed
	}

	return plaintext, nil
}

// WriteEncryptedFileWithKey encrypts plaintext using an existing key and atomically writes it with 0600 permissions.
func WriteEncryptedFileWithKey(targetPath string, plaintext, key []byte) error {
	encrypted, err := EncryptWithKey(plaintext, key)
	if err != nil {
		return err
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tempFile, err := os.CreateTemp(dir, "lazymail-cache-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tempFile.Name())
	}()

	if err := tempFile.Chmod(0600); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if _, err := tempFile.Write(encrypted); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tempFile.Name(), targetPath); err != nil {
		return fmt.Errorf("rename temp file to %s: %w", targetPath, err)
	}

	return nil
}

// ReadEncryptedFileWithKey reads and decrypts the file at sourcePath using the provided key.
func ReadEncryptedFileWithKey(sourcePath string, key []byte) ([]byte, error) {
	encrypted, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", sourcePath, err)
	}

	return DecryptWithKey(encrypted, key)
}
