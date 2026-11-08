package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
)

const (
	defaultConfigDir = "lazymail"
	credentialsFile  = "credentials.json"
	tokenFile        = "token.json.enc"
)

// DefaultScopes defines the Gmail scopes needed for viewing, trashing, permanent purge, and unsubscribe emails.
var DefaultScopes = []string{
	gmail.MailGoogleComScope,
}

// ConfigDir returns the default configuration directory path (~/.config/lazymail).
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".config", defaultConfigDir), nil
}

// CredentialsPath returns the path to credentials.json.
func CredentialsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, credentialsFile), nil
}

// TokenPath returns the path to token.json.enc.
func TokenPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, tokenFile), nil
}

// LoadOAuthConfig reads Google OAuth2 credentials from the given JSON file.
func LoadOAuthConfig(credentialsPath string, scopes []string) (*oauth2.Config, error) {
	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("read credentials file (%s): %w", credentialsPath, err)
	}

	config, err := google.ConfigFromJSON(data, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parse oauth credentials json: %w", err)
	}

	return config, nil
}

// TokenExists checks whether an encrypted token file exists.
func TokenExists() (bool, error) {
	path, err := TokenPath()
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

// StartOAuthFlow starts a local loopback webserver, opens the browser, and completes OAuth2 code exchange.
func StartOAuthFlow(ctx context.Context, config *oauth2.Config, passphrase string, openBrowser bool) (*oauth2.Token, error) {
	if passphrase == "" {
		return nil, ErrEmptyPassphrase
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen on loopback: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/oauth2callback", port)
	configCopy := *config
	configCopy.RedirectURL = redirectURL

	// Generate random CSRF state
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("generate random state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	authURL := configCopy.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
	)

	type codeResult struct {
		code string
		err  error
	}
	resultCh := make(chan codeResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid OAuth state parameter", http.StatusBadRequest)
			resultCh <- codeResult{err: errors.New("state parameter mismatch")}
			return
		}
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			http.Error(w, "Authentication denied: "+errMsg, http.StatusForbidden)
			resultCh <- codeResult{err: fmt.Errorf("oauth error: %s", errMsg)}
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			resultCh <- codeResult{err: errors.New("no authorization code received")}
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, "<html><body><h2>Authentication successful!</h2><p>You can close this tab and return to LazyMail in your terminal.</p></body></html>")
		resultCh <- codeResult{code: code}
	})

	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if openBrowser {
		_ = OpenBrowser(authURL)
	}

	fmt.Printf("\nPlease authenticate in your browser. If it doesn't open automatically, visit:\n%s\n\n", authURL)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultCh:
		if res.err != nil {
			return nil, res.err
		}

		token, err := configCopy.Exchange(ctx, res.code)
		if err != nil {
			return nil, fmt.Errorf("exchange authorization code: %w", err)
		}

		if err := SaveEncryptedToken(token, passphrase); err != nil {
			return nil, fmt.Errorf("save encrypted token: %w", err)
		}

		return token, nil
	}
}

// SaveEncryptedToken serializes the token to JSON and saves it encrypted to TokenPath().
func SaveEncryptedToken(token *oauth2.Token, passphrase string) error {
	tokenData, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	tokenFilePath, err := TokenPath()
	if err != nil {
		return err
	}

	return EncryptToFile(tokenFilePath, tokenData, passphrase)
}

// LoadEncryptedToken reads and decrypts the stored OAuth2 token using the session passphrase.
func LoadEncryptedToken(passphrase string) (*oauth2.Token, error) {
	tokenFilePath, err := TokenPath()
	if err != nil {
		return nil, err
	}

	data, err := DecryptFromFile(tokenFilePath, passphrase)
	if err != nil {
		return nil, err
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("unmarshal token: %w", err)
	}

	return &token, nil
}

// AutoSavingTokenSource wraps an oauth2.TokenSource to ensure refreshed tokens are immediately re-encrypted to disk.
type AutoSavingTokenSource struct {
	mu         sync.Mutex
	base       oauth2.TokenSource
	lastToken  *oauth2.Token
	passphrase string
}

// NewAutoSavingTokenSource returns an oauth2.TokenSource that saves refreshed tokens encrypted to disk.
func NewAutoSavingTokenSource(base oauth2.TokenSource, initialToken *oauth2.Token, passphrase string) oauth2.TokenSource {
	return &AutoSavingTokenSource{
		base:       base,
		lastToken:  initialToken,
		passphrase: passphrase,
	}
}

// Token returns a valid token or refreshes it, re-encrypting to disk if renewed.
func (s *AutoSavingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tok, err := s.base.Token()
	if err != nil {
		return nil, err
	}

	// If token was refreshed (different access token or different expiry), save it encrypted
	if s.lastToken == nil || tok.AccessToken != s.lastToken.AccessToken {
		s.lastToken = tok
		if saveErr := SaveEncryptedToken(tok, s.passphrase); saveErr != nil {
			// Log or handle error without breaking session
			fmt.Fprintf(os.Stderr, "Warning: failed to persist refreshed token: %v\n", saveErr)
		}
	}

	return tok, nil
}

// GetClient returns an authenticated http.Client configured with auto-saving token refresher.
func GetClient(ctx context.Context, config *oauth2.Config, passphrase string) (*http.Client, error) {
	token, err := LoadEncryptedToken(passphrase)
	if err != nil {
		return nil, fmt.Errorf("load token: %w", err)
	}

	baseSource := config.TokenSource(ctx, token)
	autoSavingSource := NewAutoSavingTokenSource(baseSource, token, passphrase)

	return oauth2.NewClient(ctx, autoSavingSource), nil
}

// OpenBrowser opens the given URL in the user's default web browser.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}
