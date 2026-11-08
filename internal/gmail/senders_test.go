package gmail

import (
	"testing"
)

func TestParseSender(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantName  string
		wantEmail string
	}{
		{
			name:      "standard name and angle address",
			raw:       "John Doe <john.doe@example.com>",
			wantName:  "John Doe",
			wantEmail: "john.doe@example.com",
		},
		{
			name:      "quoted name",
			raw:       `"GitHub Notifications" <notifications@github.com>`,
			wantName:  "GitHub Notifications",
			wantEmail: "notifications@github.com",
		},
		{
			name:      "bare email without name",
			raw:       "newsletter@weekly.com",
			wantName:  "newsletter@weekly.com",
			wantEmail: "newsletter@weekly.com",
		},
		{
			name:      "mixed case email normalized to lower",
			raw:       "Alice <Alice.Smith@COMPANY.COM>",
			wantName:  "Alice",
			wantEmail: "alice.smith@company.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, email := ParseSender(tt.raw)
			if name != tt.wantName {
				t.Errorf("ParseSender name = %q, want %q", name, tt.wantName)
			}
			if email != tt.wantEmail {
				t.Errorf("ParseSender email = %q, want %q", email, tt.wantEmail)
			}
		})
	}
}

func TestAggregateSenders(t *testing.T) {
	messages := []*MessageSummary{
		{From: "Bob <bob@test.com>", IsUnread: true, Date: "2026-11-01"},
		{From: "Alice <alice@test.com>", IsUnread: false, Date: "2026-11-02"},
		{From: "Bob <bob@test.com>", IsUnread: false, Date: "2026-11-03"},
		{From: "Bob <bob@test.com>", IsUnread: true, Date: "2026-11-04"},
		{From: "Charlie <charlie@test.com>", IsUnread: true, Date: "2026-11-05"},
	}

	senders := AggregateSenders(messages)
	if len(senders) != 3 {
		t.Fatalf("expected 3 senders, got %d", len(senders))
	}

	// Senders should be sorted alphabetically (A to Z): Alice, Bob, Charlie
	if senders[0].Email != "alice@test.com" || senders[0].TotalCount != 1 || senders[0].UnreadCount != 0 {
		t.Errorf("unexpected first sender: %+v", senders[0])
	}
	if senders[1].Email != "bob@test.com" || senders[1].TotalCount != 3 || senders[1].UnreadCount != 2 {
		t.Errorf("unexpected second sender: %+v", senders[1])
	}
	if senders[2].Email != "charlie@test.com" || senders[2].TotalCount != 1 || senders[2].UnreadCount != 1 {
		t.Errorf("unexpected third sender: %+v", senders[2])
	}
}

func TestFilterMessagesBySender(t *testing.T) {
	messages := []*MessageSummary{
		{ID: "1", From: "Bob <bob@test.com>"},
		{ID: "2", From: "Alice <alice@test.com>"},
		{ID: "3", From: "Bob <bob@test.com>"},
		{ID: "4", From: "Charlie <charlie@test.com>"},
	}

	bobMsgs := FilterMessagesBySender(messages, "bob@test.com")
	if len(bobMsgs) != 2 {
		t.Fatalf("expected 2 messages for bob, got %d", len(bobMsgs))
	}
	if bobMsgs[0].ID != "1" || bobMsgs[1].ID != "3" {
		t.Errorf("unexpected IDs for bob: %v, %v", bobMsgs[0].ID, bobMsgs[1].ID)
	}

	empty := FilterMessagesBySender(messages, "none@test.com")
	if len(empty) != 0 {
		t.Errorf("expected 0 messages for none, got %d", len(empty))
	}

	all := FilterMessagesBySender(messages, "")
	if len(all) != 4 {
		t.Errorf("expected all 4 messages for empty sender, got %d", len(all))
	}
}
