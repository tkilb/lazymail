package statusbar

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

// Model represents the bottom status bar and keybinding hints.
type Model struct {
	ActivePanel   int
	CurrentFolder string
	Notification  string
	ErrorMessage  string
	Width         int
}

// New initializes the status bar model.
func New() Model {
	return Model{}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	return m, nil
}

func (m Model) View() string {
	if m.Width <= 0 {
		return ""
	}

	hintKey := lipgloss.NewStyle().Bold(true).Foreground(panel.HighlightColor)
	hintDesc := lipgloss.NewStyle().Faint(true)

	// Available printable width inside horizontal padding (1 left, 1 right)
	availWidth := m.Width - 2
	if availWidth < 10 {
		availWidth = 10
	}

	var banner string
	if m.ErrorMessage != "" {
		errStyle := lipgloss.NewStyle().
			Bold(true).
			Background(panel.WarningColor).
			Foreground(lipgloss.Color("#000000")).
			Padding(0, 1)
		msg := m.ErrorMessage
		maxMsgLen := availWidth - 12
		if maxMsgLen < 5 {
			maxMsgLen = 5
		}
		if len(msg) > maxMsgLen {
			msg = msg[:maxMsgLen-1] + "…"
		}
		banner = errStyle.Render("ERROR: " + msg)
	} else if m.Notification != "" {
		notifStyle := lipgloss.NewStyle().
			Bold(true).
			Background(panel.InfoColor).
			Foreground(lipgloss.Color("#000000")).
			Padding(0, 1)
		msg := m.Notification
		maxMsgLen := availWidth - 12
		if maxMsgLen < 5 {
			maxMsgLen = 5
		}
		if len(msg) > maxMsgLen {
			msg = msg[:maxMsgLen-1] + "…"
		}
		banner = notifStyle.Render(msg)
	}

	remWidth := availWidth
	if banner != "" {
		remWidth = availWidth - lipgloss.Width(banner) - 3 // space for " | "
		if remWidth < 0 {
			remWidth = 0
		}
	}

	// Candidate hints in priority order
	var candidates []string
	candidates = append(candidates, fmt.Sprintf("%s %s", hintKey.Render("[0-4/Tab]"), hintDesc.Render("Panels")))

	switch m.ActivePanel {
	case panel.PanelMessages:
		candidates = append(candidates,
			fmt.Sprintf("%s %s", hintKey.Render("[Space]"), hintDesc.Render("Select")),
			fmt.Sprintf("%s %s", hintKey.Render("[A]"), hintDesc.Render("All")),
			fmt.Sprintf("%s %s", hintKey.Render("[o]"), hintDesc.Render("Reader")),
		)
		if m.CurrentFolder == "TRASH" {
			candidates = append(candidates,
				fmt.Sprintf("%s %s", hintKey.Render("[d/R]"), hintDesc.Render("Restore")),
				fmt.Sprintf("%s %s", hintKey.Render("[D]"), hintDesc.Render("Purge")),
			)
		} else {
			candidates = append(candidates,
				fmt.Sprintf("%s %s", hintKey.Render("[e/a]"), hintDesc.Render("Archive")),
				fmt.Sprintf("%s %s", hintKey.Render("[d]"), hintDesc.Render("Trash")),
				fmt.Sprintf("%s %s", hintKey.Render("[f]"), hintDesc.Render("Filter")),
				fmt.Sprintf("%s %s", hintKey.Render("[u]"), hintDesc.Render("Unsub")),
			)
		}
	case panel.PanelFolders:
		candidates = append(candidates,
			fmt.Sprintf("%s %s", hintKey.Render("[Enter]"), hintDesc.Render("Open")),
		)
	case panel.PanelSenders:
		candidates = append(candidates,
			fmt.Sprintf("%s %s", hintKey.Render("[Enter]"), hintDesc.Render("Filter")),
		)
	case panel.PanelPreview:
		candidates = append(candidates,
			fmt.Sprintf("%s %s", hintKey.Render("[o]"), hintDesc.Render("Reader")),
			fmt.Sprintf("%s %s", hintKey.Render("[d/u]"), hintDesc.Render("Scroll")),
			fmt.Sprintf("%s %s", hintKey.Render("[u]"), hintDesc.Render("Unsub")),
		)
	}

	candidates = append(candidates,
		fmt.Sprintf("%s %s", hintKey.Render("[?]"), hintDesc.Render("Help")),
		fmt.Sprintf("%s %s", hintKey.Render("[q]"), hintDesc.Render("Quit")),
	)

	// Greedily append only hints that fit within remWidth without wrapping
	var fittedHints []string
	currentWidth := 0
	for _, h := range candidates {
		hWidth := lipgloss.Width(h)
		extra := 0
		if len(fittedHints) > 0 {
			extra = 2 // "  " separator
		}
		if currentWidth+extra+hWidth <= remWidth {
			fittedHints = append(fittedHints, h)
			currentWidth += extra + hWidth
		}
	}

	var bar string
	if banner != "" && len(fittedHints) > 0 {
		bar = banner + " | " + strings.Join(fittedHints, "  ")
	} else if banner != "" {
		bar = banner
	} else {
		bar = strings.Join(fittedHints, "  ")
	}

	rendered := lipgloss.NewStyle().
		Width(m.Width).
		MaxHeight(1).
		Padding(0, 1).
		Render(bar)

	// Guarantee single line strictly (never wrap to 2 lines)
	if lines := strings.Split(rendered, "\n"); len(lines) > 0 {
		return lines[0]
	}
	return rendered
}
