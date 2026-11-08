package status

import (
	"strings"
	"testing"
)

func TestStatusModel_SingleLineFlex(t *testing.T) {
	m := New()
	m.Width = 60
	m.Height = 3 // 1 content line + 2 border lines
	m.SyncState = "Connected"
	m.TotalMessages = 150
	m.UnreadMessages = 12

	view := m.View()
	lines := strings.Split(view, "\n")

	// Must be exactly 3 lines (top border, 1 content line, bottom border)
	if len(lines) != 3 {
		t.Fatalf("expected exactly 3 lines in status panel (1 content + 2 border), got %d:\n%s", len(lines), view)
	}

	contentLine := lines[1]

	// Must contain status on left and totals on right
	if !strings.Contains(contentLine, "Status:") || !strings.Contains(contentLine, "Connected") {
		t.Errorf("expected content line to contain status, got %q", contentLine)
	}
	if !strings.Contains(contentLine, "150 total") || !strings.Contains(contentLine, "12 unread") {
		t.Errorf("expected content line to contain message totals, got %q", contentLine)
	}

	// Verify flex space between: Status comes before Totals with space in between
	idxStatus := strings.Index(contentLine, "Status:")
	idxTotals := strings.Index(contentLine, "150 total")
	if idxStatus >= idxTotals {
		t.Errorf("expected status to precede totals with space between, got status at %d and totals at %d", idxStatus, idxTotals)
	}
}

func TestStatusModel_OperationState(t *testing.T) {
	m := New()
	m.Width = 50
	m.Height = 3
	m.OperationState = "Archiving 3 messages..."
	m.TotalMessages = 50
	m.UnreadMessages = 0

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected exactly 3 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[1], "Archiving 3 messages...") {
		t.Errorf("expected operation state in content line, got %q", lines[1])
	}
}
