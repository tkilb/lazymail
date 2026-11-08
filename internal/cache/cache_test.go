package cache

import (
	"os"
	"testing"

	"github.com/tkilb/lazymail/internal/gmail"
)

func TestCache_EncryptedPersistence(t *testing.T) {
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	_ = os.Setenv("HOME", tempDir)

	passphrase := "test-cache-key-999"

	c, err := New(passphrase)
	if err != nil {
		t.Fatalf("New() cache error: %v", err)
	}

	messages := []*gmail.MessageSummary{
		{ID: "101", Subject: "Important Email", From: "boss@corp.com", IsUnread: true},
		{ID: "102", Subject: "Newsletter", From: "news@daily.com", IsUnread: false},
	}

	// Save to cache
	if err := c.SetMessages("INBOX", messages); err != nil {
		t.Fatalf("SetMessages failed: %v", err)
	}

	// Verify in-memory retrieval
	got := c.GetMessages("INBOX")
	if len(got) != 2 || got[0].Subject != "Important Email" {
		t.Fatalf("unexpected cached messages: %+v", got)
	}

	// Create a new Cache instance simulating app restart with the same passphrase
	c2, err := New(passphrase)
	if err != nil {
		t.Fatalf("New() second cache error: %v", err)
	}

	reloaded := c2.GetMessages("INBOX")
	if len(reloaded) != 2 || reloaded[0].Subject != "Important Email" {
		t.Fatalf("reloaded cached messages failed: %+v", reloaded)
	}

	// Verify that loading with wrong passphrase fails to decrypt
	cWrong, err := New("wrong-passphrase")
	if err != nil {
		t.Fatalf("New() wrong cache error: %v", err)
	}
	wrongMsgs := cWrong.GetMessages("INBOX")
	if wrongMsgs != nil {
		t.Errorf("expected nil for failed decryption, got: %+v", wrongMsgs)
	}
}
