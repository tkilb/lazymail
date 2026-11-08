package gmail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"sort"
	"strings"
	"sync"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

const (
	DefaultIMAPServer = "imap.gmail.com:993"
	DefaultSMTPServer = "smtp.gmail.com:587"
)

// Client wraps direct IMAP and SMTP connections to Gmail.
type Client struct {
	Email       string
	AppPassword string
	IMAPServer  string
	SMTPServer  string

	mu             sync.Mutex
	imapClient     *client.Client
	currentMailbox string
	trashBox       string
	sentBox        string
	allBox         string

	bgMu     sync.Mutex
	bgClient *client.Client

	structMu       sync.RWMutex
	structureCache map[string]*MIMEStructureInfo
}

// NewClient establishes an authenticated IMAP session using an App Password.
func NewClient(email, appPassword string) (*Client, error) {
	c := &Client{
		Email:          strings.TrimSpace(email),
		AppPassword:    strings.ReplaceAll(appPassword, " ", ""), // strip spaces from "abcd efgh ijkl mnop"
		IMAPServer:     DefaultIMAPServer,
		SMTPServer:     DefaultSMTPServer,
		trashBox:       "[Gmail]/Trash",
		sentBox:        "[Gmail]/Sent Mail",
		allBox:         "[Gmail]/All Mail",
		structureCache: make(map[string]*MIMEStructureInfo),
	}

	if err := c.connect(); err != nil {
		return nil, fmt.Errorf("connect to IMAP (%s): %w", c.Email, err)
	}

	c.discoverSpecialMailboxes()
	return c, nil
}

func (c *Client) getMIMEStructure(mailbox, uidStr string) *MIMEStructureInfo {
	c.structMu.RLock()
	defer c.structMu.RUnlock()
	if c.structureCache == nil {
		return nil
	}
	return c.structureCache[mailbox+":"+uidStr]
}

func (c *Client) setMIMEStructure(mailbox, uidStr string, info *MIMEStructureInfo) {
	c.structMu.Lock()
	defer c.structMu.Unlock()
	if c.structureCache == nil {
		c.structureCache = make(map[string]*MIMEStructureInfo)
	}
	if len(c.structureCache) > 1000 {
		c.structureCache = make(map[string]*MIMEStructureInfo)
	}
	c.structureCache[mailbox+":"+uidStr] = info
}

// WarmMIMEStructure pre-populates the in-memory structure cache from a cached summary.
func (c *Client) WarmMIMEStructure(mailbox string, s *MessageSummary) {
	if s == nil || s.ID == "" {
		return
	}
	info := &MIMEStructureInfo{
		HasAttachments: s.HasAttachments,
	}
	if len(s.HTMLPartPath) > 0 || s.HTMLEncoding != "" {
		info.HTMLPart = &MIMEPartInfo{
			Path:     s.HTMLPartPath,
			Encoding: s.HTMLEncoding,
			Charset:  s.HTMLCharset,
			MIMEType: "text/html",
		}
	}
	if len(s.PlainPartPath) > 0 || s.PlainEncoding != "" {
		info.PlainPart = &MIMEPartInfo{
			Path:     s.PlainPartPath,
			Encoding: s.PlainEncoding,
			Charset:  s.PlainCharset,
			MIMEType: "text/plain",
		}
	}
	c.setMIMEStructure(mailbox, s.ID, info)
}

func (c *Client) connect() error {
	tlsConfig := &tls.Config{
		ServerName: "imap.gmail.com",
	}

	imapCl, err := client.DialTLS(c.IMAPServer, tlsConfig)
	if err != nil {
		return fmt.Errorf("dial IMAP: %w", err)
	}

	if err := imapCl.Login(c.Email, c.AppPassword); err != nil {
		_ = imapCl.Logout()
		return fmt.Errorf("login failed (verify email and 16-character App Password): %w", err)
	}

	c.imapClient = imapCl
	c.currentMailbox = ""
	return nil
}

// ensureConnected verifies IMAP session is active or reconnects.
func (c *Client) ensureConnected() (*client.Client, error) {
	if c.imapClient != nil && c.imapClient.State() != imap.LogoutState {
		return c.imapClient, nil
	}
	if err := c.connect(); err != nil {
		return nil, err
	}
	return c.imapClient, nil
}

// ensureBgConnected verifies or establishes a dedicated secondary IMAP session for non-blocking background tasks.
func (c *Client) ensureBgConnected() (*client.Client, error) {
	if c.bgClient != nil && c.bgClient.State() != imap.LogoutState {
		return c.bgClient, nil
	}
	tlsConfig := &tls.Config{
		ServerName: "imap.gmail.com",
	}
	cl, err := client.DialTLS(c.IMAPServer, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("dial bg IMAP: %w", err)
	}
	if err := cl.Login(c.Email, c.AppPassword); err != nil {
		_ = cl.Logout()
		return nil, fmt.Errorf("login bg IMAP: %w", err)
	}
	c.bgClient = cl
	return c.bgClient, nil
}


