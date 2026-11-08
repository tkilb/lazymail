package gmail

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/quotedprintable"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/emersion/go-imap"
	"golang.org/x/net/html/charset"
)

// AttachmentInfo describes a non-body attachment detected from BodyStructure without downloading content.
type AttachmentInfo struct {
	PartPath []int
	Filename string
	MIMEType string
	Size     uint32
}

// MIMEPartInfo tracks an individual MIME body or attachment part.
type MIMEPartInfo struct {
	Path     []int
	MIMEType string
	Encoding string
	Charset  string
	Size     uint32
	Filename string
}

// MIMEStructureInfo records the parsed body and attachment parts from IMAP BODYSTRUCTURE.
type MIMEStructureInfo struct {
	HasAttachments bool
	HTMLPart       *MIMEPartInfo
	PlainPart      *MIMEPartInfo
	Attachments    []*AttachmentInfo
}

// MessageSummary contains core metadata for rendering an email in the list view.
type MessageSummary struct {
	ID             string // UID converted to string
	UID            uint32
	Subject        string
	From           string
	To             string
	Date           string
	Snippet        string
	IsUnread       bool
	HasUnsubscribe bool
	HasAttachments bool
	HTMLPartPath   []int
	PlainPartPath  []int
	HTMLEncoding   string
	PlainEncoding  string
	HTMLCharset    string
	PlainCharset   string
}

// MessageDetail contains full email contents including body and unsubscribe info.
type MessageDetail struct {
	MessageSummary
	BodyPlain   string
	BodyHTML    string
	Headers     map[string]string
	Attachments []*AttachmentInfo
	Unsubscribe *UnsubscribeInfo
}

// ResolveMailbox maps friendly mailbox IDs (INBOX, TRASH, SENT, SUBSCRIPTIONS) to IMAP folder names.
func (c *Client) ResolveMailbox(mailboxID string) string {
	upper := strings.ToUpper(mailboxID)
	switch upper {
	case "INBOX", "SUBSCRIPTIONS":
		return "INBOX"
	case "TRASH":
		return c.trashBox
	case "SENT":
		return c.sentBox
	case "ALL":
		return c.allBox
	default:
		return mailboxID
	}
}

