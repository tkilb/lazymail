package gmail

import (
	"fmt"
	"strings"
)

// Filter specifies criteria for querying messages.
type Filter struct {
	Label           string
	Sender          string
	Subject         string
	QueryText       string
	OnlyUnread      bool
	HasAttachment   bool
	IsSubscription  bool
}

// Build constructs a Gmail query string from the filter criteria.
func (f Filter) Build() string {
	var parts []string

	if f.Label != "" {
		parts = append(parts, fmt.Sprintf("label:%s", f.Label))
	}
	if f.Sender != "" {
		parts = append(parts, fmt.Sprintf("from:%s", cleanQueryToken(f.Sender)))
	}
	if f.Subject != "" {
		parts = append(parts, fmt.Sprintf("subject:%s", cleanQueryToken(f.Subject)))
	}
	if f.OnlyUnread {
		parts = append(parts, "is:unread")
	}
	if f.HasAttachment {
		parts = append(parts, "has:attachment")
	}
	if f.IsSubscription {
		// Matches typical bulk/newsletter/subscription headers or unsubscribe indicators
		parts = append(parts, "unsubscribe")
	}
	if f.QueryText != "" {
		parts = append(parts, f.QueryText)
	}

	return strings.Join(parts, " ")
}

// cleanQueryToken wraps tokens containing spaces in quotes.
func cleanQueryToken(token string) string {
	token = strings.TrimSpace(token)
	if strings.Contains(token, " ") && !strings.HasPrefix(token, "\"") {
		return fmt.Sprintf("\"%s\"", token)
	}
	return token
}

// Predefined smart queries
func InboxQuery() string {
	return "label:INBOX"
}

func TrashQuery() string {
	return "label:TRASH"
}

func SubscriptionsQuery() string {
	return "label:INBOX unsubscribe"
}

func SenderQuery(senderEmail string) string {
	return Filter{Sender: senderEmail}.Build()
}
