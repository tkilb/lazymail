package statusbar

import (
	"strings"
	"testing"
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
	if strings.Contains(view, "\n") {
		t.Errorf("expected status bar to be strictly a single line, but found newline: %q", view)
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
