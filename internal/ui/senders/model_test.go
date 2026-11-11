package senders

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tkilb/lazymail/internal/gmail"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

func TestSendersModel_NavigationAndSelect(t *testing.T) {
	m := New()
	m.Focused = true
	m.SetSenders([]*gmail.SenderSummary{
		{DisplayName: "Alice", Email: "alice@test.com", TotalCount: 10},
		{DisplayName: "Bob", Email: "bob@test.com", TotalCount: 5},
	})

	// Index 0 should be "(all)"
	if m.CurrentSender().DisplayName != "(all)" || m.CurrentSender().Email != "" {
		t.Fatalf("expected index 0 to be (all), got %+v", m.CurrentSender())
	}
	if m.Selected != 0 {
		t.Errorf("expected default Selected 0 for (all), got %d", m.Selected)
	}

	// Move down ('j') to Alice (index 1)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.CurrentSender().Email != "alice@test.com" {
		t.Errorf("expected cursor alice, got %s", m.CurrentSender().Email)
	}
	if cmd != nil {
		t.Errorf("expected no command on j navigation until enter")
	}

	// Move down ('j') to Bob (index 2)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.CurrentSender().Email != "bob@test.com" {
		t.Errorf("expected cursor bob, got %s", m.CurrentSender().Email)
	}

	// Press Enter to select Bob
	m, enterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if enterCmd == nil {
		t.Fatalf("expected command on enter")
	}
	msg := enterCmd()
	sel, ok := msg.(SelectSenderMsg)
	if !ok || sel.Sender.Email != "bob@test.com" {
		t.Errorf("expected SelectSenderMsg for bob, got %v", msg)
	}
	if m.Selected != 2 {
		t.Errorf("expected Selected index 2 for Bob, got %d", m.Selected)
	}

	// Move up to "(all)" and select
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.CurrentSender().DisplayName != "(all)" {
		t.Errorf("expected cursor at (all), got %s", m.CurrentSender().DisplayName)
	}
	m, allCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if allCmd == nil {
		t.Fatalf("expected command on selecting All")
	}
	allMsg := allCmd()
	allSel, ok := allMsg.(SelectSenderMsg)
	if !ok || allSel.Sender.Email != "" {
		t.Errorf("expected SelectSenderMsg with empty email for All, got %v", allMsg)
	}
	if m.Selected != 0 {
		t.Errorf("expected Selected index 0 for All, got %d", m.Selected)
	}

	// Test SelectEmail helper
	m.SelectEmail("alice@test.com")
	if m.Selected != 1 {
		t.Errorf("expected Selected 1 for alice, got %d", m.Selected)
	}
	m.SelectEmail("")
	if m.Selected != 0 {
		t.Errorf("expected Selected 0 for empty email, got %d", m.Selected)
	}
}

func TestSendersModel_FolderTotalAndSenderCount(t *testing.T) {
	m := New()
	m.SetSenders([]*gmail.SenderSummary{
		{DisplayName: "Alice", Email: "alice@test.com", TotalCount: 10},
	}, 150) // true folder total is 150

	if m.Senders[0].TotalCount != 150 {
		t.Fatalf("expected (all) count 150, got %d", m.Senders[0].TotalCount)
	}

	// Update specific sender count from IMAP search
	m.UpdateSenderCount("alice@test.com", 42)
	if m.Senders[1].TotalCount != 42 {
		t.Fatalf("expected Alice count 42 after update, got %d", m.Senders[1].TotalCount)
	}

	// SetFolderTotal helper
	m.SetFolderTotal(200)
	if m.Senders[0].TotalCount != 200 {
		t.Fatalf("expected (all) count 200, got %d", m.Senders[0].TotalCount)
	}

	// MergeSenders preserves existing higher counts
	m.MergeSenders([]*gmail.SenderSummary{
		{DisplayName: "Alice", Email: "alice@test.com", TotalCount: 5},   // lower count, should stay 42
		{DisplayName: "Charlie", Email: "charlie@test.com", TotalCount: 8}, // new sender
	}, 200)

	if m.Senders[1].TotalCount != 42 {
		t.Errorf("expected Alice count to remain 42, got %d", m.Senders[1].TotalCount)
	}
	if len(m.Senders) != 3 || m.Senders[2].Email != "charlie@test.com" {
		t.Errorf("expected Charlie appended as 3rd sender, got %+v", m.Senders)
	}
}

func TestSendersModel_EmailDisplayAndTitle(t *testing.T) {
	m := New()
	m.Width = 40
	m.Height = 10
	m.Focused = true
	m.SetSenders([]*gmail.SenderSummary{
		{DisplayName: "GitHub", Email: "notifications@github.com", TotalCount: 7},
	}, 100)

	// Move cursor to GitHub
	m.Cursor = 1
	view := m.View()

	// Should contain email address in view
	if !strings.Contains(view, "notifications@github.com") {
		t.Fatalf("expected view to contain email address, got:\n%s", view)
	}

	// Should show email in title
	if !strings.Contains(view, "notifications@github.com") {
		t.Fatalf("expected title to show hovered email, got:\n%s", view)
	}
}

