package cache

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/tkilb/lazymail/internal/auth"
	"github.com/tkilb/lazymail/internal/gmail"
)

// Cache manages encrypted-at-rest persistence and fast in-memory access for email data.
type Cache struct {
	mu         sync.RWMutex
	dir        string
	passphrase string
	derivedKey []byte

	mailboxes map[string][]*gmail.MessageSummary
	details   map[string]*gmail.MessageDetail
	senders   map[string][]*gmail.SenderSummary
}

// New initializes the cache, creating ~/.config/lazymail/cache if needed and pre-deriving the key once.
func New(passphrase string) (*Cache, error) {
	configDir, err := auth.ConfigDir()
	if err != nil {
		return nil, err
	}

	cacheDir := filepath.Join(configDir, "cache")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	saltPath := filepath.Join(cacheDir, "salt.bin")
	salt, err := os.ReadFile(saltPath)
	if err != nil || len(salt) != 16 {
		salt = make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, fmt.Errorf("generate cache salt: %w", err)
		}
		_ = os.WriteFile(saltPath, salt, 0600)
	}

	key, err := auth.DeriveKey(passphrase, salt)
	if err != nil {
		return nil, fmt.Errorf("derive cache key: %w", err)
	}

	c := &Cache{
		dir:        cacheDir,
		passphrase: passphrase,
		derivedKey: key,
		mailboxes:  make(map[string][]*gmail.MessageSummary),
		details:    make(map[string]*gmail.MessageDetail),
		senders:    make(map[string][]*gmail.SenderSummary),
	}

	// Preload existing encrypted cache files into memory
	c.loadAllFromDisk()
	return c, nil
}

func (c *Cache) mailboxFilePath(mailbox string) string {
	h := sha256.Sum256([]byte(mailbox))
	return filepath.Join(c.dir, fmt.Sprintf("mbox_%s.enc", hex.EncodeToString(h[:8])))
}

func (c *Cache) detailsFilePath() string {
	return filepath.Join(c.dir, "details.enc")
}

func (c *Cache) sendersFilePath(mailbox string) string {
	h := sha256.Sum256([]byte("senders:" + mailbox))
	return filepath.Join(c.dir, fmt.Sprintf("senders_%s.enc", hex.EncodeToString(h[:8])))
}

func (c *Cache) loadAllFromDisk() {
	c.mu.Lock()
	defer c.mu.Unlock()

	detPath := c.detailsFilePath()
	if data, err := auth.ReadEncryptedFileWithKey(detPath, c.derivedKey); err == nil {
		var details map[string]*gmail.MessageDetail
		if err := json.Unmarshal(data, &details); err == nil {
			c.details = details
		}
	}
}

// GetMessages returns cached message summaries for a mailbox from memory.
func (c *Cache) GetMessages(mailbox string) []*gmail.MessageSummary {
	c.mu.RLock()
	if msgs, exists := c.mailboxes[mailbox]; exists {
		c.mu.RUnlock()
		return msgs
	}
	c.mu.RUnlock()

	// Try loading from encrypted disk file using pre-derived key
	path := c.mailboxFilePath(mailbox)
	data, err := auth.ReadEncryptedFileWithKey(path, c.derivedKey)
	if err != nil {
		return nil
	}

	var msgs []*gmail.MessageSummary
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil
	}

	// Store in memory
	c.mu.Lock()
	c.mailboxes[mailbox] = msgs
	c.mu.Unlock()
	return msgs
}

// SetMessages updates cached message summaries both in-memory and encrypted to disk.
func (c *Cache) SetMessages(mailbox string, messages []*gmail.MessageSummary) error {
	c.mu.Lock()
	c.mailboxes[mailbox] = messages
	c.mu.Unlock()

	// Serialize and encrypt with pre-derived key (0.05ms)
	data, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	path := c.mailboxFilePath(mailbox)
	return auth.WriteEncryptedFileWithKey(path, data, c.derivedKey)
}

// GetDetail returns cached message detail for a specific message UID in 0ms from RAM.
func (c *Cache) GetDetail(mailbox, id string) *gmail.MessageDetail {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", mailbox, id)
	return c.details[key]
}

// SetDetail stores message detail in-memory immediately and asynchronously encrypts to disk.
func (c *Cache) SetDetail(mailbox, id string, detail *gmail.MessageDetail) error {
	c.mu.Lock()
	key := fmt.Sprintf("%s:%s", mailbox, id)
	c.details[key] = detail

	// Create snapshot for non-blocking disk persistence
	snapshot := make(map[string]*gmail.MessageDetail, len(c.details))
	for k, v := range c.details {
		snapshot[k] = v
	}
	c.mu.Unlock()

	go func(snap map[string]*gmail.MessageDetail) {
		data, err := json.Marshal(snap)
		if err != nil {
			return
		}
		path := c.detailsFilePath()
		_ = auth.WriteEncryptedFileWithKey(path, data, c.derivedKey)
	}(snapshot)

	return nil
}

// GetSenders returns cached sender statistics for a mailbox from memory or encrypted disk.
func (c *Cache) GetSenders(mailbox string) []*gmail.SenderSummary {
	c.mu.RLock()
	if s, exists := c.senders[mailbox]; exists {
		c.mu.RUnlock()
		return s
	}
	c.mu.RUnlock()

	path := c.sendersFilePath(mailbox)
	data, err := auth.ReadEncryptedFileWithKey(path, c.derivedKey)
	if err != nil {
		return nil
	}

	var list []*gmail.SenderSummary
	if err := json.Unmarshal(data, &list); err != nil {
		return nil
	}

	c.mu.Lock()
	c.senders[mailbox] = list
	c.mu.Unlock()
	return list
}

// SetSenders updates cached sender statistics in-memory and asynchronously encrypts to disk.
func (c *Cache) SetSenders(mailbox string, senders []*gmail.SenderSummary) error {
	c.mu.Lock()
	c.senders[mailbox] = senders
	snapshot := make([]*gmail.SenderSummary, len(senders))
	copy(snapshot, senders)
	c.mu.Unlock()

	go func(snap []*gmail.SenderSummary) {
		data, err := json.Marshal(snap)
		if err != nil {
			return
		}
		path := c.sendersFilePath(mailbox)
		_ = auth.WriteEncryptedFileWithKey(path, data, c.derivedKey)
	}(snapshot)

	return nil
}

