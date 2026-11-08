package helppopup

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

// Render generates the centered help modal dialog.
func Render(width, height int) string {
	boxWidth := 60
	if boxWidth > width-4 {
		boxWidth = width - 4
	}

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(panel.HighlightColor)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(panel.InfoColor)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F8F8F2"))

	var content []string
	content = append(content, titleStyle.Render("── LazyMail Keybindings & Navigation ──"), "")

	content = append(content, titleStyle.Render("Global Navigation:"))
	content = append(content,
		keyStyle.Render("  0-4        ")+"Direct jump to panel (0:Preview, 1:Status, 2:Messages, 3:Folders, 4:Senders)",
		keyStyle.Render("  Tab / S-Tab")+"Cycle forward / backward through panels",
		keyStyle.Render("  ?          ")+"Toggle this Help popup",
		keyStyle.Render("  q / Ctrl+C ")+"Quit LazyMail",
		"",
	)

	content = append(content, titleStyle.Render("Panel 2 (Messages) & Decluttering:"))
	content = append(content,
		keyStyle.Render("  j / k      ")+"Navigate up / down email list",
		keyStyle.Render("  o / Enter  ")+"Open highlighted email in full-screen Reader mode",
		keyStyle.Render("  Space      ")+"Toggle selection of email and move down (Yazi-style)",
		keyStyle.Render("  v / V      ")+"Visual range select / unselect mode",
		keyStyle.Render("  Ctrl+A     ")+"Select all loaded emails",
		keyStyle.Render("  Esc        ")+"Exit visual mode / clear selection / clear filter",
		keyStyle.Render("  e / a      ")+"Archive selected or current message",
		keyStyle.Render("  d          ")+"Trash selected or current (restore if in Trash)",
		keyStyle.Render("  R          ")+"Restore selected or current (when inside Trash)",
		keyStyle.Render("  D (Shift+D)")+"Permanently delete (guarded: enabled ONLY inside Trash)",
		keyStyle.Render("  f          ")+"Filter all messages by sender of currently highlighted email",
		keyStyle.Render("  u          ")+"Trigger one-click unsubscribe / open link in browser",
		keyStyle.Render("  r          ")+"Refresh current mailbox",
		"",
	)

	content = append(content, titleStyle.Render("Panel 0 (Preview):"))
	content = append(content,
		keyStyle.Render("  j / k      ")+"Scroll message body line down / up",
		keyStyle.Render("  d / u      ")+"Page down / half page up",
		keyStyle.Render("  o          ")+"Open email in full-screen Reader mode",
		keyStyle.Render("  u          ")+"Execute unsubscribe action",
		"",
	)

	content = append(content, descStyle.Render("Press [?] or [Esc] to close this help window."))

	modalBox := lipgloss.NewStyle().
		BorderStyle(lipgloss.DoubleBorder()).
		BorderForeground(panel.FocusBorderColor).
		Padding(1, 2).
		Width(boxWidth).
		Render(strings.Join(content, "\n"))

	// Center modal overlay inside available terminal space
	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		modalBox,
	)
}
