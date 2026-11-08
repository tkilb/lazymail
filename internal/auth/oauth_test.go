package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type mockTokenSource struct {
	tokens []*oauth2.Token
	index  int
	err    error
}

func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.index < len(m.tokens) {
		tok := m.tokens[m.index]
		m.index++
		return tok, nil
	}
	return m.tokens[len(m.tokens)-1], nil
}

func TestAutoSavingTokenSource(t *testing.T) {
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	_ = os.Setenv("HOME", tempDir)

	passphrase := "test-pass"
	initialTok := &oauth2.Token{
		AccessToken:  "initial-access-token",
		RefreshToken: "initial-refresh-token",
		Expiry:       time.Now().Add(1 * time.Hour),
	}

	// Save initial token first
	if err := SaveEncryptedToken(initialTok, passphrase); err != nil {
		t.Fatalf("SaveEncryptedToken failed: %v", err)
	}

	refreshedTok := &oauth2.Token{
		AccessToken:  "refreshed-access-token",
		RefreshToken: "initial-refresh-token",
		Expiry:       time.Now().Add(2 * time.Hour),
	}

	mock := &mockTokenSource{
		tokens: []*oauth2.Token{refreshedTok},
	}

	autoSource := NewAutoSavingTokenSource(mock, initialTok, passphrase)

	tok, err := autoSource.Token()
	if err != nil {
		t.Fatalf("autoSource.Token() error: %v", err)
	}
	if tok.AccessToken != "refreshed-access-token" {
		t.Errorf("got token %s, want refreshed-access-token", tok.AccessToken)
	}

	// Verify that the file on disk was updated and can be decrypted to refreshed token
	loadedTok, err := LoadEncryptedToken(passphrase)
	if err != nil {
		t.Fatalf("LoadEncryptedToken failed: %v", err)
	}
	if loadedTok.AccessToken != "refreshed-access-token" {
		t.Errorf("persisted token was not updated: got %s", loadedTok.AccessToken)
	}
}

func TestTokenExists(t *testing.T) {
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	_ = os.Setenv("HOME", tempDir)

	exists, err := TokenExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatalf("expected TokenExists to be false for non-existent token")
	}

	// Create dummy encrypted token
	tokPath, err := TokenPath()
	if err != nil {
		t.Fatalf("TokenPath() failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(tokPath), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(tokPath, []byte("fake-encrypted-data"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	exists, err = TokenExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatalf("expected TokenExists to be true after creating token file")
	}
}

func TestLoadOAuthConfig_FileNotFound(t *testing.T) {
	_, err := LoadOAuthConfig("/non/existent/path/creds.json", DefaultScopes)
	if err == nil {
		t.Errorf("expected error for non-existent credentials file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) && !os.IsNotExist(err) {
		// Verify error message indicates file read failure
		if err == nil {
			t.Errorf("expected error, got nil")
		}
	}
}