// ListMessages retrieves message summaries from the specified mailbox with pagination support.
// offset indicates how many of the newest messages to skip (for infinite scroll).
// Returns the slice of summaries, the total number of messages available, and any error.
func (c *Client) ListMessages(ctx context.Context, mailbox string, offset, limit int, queryFilter string) ([]*MessageSummary, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cl, err := c.ensureConnected()
	if err != nil {
		return nil, 0, err
	}

	actualBox := c.ResolveMailbox(mailbox)
	mbox, err := c.SelectMailbox(cl, actualBox)
	if err != nil {
		return nil, 0, fmt.Errorf("select mailbox %s: %w", actualBox, err)
	}

	if mbox.Messages == 0 {
		return []*MessageSummary{}, 0, nil
	}

	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchUid,
		imap.FetchBodyStructure,
	}

	var fetchDone <-chan error
	messagesChan := make(chan *imap.Message, limit)
	totalAvailable := 0

	// Check if we can use server-side search for Subscriptions or Sender filters
	if strings.EqualFold(mailbox, "SUBSCRIPTIONS") || (queryFilter != "" && strings.HasPrefix(queryFilter, "from:")) {
		criteria := imap.NewSearchCriteria()
		if strings.EqualFold(mailbox, "SUBSCRIPTIONS") {
			criteria.Text = []string{"unsubscribe"}
		}
		if queryFilter != "" && strings.HasPrefix(queryFilter, "from:") {
			senderEmail := strings.Trim(strings.TrimPrefix(queryFilter, "from:"), "\" ")
			criteria.Header.Add("From", senderEmail)
		}

		uids, err := cl.UidSearch(criteria)
		if err != nil {
			return nil, 0, fmt.Errorf("search messages: %w", err)
		}
		totalAvailable = len(uids)
		if totalAvailable == 0 || offset >= totalAvailable {
			return []*MessageSummary{}, totalAvailable, nil
		}

		endIdx := totalAvailable - offset
		startIdx := endIdx - limit
		if startIdx < 0 {
			startIdx = 0
		}
		fetchUIDs := uids[startIdx:endIdx]

		seqset := new(imap.SeqSet)
		seqset.AddNum(fetchUIDs...)

		done := make(chan error, 1)
		go func() {
			done <- cl.UidFetch(seqset, items, messagesChan)
		}()
		fetchDone = done
	} else {
		totalAvailable = int(mbox.Messages)
		if totalAvailable == 0 || offset >= totalAvailable {
			return []*MessageSummary{}, totalAvailable, nil
		}

		to := totalAvailable - offset
		from := to - limit + 1
		if from < 1 {
			from = 1
		}

		seqset := new(imap.SeqSet)
		seqset.AddRange(uint32(from), uint32(to))

		done := make(chan error, 1)
		go func() {
			done <- cl.Fetch(seqset, items, messagesChan)
		}()
		fetchDone = done
	}

	var summaries []*MessageSummary

	for msg := range messagesChan {
		if msg == nil || msg.Envelope == nil {
			continue
		}

		s := &MessageSummary{
			ID:       strconv.FormatUint(uint64(msg.Uid), 10),
			UID:      msg.Uid,
			Subject:  msg.Envelope.Subject,
			Date:     msg.Envelope.Date.Format("02 Jan 15:04"),
			IsUnread: true,
		}

		for _, flag := range msg.Flags {
			if flag == imap.SeenFlag {
				s.IsUnread = false
				break
			}
		}

		if len(msg.Envelope.From) > 0 {
			addr := msg.Envelope.From[0]
			name := addr.PersonalName
			email := fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName)
			if name != "" {
				s.From = fmt.Sprintf("%s <%s>", name, email)
			} else {
				s.From = email
			}
		}

		if len(msg.Envelope.To) > 0 {
			addr := msg.Envelope.To[0]
			s.To = fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName)
		}

		if strings.EqualFold(mailbox, "SUBSCRIPTIONS") {
			s.HasUnsubscribe = true
		}

		if msg.BodyStructure != nil {
			structInfo := parseMIMEStructure(msg.BodyStructure, nil)
			c.setMIMEStructure(mailbox, s.ID, structInfo)
			s.HasAttachments = structInfo.HasAttachments
			if structInfo.HTMLPart != nil {
				s.HTMLPartPath = structInfo.HTMLPart.Path
				s.HTMLEncoding = structInfo.HTMLPart.Encoding
				s.HTMLCharset = structInfo.HTMLPart.Charset
			}
			if structInfo.PlainPart != nil {
				s.PlainPartPath = structInfo.PlainPart.Path
				s.PlainEncoding = structInfo.PlainPart.Encoding
				s.PlainCharset = structInfo.PlainPart.Charset
			}
		}

		summaries = append(summaries, s)
	}

	if err := <-fetchDone; err != nil {
		return nil, 0, fmt.Errorf("fetch messages: %w", err)
	}

	// Reverse so newest messages appear at top of list
	for i, j := 0, len(summaries)-1; i < j; i, j = i+1, j-1 {
		summaries[i], summaries[j] = summaries[j], summaries[i]
	}

	return summaries, totalAvailable, nil
}

// FormatByteSize formats a byte count into a human-readable size string (e.g. "2.4 MB").
func FormatByteSize(bytes uint32) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	} else {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
}

// parseMIMEStructure walks an IMAP BodyStructure tree, identifying the targeted plain/html parts
// and cataloging any attachments, strictly excluding them from body downloads.
func parseMIMEStructure(bs *imap.BodyStructure, currentPath []int) *MIMEStructureInfo {
	info := &MIMEStructureInfo{}
	walkMIMEStructure(bs, currentPath, info)
	return info
}

