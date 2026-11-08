package gmail

import (
	"context"
	"fmt"
	"net/mail"
	"sort"
	"strconv"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// SenderSummary contains aggregated volume statistics for an individual sender.
type SenderSummary struct {
	DisplayName  string
	Email        string
	TotalCount   int
	UnreadCount  int
	LastDate     string
}

// ParseSender parses an RFC 5322 address string into display name and email address.
func ParseSender(raw string) (name, email string) {
	raw = strings.TrimSpace(raw)
	if addr, err := mail.ParseAddress(raw); err == nil {
		name = addr.Name
		email = strings.ToLower(addr.Address)
		if name == "" {
			name = email
		}
		return name, email
	}

	// Fallback for non-standard formats
	clean := strings.Trim(raw, "<>\" ")
	return clean, strings.ToLower(clean)
}

// AggregateSenders aggregates a list of messages into sender statistics.
func AggregateSenders(messages []*MessageSummary) []*SenderSummary {
	counts := make(map[string]*SenderSummary)

	for _, msg := range messages {
		name, email := ParseSender(msg.From)
		if email == "" {
			continue
		}

		s, exists := counts[email]
		if !exists {
			s = &SenderSummary{
				DisplayName: name,
				Email:       email,
				LastDate:    msg.Date,
			}
			counts[email] = s
		}

		s.TotalCount++
		if msg.IsUnread {
			s.UnreadCount++
		}
	}

	result := make([]*SenderSummary, 0, len(counts))
	for _, s := range counts {
		result = append(result, s)
	}

	// Sort alphabetically by sender name / email (A to Z)
	sort.Slice(result, func(i, j int) bool {
		keyI := SenderSortKey(result[i])
		keyJ := SenderSortKey(result[j])
		if keyI != keyJ {
			return keyI < keyJ
		}
		return strings.ToLower(result[i].Email) < strings.ToLower(result[j].Email)
	})

	return result
}

// SenderSortKey returns the normalized string used to sort senders alphabetically (A-Z).
func SenderSortKey(s *SenderSummary) string {
	if s == nil {
		return ""
	}
	name := strings.TrimSpace(s.DisplayName)
	if name != "" && !strings.EqualFold(name, s.Email) {
		return strings.ToLower(name)
	}
	return strings.ToLower(s.Email)
}

// SampleSenders fetches recent message envelopes from key folders (INBOX and All Mail)
// to build a comprehensive global list of top senders.
func (c *Client) SampleSenders(ctx context.Context, sampleLimit int) ([]*SenderSummary, error) {
	if sampleLimit <= 0 {
		sampleLimit = 100
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cl, err := c.ensureConnected()
	if err != nil {
		return nil, err
	}

	var allSampled []*MessageSummary

	// Sample from INBOX
	inboxSummaries, err := c.sampleFolderLocked(cl, "INBOX", sampleLimit)
	if err == nil {
		allSampled = append(allSampled, inboxSummaries...)
	}

	// Sample from All Mail
	allMailSummaries, err := c.sampleFolderLocked(cl, c.allBox, sampleLimit)
	if err == nil {
		allSampled = append(allSampled, allMailSummaries...)
	}

	if len(allSampled) == 0 {
		return []*SenderSummary{}, nil
	}

	return AggregateSenders(allSampled), nil
}

// SampleFolderSenders samples up to sampleLimit (default 150) messages from the specified folder
// using the dedicated background IMAP session to prevent blocking user email retrieval.
func (c *Client) SampleFolderSenders(ctx context.Context, folder string, sampleLimit int) ([]*SenderSummary, int, error) {
	if sampleLimit <= 0 {
		sampleLimit = 150
	}

	c.bgMu.Lock()
	defer c.bgMu.Unlock()

	cl, err := c.ensureBgConnected()
	if err != nil {
		// Fallback to foreground client if secondary connection fails
		return c.sampleFolderForegroundFallback(folder, sampleLimit)
	}

	actual := c.ResolveMailbox(folder)
	mbox, err := cl.Select(actual, true)
	if err != nil || mbox == nil || mbox.Messages == 0 {
		return []*SenderSummary{}, 0, err
	}

	total := int(mbox.Messages)
	summaries, err := c.sampleFolderWithClient(cl, mbox, sampleLimit)
	if err != nil {
		return nil, total, err
	}

	return AggregateSenders(summaries), total, nil
}

func (c *Client) sampleFolderForegroundFallback(folder string, limit int) ([]*SenderSummary, int, error) {
	// Restrict foreground sample to 25 so it completes in ~100ms and never blocks user email opening
	if limit > 25 {
		limit = 25
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cl, err := c.ensureConnected()
	if err != nil {
		return nil, 0, err
	}

	actual := c.ResolveMailbox(folder)
	mbox, err := c.SelectMailbox(cl, actual)
	if err != nil || mbox == nil || mbox.Messages == 0 {
		return []*SenderSummary{}, 0, err
	}

	total := int(mbox.Messages)
	summaries, err := c.sampleFolderWithClient(cl, mbox, limit)
	if err != nil {
		return nil, total, err
	}

	return AggregateSenders(summaries), total, nil
}

func (c *Client) sampleFolderLocked(cl *client.Client, folder string, limit int) ([]*MessageSummary, error) {
	actual := c.ResolveMailbox(folder)
	mbox, err := c.SelectMailbox(cl, actual)
	if err != nil || mbox == nil || mbox.Messages == 0 {
		return nil, err
	}
	return c.sampleFolderWithClient(cl, mbox, limit)
}

func (c *Client) sampleFolderWithClient(cl *client.Client, mbox *imap.MailboxStatus, limit int) ([]*MessageSummary, error) {
	if mbox == nil || mbox.Messages == 0 {
		return nil, nil
	}

	to := mbox.Messages
	from := uint32(1)
	if to > uint32(limit) {
		from = to - uint32(limit) + 1
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchUid,
	}

	messagesChan := make(chan *imap.Message, limit)
	done := make(chan error, 1)
	go func() {
		done <- cl.Fetch(seqset, items, messagesChan)
	}()

	var summaries []*MessageSummary
	for msg := range messagesChan {
		if msg == nil || msg.Envelope == nil {
			continue
		}
		fromStr := ""
		if len(msg.Envelope.From) > 0 {
			addr := msg.Envelope.From[0]
			email := fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName)
			if addr.PersonalName != "" {
				fromStr = fmt.Sprintf("%s <%s>", addr.PersonalName, email)
			} else {
				fromStr = email
			}
		}

		s := &MessageSummary{
			ID:   strconv.FormatUint(uint64(msg.Uid), 10),
			UID:  msg.Uid,
			From: fromStr,
			Date: msg.Envelope.Date.Format("02 Jan 15:04"),
		}
		summaries = append(summaries, s)
	}

	_ = <-done
	return summaries, nil
}

// FilterMessagesBySender filters a slice of messages matching a given sender email address.
func FilterMessagesBySender(messages []*MessageSummary, senderEmail string) []*MessageSummary {
	target := strings.ToLower(strings.TrimSpace(senderEmail))
	if target == "" {
		return messages
	}
	var matched []*MessageSummary
	for _, m := range messages {
		_, email := ParseSender(m.From)
		if strings.EqualFold(email, target) {
			matched = append(matched, m)
		}
	}
	return matched
}