// SelectMailbox ensures actualBox is selected on the IMAP connection without redundant round-trips.
func (c *Client) SelectMailbox(cl *client.Client, actualBox string) (*imap.MailboxStatus, error) {
	if c.currentMailbox == actualBox && cl.Mailbox() != nil && cl.Mailbox().Name == actualBox {
		return cl.Mailbox(), nil
	}
	mbox, err := cl.Select(actualBox, false)
	if err != nil {
		c.currentMailbox = ""
		return nil, err
	}
	c.currentMailbox = actualBox
	return mbox, nil
}

// discoverSpecialMailboxes finds localized names for Trash and Sent (e.g. "[Gmail]/Bin").
func (c *Client) discoverSpecialMailboxes() {
	mailboxes := make(chan *imap.MailboxInfo, 20)
	done := make(chan error, 1)
	go func() {
		done <- c.imapClient.List("", "*", mailboxes)
	}()

	for m := range mailboxes {
		name := m.Name
		lower := strings.ToLower(name)
		if strings.Contains(lower, "trash") || strings.Contains(lower, "bin") {
			c.trashBox = name
		} else if strings.Contains(lower, "sent") {
			c.sentBox = name
		} else if strings.Contains(lower, "all") {
			c.allBox = name
		}
	}
	_ = <-done
}

// TrashMailbox returns the detected Trash folder name.
func (c *Client) TrashMailbox() string {
	return c.trashBox
}

// SentMailbox returns the detected Sent folder name.
func (c *Client) SentMailbox() string {
	return c.sentBox
}

// AllMailbox returns the detected All Mail folder name.
func (c *Client) AllMailbox() string {
	return c.allBox
}

// MailboxSummary represents a discovered mailbox for UI listing.
type MailboxSummary struct {
	ID    string
	Title string
	Query string
}

// DiscoverCleanMailboxes queries all server IMAP mailboxes, excludes system noise,
// and returns a clean, sorted list of standard and user custom folders.
func (c *Client) DiscoverCleanMailboxes() ([]MailboxSummary, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cl, err := c.ensureConnected()
	if err != nil {
		return nil, err
	}

	mailboxes := make(chan *imap.MailboxInfo, 50)
	done := make(chan error, 1)
	go func() {
		done <- cl.List("", "*", mailboxes)
	}()

	var userFolders []string
	for m := range mailboxes {
		name := m.Name
		lower := strings.ToLower(name)

		if strings.Contains(lower, "trash") || strings.Contains(lower, "bin") {
			c.trashBox = name
			continue
		} else if strings.Contains(lower, "sent") {
			c.sentBox = name
			continue
		} else if strings.Contains(lower, "all") {
			c.allBox = name
			continue
		} else if strings.EqualFold(name, "INBOX") {
			continue
		}

		// Filter out internal Gmail system folders
		if strings.HasPrefix(name, "[Gmail]") {
			continue
		}

		userFolders = append(userFolders, name)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("list mailboxes: %w", err)
	}

	sort.Strings(userFolders)

	var list []MailboxSummary
	list = append(list, MailboxSummary{ID: "INBOX", Title: "Inbox", Query: "label:INBOX"})
	list = append(list, MailboxSummary{ID: "SUBSCRIPTIONS", Title: "Subscriptions", Query: "label:INBOX unsubscribe"})

	for _, folder := range userFolders {
		list = append(list, MailboxSummary{
			ID:    folder,
			Title: folder,
			Query: "",
		})
	}

	list = append(list,
		MailboxSummary{ID: "ALL", Title: "All Mail", Query: ""},
		MailboxSummary{ID: "SENT", Title: "Sent", Query: "label:SENT"},
		MailboxSummary{ID: "TRASH", Title: "Trash", Query: "label:TRASH"},
	)

	return list, nil
}

// SendEmail sends an email via Gmail SMTP using the App Password.
func (c *Client) SendEmail(ctx context.Context, to, subject, body string) error {
	auth := smtp.PlainAuth("", c.Email, c.AppPassword, "smtp.gmail.com")
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", c.Email, to, subject, body))

	return smtp.SendMail(c.SMTPServer, auth, c.Email, []string{to}, msg)
}

// Close gracefully closes the IMAP sessions.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.imapClient != nil {
		_ = c.imapClient.Logout()
		c.imapClient = nil
	}
	c.mu.Unlock()

	c.bgMu.Lock()
	if c.bgClient != nil {
		_ = c.bgClient.Logout()
		c.bgClient = nil
	}
	c.bgMu.Unlock()
	return nil
}

