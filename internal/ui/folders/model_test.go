package folders

import (
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

func TestFoldersModel_Navigation(t *testing.T) {
	m := New()
	m.Focused = true

	if m.CurrentItem().ID != "INBOX" {
		t.Fatalf("expected initial item to be INBOX, got %s", m.CurrentItem().ID)
	}

	// Move down ('j') - moves cursor only without command
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.CurrentItem().ID != "SUBSCRIPTIONS" {
		t.Errorf("expected cursor to be SUBSCRIPTIONS, got %s", m.CurrentItem().ID)
	}
	if cmd != nil {
		t.Errorf("expected no command on navigation until enter is pressed")
	}

	// Press Enter to select
	m, enterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if enterCmd == nil {
		t.Fatalf("expected command on enter")
	}
	msg := enterCmd()
	sel, ok := msg.(SelectFolderMsg)
	if !ok || sel.Item.ID != "SUBSCRIPTIONS" {
		t.Errorf("expected SelectFolderMsg for SUBSCRIPTIONS, got %v", msg)
	}

	// Move back up ('k')
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.CurrentItem().ID != "INBOX" {
		t.Errorf("expected cursor to be INBOX, got %s", m.CurrentItem().ID)
	}
}
