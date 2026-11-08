package messages

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tkilb/lazymail/internal/gmail"
	"testing"
)

func TestMessagesModel_CursorNavigation(t *testing.T) {
	m := New()
	m.Focused = true
	m.Items = []*gmail.MessageSummary{
		{ID: "m1", Subject: "Email 1", From: "a@test.com"},
		{ID: "m2", Subject: "Email 2", From: "b@test.com"},
		{ID: "m3", Subject: "Email 3", From: "c@test.com"},
	}

	// Default cursor is on m1
	ids := m.GetSelectedOrCurrentIDs()
	if len(ids) != 1 || ids[0] != "m1" {
		t.Errorf("expected [m1], got %v", ids)
	}

	// Move cursor down (j)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.Cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", m.Cursor)
	}
	ids = m.GetSelectedOrCurrentIDs()
	if len(ids) != 1 || ids[0] != "m2" {
		t.Errorf("expected [m2], got %v", ids)
	}

	// Move cursor up (k)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.Cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.Cursor)
	}
}

func TestMessagesModel_InfiniteScroll(t *testing.T) {
	m := New()
	initial := []*gmail.MessageSummary{
		{ID: "m1", Subject: "Email 1"},
		{ID: "m2", Subject: "Email 2"},
	}
	m.SetMessages(initial, 10)
	if len(m.Items) != 2 || m.TotalCount != 10 {
		t.Fatalf("expected 2 items, total 10; got %d, %d", len(m.Items), m.TotalCount)
	}

	more := []*gmail.MessageSummary{
		{ID: "m3", Subject: "Email 3"},
		{ID: "m4", Subject: "Email 4"},
	}
	m.AppendMessages(more, 10)
	if len(m.Items) != 4 || m.TotalCount != 10 {
		t.Fatalf("expected 4 items, total 10; got %d, %d", len(m.Items), m.TotalCount)
	}
}

func TestMessagesModel_SpaceSelection(t *testing.T) {
	m := New()
	m.Focused = true
	m.Items = []*gmail.MessageSummary{
		{ID: "m1", Subject: "Email 1"},
		{ID: "m2", Subject: "Email 2"},
		{ID: "m3", Subject: "Email 3"},
	}

	// Space toggles m1 and moves down to m2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.Cursor != 1 {
		t.Errorf("expected cursor to advance to 1, got %d", m.Cursor)
	}
	if !m.Selected["m1"] {
		t.Errorf("expected m1 to be selected")
	}

	// Space toggles m2 and moves down to m3
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.Cursor != 2 {
		t.Errorf("expected cursor to advance to 2, got %d", m.Cursor)
	}
	if !m.Selected["m2"] {
		t.Errorf("expected m2 to be selected")
	}

	ids := m.GetSelectedOrCurrentIDs()
	if len(ids) != 2 || ids[0] != "m1" || ids[1] != "m2" {
		t.Errorf("expected [m1, m2], got %v", ids)
	}

	// Escape clears selection
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.Selected) != 0 {
		t.Errorf("expected selection to be cleared, got %v", m.Selected)
	}
	// Fall back to cursor item
	currIDs := m.GetSelectedOrCurrentIDs()
	if len(currIDs) != 1 || currIDs[0] != "m3" {
		t.Errorf("expected [m3], got %v", currIDs)
	}
}

func TestMessagesModel_VisualMode(t *testing.T) {
	m := New()
	m.Focused = true
	m.Items = []*gmail.MessageSummary{
		{ID: "m1", Subject: "Email 1"},
		{ID: "m2", Subject: "Email 2"},
		{ID: "m3", Subject: "Email 3"},
		{ID: "m4", Subject: "Email 4"},
	}

	// Enter visual mode at cursor 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.Visual.Mode != VisualSelect {
		t.Fatalf("expected VisualSelect mode")
	}

	// Move cursor down 2 steps to m3
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.Cursor != 2 {
		t.Fatalf("expected cursor 2, got %d", m.Cursor)
	}

	// m1, m2, m3 should all be selected
	if !m.Selected["m1"] || !m.Selected["m2"] || !m.Selected["m3"] || m.Selected["m4"] {
		t.Errorf("expected m1, m2, m3 selected; got %v", m.Selected)
	}

	// First escape leaves visual mode but keeps selection
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.Visual.Mode != VisualOff {
		t.Errorf("expected visual mode off")
	}
	if len(m.Selected) != 3 {
		t.Errorf("expected 3 items selected after visual exit, got %d", len(m.Selected))
	}

	// Second escape clears selection
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.Selected) != 0 {
		t.Errorf("expected selection cleared, got %d", len(m.Selected))
	}
}

func TestMessagesModel_SelectAll(t *testing.T) {
	m := New()
	m.Focused = true
	m.Items = []*gmail.MessageSummary{
		{ID: "m1", Subject: "Email 1"},
		{ID: "m2", Subject: "Email 2"},
		{ID: "m3", Subject: "Email 3"},
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if len(m.Selected) != 3 {
		t.Fatalf("expected 3 selected items, got %d", len(m.Selected))
	}

	ids := m.GetSelectedOrCurrentIDs()
	if len(ids) != 3 {
		t.Errorf("expected 3 IDs, got %d", len(ids))
	}
}