func walkMIMEStructure(bs *imap.BodyStructure, currentPath []int, info *MIMEStructureInfo) {
	if bs == nil {
		return
	}

	if isAttachment(bs) {
		info.HasAttachments = true
		filename := getPartFilename(bs)
		info.Attachments = append(info.Attachments, &AttachmentInfo{
			PartPath: currentPath,
			Filename: filename,
			MIMEType: fmt.Sprintf("%s/%s", strings.ToLower(bs.MIMEType), strings.ToLower(bs.MIMESubType)),
			Size:     bs.Size,
		})
		return
	}

	if strings.EqualFold(bs.MIMEType, "multipart") {
		for i, part := range bs.Parts {
			childPath := make([]int, len(currentPath)+1)
			copy(childPath, currentPath)
			childPath[len(currentPath)] = i + 1
			walkMIMEStructure(part, childPath, info)
		}
		return
	}

	mimeType := strings.ToLower(bs.MIMEType)
	subType := strings.ToLower(bs.MIMESubType)

	if mimeType == "text" {
		partInfo := &MIMEPartInfo{
			Path:     currentPath,
			MIMEType: fmt.Sprintf("%s/%s", mimeType, subType),
			Encoding: bs.Encoding,
			Charset:  getPartCharset(bs),
			Size:     bs.Size,
		}
		if subType == "html" {
			if info.HTMLPart == nil {
				info.HTMLPart = partInfo
			}
		} else if subType == "plain" {
			if info.PlainPart == nil {
				info.PlainPart = partInfo
			}
		}
	} else {
		// Non-text, non-multipart leaf that is not a body text part
		if !strings.EqualFold(bs.Disposition, "inline") {
			info.HasAttachments = true
		}
		info.Attachments = append(info.Attachments, &AttachmentInfo{
			PartPath: currentPath,
			Filename: getPartFilename(bs),
			MIMEType: fmt.Sprintf("%s/%s", mimeType, subType),
			Size:     bs.Size,
		})
	}
}

func isAttachment(bs *imap.BodyStructure) bool {
	if bs == nil {
		return false
	}
	if strings.EqualFold(bs.Disposition, "attachment") {
		return true
	}
	if strings.EqualFold(bs.Disposition, "inline") {
		return false
	}
	// Non-text, non-multipart parts (PDF, image, audio, zip, etc.) are treated as attachments
	if !strings.EqualFold(bs.MIMEType, "text") && !strings.EqualFold(bs.MIMEType, "multipart") {
		return true
	}
	return false
}

func getPartFilename(bs *imap.BodyStructure) string {
	if bs == nil {
		return ""
	}
	if bs.DispositionParams != nil {
		if fn, ok := bs.DispositionParams["filename"]; ok && strings.TrimSpace(fn) != "" {
			return strings.TrimSpace(fn)
		}
	}
	if bs.Params != nil {
		if name, ok := bs.Params["name"]; ok && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

func getPartCharset(bs *imap.BodyStructure) string {
	if bs == nil || bs.Params == nil {
		return "utf-8"
	}
	if cs, ok := bs.Params["charset"]; ok && strings.TrimSpace(cs) != "" {
		return strings.TrimSpace(cs)
	}
	return "utf-8"
}

// findMIMEBodyParts walks an IMAP BodyStructure tree to locate the part paths for HTML and plain text bodies.
func findMIMEBodyParts(bs *imap.BodyStructure, currentPath []int) (htmlPath, plainPath []int) {
	info := parseMIMEStructure(bs, currentPath)
	if info.HTMLPart != nil {
		htmlPath = info.HTMLPart.Path
	}
	if info.PlainPart != nil {
		plainPath = info.PlainPart.Path
	}
	return htmlPath, plainPath
}

// decodeBodyBytes decodes raw transfer-encoded bytes (quoted-printable or base64) and converts charset to UTF-8.
func decodeBodyBytes(raw []byte, encoding string, cs string) string {
	if len(raw) == 0 {
		return ""
	}

	var decoded []byte
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		r := quotedprintable.NewReader(bytes.NewReader(raw))
		b, err := io.ReadAll(r)
		if err == nil {
			decoded = b
		} else {
			decoded = raw
		}
	case "base64":
		clean := bytes.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, raw)

		if rem := len(clean) % 4; rem != 0 {
			clean = append(clean, bytes.Repeat([]byte("="), 4-rem)...)
		}

		dst := make([]byte, base64.StdEncoding.DecodedLen(len(clean)))
		n, err := base64.StdEncoding.Decode(dst, clean)
		if err == nil {
			decoded = dst[:n]
		} else {
			n, err = base64.RawStdEncoding.Decode(dst, clean)
			if err == nil {
				decoded = dst[:n]
			} else {
				decoded = raw
			}
		}
	default:
		decoded = raw
	}

	// Charset conversion to UTF-8
	cs = strings.TrimSpace(strings.ToLower(cs))
	if cs != "" && cs != "utf-8" && cs != "us-ascii" && cs != "ascii" {
		if r, err := charset.NewReaderLabel(cs, bytes.NewReader(decoded)); err == nil {
			if b, err := io.ReadAll(r); err == nil {
				return string(b)
			}
		}
	}

	return string(decoded)
}

