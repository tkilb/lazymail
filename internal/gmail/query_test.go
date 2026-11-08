package gmail

import (
	"testing"
)

func TestFilter_Build(t *testing.T) {
	tests := []struct {
		name     string
		filter   Filter
		expected string
	}{
		{
			name:     "empty filter",
			filter:   Filter{},
			expected: "",
		},
		{
			name: "inbox with unread",
			filter: Filter{
				Label:      "INBOX",
				OnlyUnread: true,
			},
			expected: "label:INBOX is:unread",
		},
		{
			name: "sender with spaces",
			filter: Filter{
				Sender: "Acme Corp <newsletter@acme.com>",
			},
			expected: `from:"Acme Corp <newsletter@acme.com>"`,
		},
		{
			name: "subscription with label and query text",
			filter: Filter{
				Label:          "INBOX",
				IsSubscription: true,
				QueryText:      "promo",
			},
			expected: "label:INBOX unsubscribe promo",
		},
		{
			name: "all options combined",
			filter: Filter{
				Label:          "SPAM",
				Sender:         "promo@store.com",
				Subject:        "Special Discount",
				OnlyUnread:     true,
				HasAttachment:  true,
				IsSubscription: true,
				QueryText:      "after:2026/01/01",
			},
			expected: `label:SPAM from:promo@store.com subject:"Special Discount" is:unread has:attachment unsubscribe after:2026/01/01`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Build()
			if got != tt.expected {
				t.Errorf("Filter.Build() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSmartQueries(t *testing.T) {
	if got := InboxQuery(); got != "label:INBOX" {
		t.Errorf("InboxQuery() = %q, want label:INBOX", got)
	}
	if got := TrashQuery(); got != "label:TRASH" {
		t.Errorf("TrashQuery() = %q, want label:TRASH", got)
	}
	if got := SubscriptionsQuery(); got != "label:INBOX unsubscribe" {
		t.Errorf("SubscriptionsQuery() = %q, want 'label:INBOX unsubscribe'", got)
	}
	if got := SenderQuery("test@example.com"); got != "from:test@example.com" {
		t.Errorf("SenderQuery() = %q, want from:test@example.com", got)
	}
}
