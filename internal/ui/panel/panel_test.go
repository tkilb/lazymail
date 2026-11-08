package panel

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	rendered := Render("[1] Status", "Account: test@example.com", 30, 10, false)
	if !strings.Contains(rendered, "[1]-Status") {
		t.Errorf("rendered panel missing title: %s", rendered)
	}
	if !strings.Contains(rendered, "test@example.com") {
		t.Errorf("rendered panel missing content: %s", rendered)
	}

	focused := Render("[1] Status", "Account: test@example.com", 30, 10, true)
	if !strings.Contains(focused, "[1]-Status") {
		t.Errorf("focused panel missing title: %s", focused)
	}

	withFooter := Render("[2] Messages", "test", 30, 10, true, "1 of 25")
	if !strings.Contains(withFooter, "1 of 25") {
		t.Errorf("rendered panel missing footer: %s", withFooter)
	}
}

func TestScrollWindow(t *testing.T) {
	// Fits without scrolling
	start, end := ScrollWindow(0, 3, 10)
	if start != 0 || end != 3 {
		t.Errorf("got start=%d end=%d, want 0, 3", start, end)
	}

	// Cursor at top of scrollable list
	start, end = ScrollWindow(0, 20, 5)
	if start != 0 || end != 5 {
		t.Errorf("got start=%d end=%d, want 0, 5", start, end)
	}

	// Cursor scrolls down
	start, end = ScrollWindow(9, 20, 5)
	if start != 5 || end != 10 {
		t.Errorf("got start=%d end=%d, want 5, 10", start, end)
	}

	// Cursor near end clamps
	start, end = ScrollWindow(19, 20, 5)
	if start != 15 || end != 20 {
		t.Errorf("got start=%d end=%d, want 15, 20", start, end)
	}
}

func TestCenteredScrollWindow(t *testing.T) {
	// Fits without scrolling
	start, end := CenteredScrollWindow(2, 3, 10)
	if start != 0 || end != 3 {
		t.Errorf("got start=%d end=%d, want 0, 3", start, end)
	}

	// Cursor in middle
	start, end = CenteredScrollWindow(10, 20, 5)
	if start != 8 || end != 13 {
		t.Errorf("got start=%d end=%d, want 8, 13", start, end)
	}

	// Clamps at ends
	start, end = CenteredScrollWindow(0, 20, 5)
	if start != 0 || end != 5 {
		t.Errorf("got start=%d end=%d, want 0, 5", start, end)
	}

	start, end = CenteredScrollWindow(19, 20, 5)
	if start != 15 || end != 20 {
		t.Errorf("got start=%d end=%d, want 15, 20", start, end)
	}
}

func TestRender_WidthBounds(t *testing.T) {
	// Long title that would overflow without proper truncation (e.g. sender email)
	longTitle := "[4] a-very-very-long-email-address-that-exceeds-width@some-domain.com"
	overflowBody := "This line is extremely long and would wrap uncontrollably across multiple columns\nShort\nAnother very long line here"
	outerWidth := 25
	outerHeight := 6

	rendered := Render(longTitle, overflowBody, outerWidth, outerHeight, true, "100 of 100")
	lines := strings.Split(rendered, "\n")
	if len(lines) != outerHeight {
		t.Fatalf("expected exactly %d lines, got %d", outerHeight, len(lines))
	}

	for idx, line := range lines {
		w := StringWidth(line)
		if w != outerWidth {
			t.Errorf("line %d visual width = %d, want exact outerWidth = %d (line: %q)", idx, w, outerWidth, line)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("hello world", 8); got != "hello w…" {
		t.Errorf("Truncate('hello world', 8) = %q, want 'hello w…'", got)
	}
	if got := Truncate("short", 10); got != "short" {
		t.Errorf("Truncate('short', 10) = %q, want 'short'", got)
	}
	if got := Truncate("abc", 0); got != "" {
		t.Errorf("Truncate('abc', 0) = %q, want ''", got)
	}
}
