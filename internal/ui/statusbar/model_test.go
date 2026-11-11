package statusbar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestStatusBar_View(t *testing.T) {
	m := New()
	m.Width = 80
	m.ErrorMessage = "Test Error Alert"

	view := m.View()
	if !strings.Contains(view, "ERROR: Test Error Alert") {
		t.Errorf("expected error message in status bar view, got: %s", view)
	}
	if !strings.Contains(view, "[0-4/Tab]") {
		t.Errorf("expected key hint in status bar view, got: %s", view)
	}
	sw := lipgloss.Width(view)
	t.Logf("m.Width=80, Width=%d", sw)
	if sw > m.Width {
		t.Errorf("statusbar width %d > m.Width %d", sw, m.Width)
	}
}

func TestStatusBar_NeverWraps(t *testing.T) {
	widths := []int{40, 60, 80, 100, 140, 200}
	notifications := []string{
		"",
		"Short notification",
		"Sender: Very Long Sender Name That Exceeds Many Columns <notifications-support-noreply@enterprise-platform.github.com> • 142 message(s)",
	}

	for _, w := range widths {
		for _, n := range notifications {
			m := New()
			m.Width = w
			m.Notification = n

			view := m.View()
			if strings.Contains(view, "\n") {
				t.Errorf("expected 0 newlines at width %d with notification %q, got: %q", w, n, view)
			}
		}
	}
}
