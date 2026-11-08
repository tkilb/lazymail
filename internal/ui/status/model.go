package status

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

// Model represents Panel 1: Status
type Model struct {
	AccountEmail   string
	SyncState      string
	OperationState string
	TotalMessages  int
	UnreadMessages int
	Width          int
	Height         int
	Focused        bool
}

// New initializes a new status panel model.
func New() Model {
	return Model{
		SyncState:      "Idle",
		OperationState: "",
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	return m, nil
}

func (m Model) View() string {
	statusLabel := lipgloss.NewStyle().Faint(true).Render("Status: ")
	statusText := m.SyncState
	if m.OperationState != "" {
		statusText = m.OperationState
	}
	if statusText == "" {
		statusText = "Idle"
	}

	syncStyle := lipgloss.NewStyle().Foreground(panel.HighlightColor)
	if strings.Contains(strings.ToLower(statusText), "err") {
		syncStyle = lipgloss.NewStyle().Foreground(panel.WarningColor)
	} else if strings.Contains(strings.ToLower(statusText), "sync") || strings.Contains(strings.ToLower(statusText), "load") {
		syncStyle = lipgloss.NewStyle().Foreground(panel.InfoColor)
	}
	left := statusLabel + syncStyle.Render(statusText)

	unreadStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	totalStyle := lipgloss.NewStyle().Faint(true)

	var right string
	if m.TotalMessages > 0 {
		var unreadStr string
		if m.UnreadMessages > 0 {
			unreadStr = fmt.Sprintf(" (%s)", unreadStyle.Render(fmt.Sprintf("%d unread", m.UnreadMessages)))
		} else {
			unreadStr = fmt.Sprintf(" (%s)", totalStyle.Render("0 unread"))
		}
		right = fmt.Sprintf("%d total%s", m.TotalMessages, unreadStr)
	} else {
		right = totalStyle.Render("0 messages")
	}

	innerWidth, _ := panel.InnerSize(m.Width, m.Height)
	availContent := innerWidth - 2
	if availContent < 1 {
		availContent = 1
	}

	leftLen := panel.StringWidth(left)
	rightLen := panel.StringWidth(right)

	var line string
	if leftLen+rightLen <= availContent {
		space := availContent - leftLen - rightLen
		line = " " + left + strings.Repeat(" ", space) + right + " "
	} else if leftLen+1 < availContent {
		availRight := availContent - leftLen - 1
		line = " " + left + " " + panel.Truncate(right, availRight) + " "
	} else {
		line = " " + panel.Truncate(left, availContent) + " "
	}

	return panel.Render("[1] Status", line, m.Width, m.Height, m.Focused)
}
