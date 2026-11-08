package folders

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

// FolderItem represents a mailbox or smart view option.
type FolderItem struct {
	ID    string
	Title string
	Query string
	Count int
}

// Model represents Panel 3: Folders
type Model struct {
	Items    []FolderItem
	Cursor   int
	Selected int
	Width    int
	Height   int
	Focused  bool
}

// SelectFolderMsg is emitted when a folder is selected or navigated to.
type SelectFolderMsg struct {
	Item FolderItem
}

// New initializes the folder panel with standard mailboxes and smart views.
func New() Model {
	items := []FolderItem{
		{ID: "INBOX", Title: "Inbox", Query: "label:INBOX"},
		{ID: "SUBSCRIPTIONS", Title: "Subscriptions", Query: "label:INBOX unsubscribe"},
		{ID: "ALL", Title: "All Mail", Query: ""},
		{ID: "SENT", Title: "Sent", Query: "label:SENT"},
		{ID: "TRASH", Title: "Trash", Query: "label:TRASH"},
	}

	return Model{
		Items:    items,
		Cursor:   0,
		Selected: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m *Model) SetFolders(items []FolderItem) {
	m.Items = items
	if m.Cursor >= len(items) {
		m.Cursor = 0
	}
	if m.Selected >= len(items) {
		m.Selected = 0
	}
}

func (m Model) CurrentItem() FolderItem {
	if len(m.Items) == 0 {
		return FolderItem{}
	}
	return m.Items[m.Cursor]
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.Focused {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.Cursor < len(m.Items)-1 {
				m.Cursor++
			}
			return m, nil
		case "k", "up":
			if m.Cursor > 0 {
				m.Cursor--
			}
			return m, nil
		case "enter":
			m.Selected = m.Cursor
			return m, func() tea.Msg {
				return SelectFolderMsg{Item: m.Items[m.Cursor]}
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	var lines []string

	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(panel.HighlightColor)

	innerWidth, innerHeight := panel.InnerSize(m.Width, m.Height)
	visibleLines := innerHeight
	if visibleLines < 1 {
		visibleLines = 1
	}

	scrollWindow := panel.CenteredScrollWindow
	if m.Focused {
		scrollWindow = panel.ScrollWindow
	}
	start, end := scrollWindow(m.Cursor, len(m.Items), visibleLines)

	for i := start; i < end; i++ {
		item := m.Items[i]
		prefix := "  "
		if i == m.Cursor && m.Focused {
			prefix = "❯ "
		} else if i == m.Selected {
			prefix = "• "
		}

		maxTitle := innerWidth - 2
		if maxTitle < 0 {
			maxTitle = 0
		}
		title := item.Title
		if panel.StringWidth(title) > maxTitle {
			title = panel.Truncate(title, maxTitle)
		}

		pad := innerWidth - 2 - panel.StringWidth(title)
		if pad < 0 {
			pad = 0
		}

		rawLine := fmt.Sprintf("%s%s%s", prefix, title, strings.Repeat(" ", pad))
		var line string
		if i == m.Cursor && m.Focused {
			line = panel.SelectedRowStyle.Render(rawLine)
		} else if i == m.Selected {
			line = selectedStyle.Render(rawLine)
		} else {
			line = rawLine
		}

		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	footer := ""
	if len(m.Items) > 0 {
		footer = fmt.Sprintf("%d of %d", m.Cursor+1, len(m.Items))
	}
	return panel.Render("[3] Folders", content, m.Width, m.Height, m.Focused, footer)
}