// GetMessageDetail fetches and parses the email body for the given message UID using targeted MIME part retrieval.
// Attachments are strictly excluded from download, eliminating multi-megabyte transfer delays.
func (c *Client) GetMessageDetail(ctx context.Context, mailbox string, uidStr string) (*MessageDetail, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	uid64, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid uid %s: %w", uidStr, err)
	}
	uid := uint32(uid64)

	cl, err := c.ensureConnected()
	if err != nil {
		return nil, err
	}

	actualBox := c.ResolveMailbox(mailbox)
	_, err = c.SelectMailbox(cl, actualBox)
	if err != nil {
		return nil, fmt.Errorf("select mailbox %s: %w", actualBox, err)
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	headerSection := &imap.BodySectionName{
		BodyPartName: imap.BodyPartName{
			Specifier: imap.HeaderSpecifier,
		},
		Peek: true,
	}

	structInfo := c.getMIMEStructure(mailbox, uidStr)

	// FAST PATH (1 roundtrip): structure already known from ListMessages
	if structInfo != nil {
		items := []imap.FetchItem{
			imap.FetchEnvelope,
			imap.FetchFlags,
			imap.FetchUid,
			headerSection.FetchItem(),
		}

		var targetPart *MIMEPartInfo
		isHTML := false
		if structInfo.HTMLPart != nil {
			targetPart = structInfo.HTMLPart
			isHTML = true
		} else if structInfo.PlainPart != nil {
			targetPart = structInfo.PlainPart
			isHTML = false
		}

		var targetSec *imap.BodySectionName
		if targetPart != nil {
			if len(targetPart.Path) == 0 {
				targetSec = &imap.BodySectionName{BodyPartName: imap.BodyPartName{Specifier: imap.TextSpecifier}, Peek: true}
			} else {
				targetSec = &imap.BodySectionName{BodyPartName: imap.BodyPartName{Path: targetPart.Path}, Peek: true}
			}
			items = append(items, targetSec.FetchItem())
		}

		messagesChan := make(chan *imap.Message, 1)
		done := make(chan error, 1)
		go func() {
			done <- cl.UidFetch(seqset, items, messagesChan)
		}()

		msg := <-messagesChan
		if err := <-done; err != nil {
			return nil, fmt.Errorf("uid fetch %d: %w", uid, err)
		}
		if msg == nil {
			return nil, errors.New("message not found")
		}

		detail := &MessageDetail{
			MessageSummary: MessageSummary{
				ID:             uidStr,
				UID:            uid,
				Subject:        msg.Envelope.Subject,
				Date:           msg.Envelope.Date.Format("Mon, 02 Jan 2006 15:04:05 MST"),
				IsUnread:       true,
				HasAttachments: structInfo.HasAttachments,
			},
			Headers:     make(map[string]string),
			Attachments: structInfo.Attachments,
		}

		for _, flag := range msg.Flags {
			if flag == imap.SeenFlag {
				detail.IsUnread = false
				break
			}
		}

		if len(msg.Envelope.From) > 0 {
			addr := msg.Envelope.From[0]
			email := fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName)
			if addr.PersonalName != "" {
				detail.From = fmt.Sprintf("%s <%s>", addr.PersonalName, email)
			} else {
				detail.From = email
			}
		}

		if hr := msg.GetBody(headerSection); hr != nil {
			tp := textproto.NewReader(bufio.NewReader(hr))
			mimeHeader, err := tp.ReadMIMEHeader()
			if err == nil {
				for k, v := range mimeHeader {
					if len(v) > 0 {
						detail.Headers[k] = v[0]
					}
				}
			}
		}

		if targetSec != nil {
			if tr := msg.GetBody(targetSec); tr != nil {
				b, _ := io.ReadAll(tr)
				enc := ""
				cs := ""
				if targetPart != nil {
					enc = targetPart.Encoding
					cs = targetPart.Charset
				}
				content := decodeBodyBytes(b, enc, cs)
				if isHTML {
					detail.BodyHTML = content
					detail.BodyPlain = HTMLToPlainText(content)
				} else {
					detail.BodyPlain = content
				}
			}
		}

		if detail.BodyPlain == "" && detail.BodyHTML != "" {
			detail.BodyPlain = HTMLToPlainText(detail.BodyHTML)
		}

		detail.Unsubscribe = ParseUnsubscribe(detail.Headers, detail.BodyHTML, detail.BodyPlain)
		detail.HasUnsubscribe = detail.Unsubscribe.HasUnsubscribe()

		return detail, nil
	}

	// FALLBACK PATH: structure not cached, fetch metadata + BodyStructure, then targeted body part
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchUid,
		imap.FetchBodyStructure,
		headerSection.FetchItem(),
	}

	messagesChan := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- cl.UidFetch(seqset, items, messagesChan)
	}()

	msg := <-messagesChan
	if err := <-done; err != nil {
		return nil, fmt.Errorf("uid fetch %d: %w", uid, err)
	}
	if msg == nil {
		return nil, errors.New("message not found")
	}

	structInfo = parseMIMEStructure(msg.BodyStructure, nil)
	c.setMIMEStructure(mailbox, uidStr, structInfo)

	detail := &MessageDetail{
		MessageSummary: MessageSummary{
			ID:             uidStr,
			UID:            uid,
			Subject:        msg.Envelope.Subject,
			Date:           msg.Envelope.Date.Format("Mon, 02 Jan 2006 15:04:05 MST"),
			IsUnread:       true,
			HasAttachments: structInfo.HasAttachments,
		},
		Headers:     make(map[string]string),
		Attachments: structInfo.Attachments,
	}

	for _, flag := range msg.Flags {
		if flag == imap.SeenFlag {
			detail.IsUnread = false
			break
		}
	}

	if len(msg.Envelope.From) > 0 {
		addr := msg.Envelope.From[0]
		email := fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName)
		if addr.PersonalName != "" {
			detail.From = fmt.Sprintf("%s <%s>", addr.PersonalName, email)
		} else {
			detail.From = email
		}
	}

	if hr := msg.GetBody(headerSection); hr != nil {
		tp := textproto.NewReader(bufio.NewReader(hr))
		mimeHeader, err := tp.ReadMIMEHeader()
		if err == nil {
			for k, v := range mimeHeader {
				if len(v) > 0 {
					detail.Headers[k] = v[0]
				}
			}
		}
	}

	// Fetch targeted single body part only (zero attachments downloaded)
	var targetPart *MIMEPartInfo
	isHTML := false
	if structInfo.HTMLPart != nil {
		targetPart = structInfo.HTMLPart
		isHTML = true
	} else if structInfo.PlainPart != nil {
		targetPart = structInfo.PlainPart
		isHTML = false
	}

	var targetSec *imap.BodySectionName
	if targetPart != nil {
		if len(targetPart.Path) == 0 {
			targetSec = &imap.BodySectionName{BodyPartName: imap.BodyPartName{Specifier: imap.TextSpecifier}, Peek: true}
		} else {
			targetSec = &imap.BodySectionName{BodyPartName: imap.BodyPartName{Path: targetPart.Path}, Peek: true}
		}
		bItems := []imap.FetchItem{targetSec.FetchItem()}
		bChan := make(chan *imap.Message, 1)
		bDone := make(chan error, 1)
		go func() {
			bDone <- cl.UidFetch(seqset, bItems, bChan)
		}()
		if bMsg := <-bChan; bMsg != nil {
			if tr := bMsg.GetBody(targetSec); tr != nil {
				b, _ := io.ReadAll(tr)
				enc := ""
				cs := ""
				if targetPart != nil {
					enc = targetPart.Encoding
					cs = targetPart.Charset
				}
				content := decodeBodyBytes(b, enc, cs)
				if isHTML {
					detail.BodyHTML = content
					detail.BodyPlain = HTMLToPlainText(content)
				} else {
					detail.BodyPlain = content
				}
			}
		}
		<-bDone
	}

	if detail.BodyPlain == "" && detail.BodyHTML != "" {
		detail.BodyPlain = HTMLToPlainText(detail.BodyHTML)
	}

	detail.Unsubscribe = ParseUnsubscribe(detail.Headers, detail.BodyHTML, detail.BodyPlain)
	detail.HasUnsubscribe = detail.Unsubscribe.HasUnsubscribe()

	return detail, nil
}

