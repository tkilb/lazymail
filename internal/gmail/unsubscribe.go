package gmail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	headerAngleRegex = regexp.MustCompile(`<([^>]+)>`)
	bodyHrefRegex    = regexp.MustCompile(`(?i)href\s*=\s*["'](https?://[^"']*(?:unsubscribe|optout|unsub)[^"']*)["']`)
	plainTextURL     = regexp.MustCompile(`(?i)(https?://\S*(?:unsubscribe|optout|unsub)\S*)`)
)

// UnsubscribeInfo holds extracted unsubscribe options from headers and email body.
type UnsubscribeInfo struct {
	HTTPURL     string
	Mailto      string
	IsOneClick  bool // RFC 8058
	FoundInBody bool
}

// HasUnsubscribe returns true if any unsubscribe mechanism is detected.
func (u *UnsubscribeInfo) HasUnsubscribe() bool {
	if u == nil {
		return false
	}
	return u.HTTPURL != "" || u.Mailto != ""
}

// ParseUnsubscribe parses RFC 2369 / RFC 8058 headers and falls back to body inspection.
func ParseUnsubscribe(headers map[string]string, bodyHTML, bodyPlain string) *UnsubscribeInfo {
	info := &UnsubscribeInfo{}

	// Case-insensitive header lookup
	var listUnsub, listUnsubPost string
	for k, v := range headers {
		lower := strings.ToLower(k)
		if lower == "list-unsubscribe" {
			listUnsub = v
		} else if lower == "list-unsubscribe-post" {
			listUnsubPost = v
		}
	}

	if strings.Contains(strings.ToLower(listUnsubPost), "list-unsubscribe=one-click") {
		info.IsOneClick = true
	}

	if listUnsub != "" {
		matches := headerAngleRegex.FindAllStringSubmatch(listUnsub, -1)
		for _, m := range matches {
			if len(m) > 1 {
				target := strings.TrimSpace(m[1])
				if strings.HasPrefix(target, "mailto:") && info.Mailto == "" {
					info.Mailto = target
				} else if (strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "http://")) && info.HTTPURL == "" {
					info.HTTPURL = target
				}
			}
		}
	}

	// Fallback to body inspection if no HTTP link was found
	if info.HTTPURL == "" {
		if bodyHTML != "" {
			if match := bodyHrefRegex.FindStringSubmatch(bodyHTML); len(match) > 1 {
				info.HTTPURL = match[1]
				info.FoundInBody = true
			}
		}
		if info.HTTPURL == "" && bodyPlain != "" {
			if match := plainTextURL.FindStringSubmatch(bodyPlain); len(match) > 1 {
				info.HTTPURL = match[1]
				info.FoundInBody = true
			}
		}
	}

	return info
}

// ExecuteOneClick performs an authenticated RFC 8058 HTTP POST request.
func ExecuteOneClick(ctx context.Context, client *http.Client, targetURL string) error {
	if targetURL == "" {
		return errors.New("empty unsubscribe url")
	}

	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	body := strings.NewReader("List-Unsubscribe=One-Click")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post one-click unsubscribe: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server responded with status %d", resp.StatusCode)
	}

	return nil
}

// SendUnsubscribeEmail sends an unsubscribe email via Gmail SMTP if mailto: URI is provided.
func SendUnsubscribeEmail(ctx context.Context, client *Client, mailtoURI string) error {
	if !strings.HasPrefix(mailtoURI, "mailto:") {
		return errors.New("invalid mailto URI")
	}

	u, err := url.Parse(mailtoURI)
	if err != nil {
		return fmt.Errorf("parse mailto url: %w", err)
	}

	to := u.Opaque
	if to == "" {
		to = u.Path
	}

	query := u.Query()
	subject := query.Get("subject")
	if subject == "" {
		subject = "unsubscribe"
	}
	body := query.Get("body")
	if body == "" {
		body = "Unsubscribe"
	}

	return client.SendEmail(ctx, to, subject, body)
}
