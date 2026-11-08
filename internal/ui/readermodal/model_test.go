package readermodal

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tkilb/lazymail/internal/gmail"
)

func TestReaderModal_OpenAndClose(t *testing.T) {
	tests := []struct {
		name     string
		closeKey string
	}{
		{name: "close via esc", closeKey: "esc"},
		{name: "close via q", closeKey: "q"},
		{name: "close via o", closeKey: "o"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			if m.Active {
				t.Fatalf("expected inactive modal upon initialization")
			}

			msg := &gmail.MessageDetail{
				MessageSummary: gmail.MessageSummary{
					ID:      "msg-123",
					Subject: "Welcome to LazyMail",
					From:    "team@example.com",
					Date:    "2026-11-07",
				},
				BodyHTML:  "<h1>Hello World</h1><p>This is a test newsletter.</p>",
				BodyPlain: "Hello World\nThis is a test newsletter.",
			}

			m.Open(msg, 100, 40)
			if !m.Active {
				t.Fatalf("expected modal to be active after Open")
			}

			view := m.View()
			if view == "" {
				t.Fatalf("expected non-empty modal view")
			}

			// Send close key
			var keyMsg tea.KeyMsg
			if tt.closeKey == "esc" {
				keyMsg = tea.KeyMsg{Type: tea.KeyEsc}
			} else {
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.closeKey)}
			}
			m, _ = m.Update(keyMsg)

			if m.Active {
				t.Fatalf("expected modal to be closed after pressing %s", tt.closeKey)
			}
		})
	}
}

func TestReaderModal_NilMessage(t *testing.T) {
	m := New()
	m.Open(nil, 80, 24)
	if m.Active {
		t.Fatalf("expected modal not to activate with nil message")
	}
	if v := m.View(); v != "" {
		t.Fatalf("expected empty view for inactive modal, got %q", v)
	}
}

func TestReaderModal_NavigationKeys(t *testing.T) {
	m := New()
	longBody := ""
	for i := 0; i < 50; i++ {
		longBody += "<p>Paragraph line item for scrolling.</p>"
	}
	msg := &gmail.MessageDetail{
		MessageSummary: gmail.MessageSummary{
			ID:      "msg-long",
			Subject: "Long newsletter",
			From:    "info@example.com",
			Date:    "2026-11-07",
		},
		BodyHTML: longBody,
	}

	m.Open(msg, 80, 24)

	navKeys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("j")},
		{Type: tea.KeyRunes, Runes: []rune("k")},
		{Type: tea.KeyRunes, Runes: []rune("d")},
		{Type: tea.KeyRunes, Runes: []rune("u")},
		{Type: tea.KeyRunes, Runes: []rune("G")},
		{Type: tea.KeyRunes, Runes: []rune("g")},
	}

	for _, k := range navKeys {
		var cmd tea.Cmd
		m, cmd = m.Update(k)
		_ = cmd
	}

	if !m.Active {
		t.Fatalf("expected modal to remain active during navigation")
	}
}

func TestReaderModal_OpenLoadingAndSetDetail(t *testing.T) {
	m := New()
	summary := &gmail.MessageSummary{
		ID:      "msg-quick",
		Subject: "Quick Email",
		From:    "alert@service.com",
		Date:    "07 Nov 19:30",
	}

	m.OpenLoading(summary, 80, 24)
	if !m.Active || !m.Loading {
		t.Fatalf("expected modal active and loading, got active=%v, loading=%v", m.Active, m.Loading)
	}

	view := m.View()
	if !strings.Contains(view, "Fetching email contents") {
		t.Errorf("expected loading placeholder in view, got:\n%s", view)
	}

	// Now deliver detail asynchronously
	detail := &gmail.MessageDetail{
		MessageSummary: *summary,
		BodyHTML:       "<h2>Email contents arrived!</h2>",
	}
	m.SetDetail(detail)
	if m.Loading {
		t.Fatalf("expected loading to be false after SetDetail")
	}

	updatedView := m.View()
	if !strings.Contains(updatedView, "Email contents arrived!") {
		t.Errorf("expected rendered content in updated view, got:\n%s", updatedView)
	}
}

