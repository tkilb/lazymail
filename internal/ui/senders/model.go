package senders

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tkilb/lazymail/internal/gmail"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

// SelectSenderMsg is emitted when a sender is chosen to filter messages.
type SelectSenderMsg struct {
	Sender *gmail.SenderSummary
}

// Model represents Panel 4: Senders
type Model struct {
	Senders      []*gmail.SenderSummary
	Cursor       int
	Selected     int
	Width        int
	Height       int
	Focused      bool
	Crawling     bool
	CrawlPercent int
}

// PeekAllSendersMsg is emitted when the user requests a full crawl of all senders.
type PeekAllSendersMsg struct{}

// New initializes an empty senders panel model with '(all)' as the default filter option.
func New() Model {
	return Model{
		Senders: []*gmail.SenderSummary{
			{DisplayName: "(all)", Email: ""},
		},
		Cursor:   0,
		Selected: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

// SetFolderTotal sets the total count on the '(all)' item at index 0.
func (m *Model) SetFolderTotal(total int) {
	if len(m.Senders) > 0 && m.Senders[0].Email == "" {
		m.Senders[0].TotalCount = total
	}
}

// UpdateSenderCount sets the exact server count for a specific sender email address.
func (m *Model) UpdateSenderCount(email string, count int) {
	if email == "" {
		m.SetFolderTotal(count)
		return
	}
	for _, s := range m.Senders {
		if strings.EqualFold(s.Email, email) {
			s.TotalCount = count
			return
		}
	}
}

// SetSenders configures the sender list, keeping '(all)' at index 0.
// If folderTotal is provided, it is assigned as the count for '(all)'.
func (m *Model) SetSenders(senders []*gmail.SenderSummary, folderTotal ...int) {
	currentEmail := ""
	if m.Cursor >= 0 && m.Cursor < len(m.Senders) {
		currentEmail = m.Senders[m.Cursor].Email
	}
	selectedEmail := ""
	if m.Selected >= 0 && m.Selected < len(m.Senders) {
		selectedEmail = m.Senders[m.Selected].Email
	}

	allItem := &gmail.SenderSummary{
		DisplayName: "(all)",
		Email:       "",
	}
	if len(folderTotal) > 0 && folderTotal[0] > 0 {
		allItem.TotalCount = folderTotal[0]
	} else {
		total := 0
		for _, s := range senders {
			if s != nil && s.Email != "" {
				total += s.TotalCount
			}
		}
		allItem.TotalCount = total
	}

	list := []*gmail.SenderSummary{allItem}
	for _, s := range senders {
		if s != nil && s.Email != "" {
			list = append(list, s)
		}
	}
	m.Senders = list

	m.SortAlphabetical()

	if currentEmail != "" {
		found := false
		for i, s := range m.Senders {
			if strings.EqualFold(s.Email, currentEmail) {
				m.Cursor = i
				found = true
				break
			}
		}
		if !found && m.Cursor >= len(m.Senders) {
			m.Cursor = 0
		}
	} else if m.Cursor >= len(m.Senders) {
		m.Cursor = 0
	}

	if selectedEmail != "" {
		found := false
		for i, s := range m.Senders {
			if strings.EqualFold(s.Email, selectedEmail) {
				m.Selected = i
				found = true
				break
			}
		}
		if !found && m.Selected >= len(m.Senders) {
			m.Selected = 0
		}
	} else if m.Selected >= len(m.Senders) {
		m.Selected = 0
	}
}

// SortAlphabetical sorts the sender list (excluding index 0 '(all)') alphabetically (A to Z)
// by sender display name or email, preserving the current cursor and selected positions.
func (m *Model) SortAlphabetical() {
	if len(m.Senders) <= 2 {
		return
	}

	currentEmail := ""
	if m.Cursor >= 0 && m.Cursor < len(m.Senders) {
		currentEmail = m.Senders[m.Cursor].Email
	}
	selectedEmail := ""
	if m.Selected >= 0 && m.Selected < len(m.Senders) {
		selectedEmail = m.Senders[m.Selected].Email
	}

	sort.SliceStable(m.Senders[1:], func(i, j int) bool {
		keyI := gmail.SenderSortKey(m.Senders[1+i])
		keyJ := gmail.SenderSortKey(m.Senders[1+j])
		if keyI != keyJ {
			return keyI < keyJ
		}
		return strings.ToLower(m.Senders[1+i].Email) < strings.ToLower(m.Senders[1+j].Email)
	})

	if currentEmail != "" {
		for i, s := range m.Senders {
			if strings.EqualFold(s.Email, currentEmail) {
				m.Cursor = i
				break
			}
		}
	}
	if selectedEmail != "" {
		for i, s := range m.Senders {
			if strings.EqualFold(s.Email, selectedEmail) {
				m.Selected = i
				break
			}
		}
	}
}

// MergeSenders merges new sender summaries into the existing list without wiping out existing counts,
// updating counts when newer or larger information is found, and keeping senders sorted alphabetically.
func (m *Model) MergeSenders(newSenders []*gmail.SenderSummary, folderTotal int) {
	if len(m.Senders) <= 1 {
		m.SetSenders(newSenders, folderTotal)
		return
	}

	existing := make(map[string]*gmail.SenderSummary, len(m.Senders))
	for _, s := range m.Senders {
		if s != nil && s.Email != "" {
			existing[strings.ToLower(s.Email)] = s
		}
	}

	for _, ns := range newSenders {
		if ns == nil || ns.Email == "" {
			continue
		}
		key := strings.ToLower(ns.Email)
		if cur, exists := existing[key]; exists {
			if ns.TotalCount > cur.TotalCount {
				cur.TotalCount = ns.TotalCount
			}
			if ns.DisplayName != "" && cur.DisplayName == cur.Email {
				cur.DisplayName = ns.DisplayName
			}
		} else {
			existing[key] = ns
			m.Senders = append(m.Senders, ns)
		}
	}

	if folderTotal > 0 && len(m.Senders) > 0 && m.Senders[0].Email == "" {
		m.Senders[0].TotalCount = folderTotal
	}

	m.SortAlphabetical()
}

// SelectEmail marks the specified email as selected, or resets to 'All' (index 0) if empty.
func (m *Model) SelectEmail(email string) {
	if email == "" {
		m.Selected = 0
		return
	}
	for i, s := range m.Senders {
		if strings.EqualFold(s.Email, email) {
			m.Selected = i
			return
		}
	}
	m.Selected = -1
}

func (m Model) CurrentSender() *gmail.SenderSummary {
	if len(m.Senders) == 0 || m.Cursor >= len(m.Senders) {
		return nil
	}
	return m.Senders[m.Cursor]
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.Focused || len(m.Senders) == 0 {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.Cursor < len(m.Senders)-1 {
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
				return SelectSenderMsg{Sender: m.Senders[m.Cursor]}
			}
		case "B":
			return m, func() tea.Msg {
				return PeekAllSendersMsg{}
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

	if len(m.Senders) == 0 {
		lines = append(lines, lipgloss.NewStyle().Faint(true).Render("No senders"))
	} else {
		scrollWindow := panel.CenteredScrollWindow
		if m.Focused {
			scrollWindow = panel.ScrollWindow
		}
		start, end := scrollWindow(m.Cursor, len(m.Senders), visibleLines)

		for i := start; i < end; i++ {
			s := m.Senders[i]
			prefix := "  "
			if i == m.Cursor && m.Focused {
				prefix = "❯ "
			} else if i == m.Selected {
				prefix = "• "
			}

			countStr := fmt.Sprintf("(%d)", s.TotalCount)
			countWidth := panel.StringWidth(countStr)

			// Space available for sender label inside innerWidth:
			// innerWidth minus prefix (2), space before count (1), countWidth
			availWidth := innerWidth - 2 - 1 - countWidth
			if availWidth < 0 {
				availWidth = 0
			}

			displayName := strings.TrimSpace(s.DisplayName)
			var label string
			if s.Email == "" {
				label = "(all)"
			} else if displayName == "" || strings.EqualFold(displayName, s.Email) {
				label = s.Email
			} else {
				// We have both DisplayName and Email
				full := fmt.Sprintf("%s <%s>", displayName, s.Email)
				if panel.StringWidth(full) <= availWidth {
					label = full
				} else if i == m.Cursor && m.Focused {
					// On cursor row when focused, prioritize showing the email address
					if panel.StringWidth(s.Email) <= availWidth {
						label = s.Email
					} else {
						label = panel.Truncate(s.Email, availWidth)
					}
				} else {
					if panel.StringWidth(displayName) <= availWidth {
						label = displayName
					} else {
						label = panel.Truncate(displayName, availWidth)
					}
				}
			}

			if panel.StringWidth(label) > availWidth {
				label = panel.Truncate(label, availWidth)
			}

			pad := innerWidth - 2 - panel.StringWidth(label) - 1 - countWidth
			if pad < 0 {
				pad = 0
			}

			var line string
			if i == m.Cursor && m.Focused {
				line = panel.SelectedRowStyle.Render(fmt.Sprintf("%s%s%s %s", prefix, label, strings.Repeat(" ", pad), countStr))
			} else if i == m.Selected {
				line = selectedStyle.Render(fmt.Sprintf("%s%s", prefix, label)) + strings.Repeat(" ", pad) + " " + lipgloss.NewStyle().Faint(true).Render(countStr)
			} else {
				line = fmt.Sprintf("%s%s%s %s", prefix, label, strings.Repeat(" ", pad), lipgloss.NewStyle().Faint(true).Render(countStr))
			}
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")

	// Dynamic title displaying crawling progress or hovered sender's email address
	title := "[4] Senders"
	if m.Crawling {
		title = fmt.Sprintf("[4] Senders (%d%%)", m.CrawlPercent)
	} else if m.Focused && m.Cursor < len(m.Senders) && m.Senders[m.Cursor].Email != "" {
		email := m.Senders[m.Cursor].Email
		title = fmt.Sprintf("[4] %s", email)
	}

	footer := ""
	if len(m.Senders) > 0 {
		footer = fmt.Sprintf("%d of %d", m.Cursor+1, len(m.Senders))
	}

	return panel.Render(title, content, m.Width, m.Height, m.Focused, footer)
}