func TestSendersModel_AlphabeticalSorting(t *testing.T) {
	m := New()
	m.SetSenders([]*gmail.SenderSummary{
		{DisplayName: "Zack", Email: "zack@test.com", TotalCount: 100},
		{DisplayName: "Bob", Email: "bob@test.com", TotalCount: 50},
		{DisplayName: "Alice", Email: "alice@test.com", TotalCount: 20},
	})

	// Index 0 is always "(all)", followed by Alice, Bob, Zack alphabetically
	if len(m.Senders) != 4 {
		t.Fatalf("expected 4 entries (all + 3 senders), got %d", len(m.Senders))
	}
	if m.Senders[0].DisplayName != "(all)" {
		t.Errorf("expected index 0 to be (all), got %s", m.Senders[0].DisplayName)
	}
	if m.Senders[1].Email != "alice@test.com" {
		t.Errorf("expected index 1 Alice, got %s", m.Senders[1].Email)
	}
	if m.Senders[2].Email != "bob@test.com" {
		t.Errorf("expected index 2 Bob, got %s", m.Senders[2].Email)
	}
	if m.Senders[3].Email != "zack@test.com" {
		t.Errorf("expected index 3 Zack, got %s", m.Senders[3].Email)
	}

	// Select Bob (index 2)
	m.Cursor = 2
	m.Selected = 2

	// Merge a new sender "Charlie" and "Aaron" which will shift indexes
	m.MergeSenders([]*gmail.SenderSummary{
		{DisplayName: "Charlie", Email: "charlie@test.com", TotalCount: 15},
		{DisplayName: "Aaron", Email: "aaron@test.com", TotalCount: 30},
	}, 215)

	// New alphabetical order:
	// 0: (all)
	// 1: Aaron
	// 2: Alice
	// 3: Bob
	// 4: Charlie
	// 5: Zack
	if len(m.Senders) != 6 {
		t.Fatalf("expected 6 senders after merge, got %d", len(m.Senders))
	}
	if m.Senders[1].Email != "aaron@test.com" ||
		m.Senders[2].Email != "alice@test.com" ||
		m.Senders[3].Email != "bob@test.com" ||
		m.Senders[4].Email != "charlie@test.com" ||
		m.Senders[5].Email != "zack@test.com" {
		var names []string
		for _, s := range m.Senders {
			names = append(names, s.DisplayName)
		}
		t.Fatalf("unexpected order after merge: %v", names)
	}

	// Bob was previously index 2, now should be index 3, and Cursor/Selected must follow Bob
	if m.Cursor != 3 {
		t.Errorf("expected Cursor to follow Bob to index 3, got %d", m.Cursor)
	}
	if m.Selected != 3 {
		t.Errorf("expected Selected to follow Bob to index 3, got %d", m.Selected)
	}
}

func TestSendersModel_ReproduceLayout(t *testing.T) {
	m := New()
	m.Width = 34
	m.Height = 21
	m.Focused = true
	m.SetSenders([]*gmail.SenderSummary{
		{DisplayName: "Amazon Health", Email: "hello@email.health.amazon.com", TotalCount: 2},
		{DisplayName: "Amazon Web Services", Email: "aws@amazon.com", TotalCount: 2},
		{DisplayName: "Amazon.com", Email: "auto-confirm@amazon.com", TotalCount: 8},
		{DisplayName: "Amazon.com", Email: "order-update@amazon.com", TotalCount: 9},
		{DisplayName: "Amazon.com", Email: "shipment-tracking@amazon.com", TotalCount: 1},
		{DisplayName: "Apple", Email: "apple@apple.com", TotalCount: 1},
		{DisplayName: "Betterment Statements", Email: "statements@betterment.com", TotalCount: 14},
	}, 17105)
	m.Cursor = 1
	m.Selected = 1

	view := m.View()
	lines := strings.Split(view, "\n")
	t.Logf("Total lines: %d", len(lines))
	for idx, line := range lines {
		w := panel.StringWidth(line)
		t.Logf("Line %2d (w=%2d): %s", idx, w, line)
		if w != m.Width {
			t.Errorf("line %d has width %d, want %d", idx, w, m.Width)
		}
	}
}

func TestSendersModel_PeekAllKeyAndCrawlView(t *testing.T) {
	m := New()
	m.Focused = true
	m.Width = 30
	m.Height = 10

	// Pressing Shift+B ('B') emits PeekAllSendersMsg
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	if cmd == nil {
		t.Fatalf("expected command on Shift+B keypress")
	}
	msg := cmd()
	if _, ok := msg.(PeekAllSendersMsg); !ok {
		t.Fatalf("expected PeekAllSendersMsg, got %T", msg)
	}

	// While Crawling is true, panel title displays progress percentage
	m.Crawling = true
	m.CrawlPercent = 42
	view := m.View()
	if !strings.Contains(view, "Senders (42%)") {
		t.Errorf("expected view title to contain Senders (42%%), got:\n%s", view)
	}
}