// BatchTrash moves messages from their current mailbox into the Trash folder.
func (c *Client) BatchTrash(ctx context.Context, mailbox string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cl, err := c.ensureConnected()
	if err != nil {
		return err
	}

	actualBox := c.ResolveMailbox(mailbox)
	_, err = c.SelectMailbox(cl, actualBox)
	if err != nil {
		return fmt.Errorf("select mailbox: %w", err)
	}

	seqset := new(imap.SeqSet)
	for _, idStr := range ids {
		if uid, err := strconv.ParseUint(idStr, 10, 32); err == nil {
			seqset.AddNum(uint32(uid))
		}
	}

	// Copy to trash
	if err := cl.UidCopy(seqset, c.trashBox); err != nil {
		return fmt.Errorf("copy to trash (%s): %w", c.trashBox, err)
	}

	// Mark \Deleted in current mailbox and expunge
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}
	if err := cl.UidStore(seqset, item, flags, nil); err != nil {
		return fmt.Errorf("mark deleted: %w", err)
	}

	return cl.Expunge(nil)
}

// BatchArchive archives messages out of their current mailbox (removing the Inbox label).
func (c *Client) BatchArchive(ctx context.Context, mailbox string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cl, err := c.ensureConnected()
	if err != nil {
		return err
	}

	actualBox := c.ResolveMailbox(mailbox)
	_, err = c.SelectMailbox(cl, actualBox)
	if err != nil {
		return fmt.Errorf("select mailbox: %w", err)
	}

	seqset := new(imap.SeqSet)
	for _, idStr := range ids {
		if uid, err := strconv.ParseUint(idStr, 10, 32); err == nil {
			seqset.AddNum(uint32(uid))
		}
	}

	// In Gmail IMAP, marking Deleted on INBOX and expunging archives the message (retaining it in All Mail)
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}
	if err := cl.UidStore(seqset, item, flags, nil); err != nil {
		return fmt.Errorf("mark deleted for archive: %w", err)
	}

	return cl.Expunge(nil)
}

// BatchRestore moves messages out of Trash back to INBOX.
func (c *Client) BatchRestore(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cl, err := c.ensureConnected()
	if err != nil {
		return err
	}

	_, err = c.SelectMailbox(cl, c.trashBox)
	if err != nil {
		return fmt.Errorf("select trash: %w", err)
	}

	seqset := new(imap.SeqSet)
	for _, idStr := range ids {
		if uid, err := strconv.ParseUint(idStr, 10, 32); err == nil {
			seqset.AddNum(uint32(uid))
		}
	}

	// Copy to INBOX
	if err := cl.UidCopy(seqset, "INBOX"); err != nil {
		return fmt.Errorf("restore to inbox: %w", err)
	}

	// Mark \Deleted in Trash and expunge
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}
	if err := cl.UidStore(seqset, item, flags, nil); err != nil {
		return fmt.Errorf("mark deleted in trash: %w", err)
	}

	return cl.Expunge(nil)
}

// BatchDelete permanently purges messages inside Trash.
func (c *Client) BatchDelete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cl, err := c.ensureConnected()
	if err != nil {
		return err
	}

	_, err = c.SelectMailbox(cl, c.trashBox)
	if err != nil {
		return fmt.Errorf("select trash: %w", err)
	}

	seqset := new(imap.SeqSet)
	for _, idStr := range ids {
		if uid, err := strconv.ParseUint(idStr, 10, 32); err == nil {
			seqset.AddNum(uint32(uid))
		}
	}

	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}
	if err := cl.UidStore(seqset, item, flags, nil); err != nil {
		return fmt.Errorf("mark deleted: %w", err)
	}

	return cl.Expunge(nil)
}

// BatchMarkRead marks the specified messages as read or unread.
func (c *Client) BatchMarkRead(ctx context.Context, mailbox string, ids []string, read bool) error {
	if len(ids) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cl, err := c.ensureConnected()
	if err != nil {
		return err
	}

	actualBox := c.ResolveMailbox(mailbox)
	_, err = cl.Select(actualBox, false)
	if err != nil {
		return fmt.Errorf("select mailbox: %w", err)
	}

	seqset := new(imap.SeqSet)
	for _, idStr := range ids {
		if uid, err := strconv.ParseUint(idStr, 10, 32); err == nil {
			seqset.AddNum(uint32(uid))
		}
	}

	var op imap.FlagsOp = imap.FlagsOp(imap.AddFlags)
	if !read {
		op = imap.FlagsOp(imap.RemoveFlags)
	}

	item := imap.FormatFlagsOp(op, true)
	flags := []interface{}{imap.SeenFlag}
	return cl.UidStore(seqset, item, flags, nil)
}
