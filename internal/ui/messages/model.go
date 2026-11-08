package messages

import (
	"fmt"
	"maps"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tkilb/lazymail/internal/gmail"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

// SelectMessageMsg is emitted when cursor lands on a message to load its preview.
type SelectMessageMsg struct {
	MessageID string
}

// LoadMoreMessagesMsg is emitted when cursor scrolls near the end to trigger background loading of older emails.
type LoadMoreMessagesMsg struct{}

// VisualMode says whether a range is being selected, unselected, or neither (Yazi-style).
type VisualMode int

const (
	VisualOff VisualMode = iota
	VisualSelect
	VisualUnselect
)

// VisualState anchors a visual-mode range.
type VisualState struct {
	Mode   VisualMode
	Anchor int
	Base   map[string]bool
}

const (
	markerBar = "▎"
)

// Model represents Panel 2: Messages
type Model struct {
	Items       []*gmail.MessageSummary
	Cursor      int
	Width       int
	Height      int
	Focused     bool
	Loading     bool
	LoadingMore bool
	TotalCount  int

	Selected map[string]bool
	Visual   VisualState
}

// New initializes an empty messages model.
func New() Model {
	return Model{
		Items:  []*gmail.MessageSummary{},
		Cursor: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m *Model) SetMessages(messages []*gmail.MessageSummary, totalCount int) {
	m.Items = messages
	m.TotalCount = totalCount
	m.LoadingMore = false
	m.Selected = nil
	m.Visual = VisualState{}
	if m.Cursor >= len(messages) {
		m.Cursor = 0
	}
}

func (m *Model) AppendMessages(more []*gmail.MessageSummary, totalCount int) {
	m.Items = append(m.Items, more...)
	m.TotalCount = totalCount
	m.LoadingMore = false
}

func (m Model) CurrentMessage() *gmail.MessageSummary {
	if len(m.Items) == 0 || m.Cursor >= len(m.Items) {
		return nil
	}
	return m.Items[m.Cursor]
}

// GetSelectedOrCurrentIDs returns all multi-selected message IDs, or the current single message ID under cursor.
func (m Model) GetSelectedOrCurrentIDs() []string {
	if len(m.Selected) > 0 {
		var ids []string
		for _, item := range m.Items {
			if m.Selected[item.ID] {
				ids = append(ids, item.ID)
			}
		}
		if len(ids) > 0 {
			return ids
		}
	}
	if curr := m.CurrentMessage(); curr != nil {
		return []string{curr.ID}
	}
	return nil
}

// ClearSelection resets any active visual mode or multi-selected rows.
func (m *Model) ClearSelection() {
	m.Selected = nil
	m.Visual = VisualState{}
}

// ToggleHovered flips the selection of the row under the cursor and moves down, matching Yazi space.
func (m *Model) ToggleHovered() tea.Cmd {
	if m.Visual.Mode != VisualOff || len(m.Items) == 0 || m.Cursor >= len(m.Items) {
		return nil
	}
	id := m.Items[m.Cursor].ID
	m.Selected = withID(m.Selected, id, !m.Selected[id])
	return m.move(1)
}

// StartVisual enters visual select or unselect mode anchored at the cursor.
func (m *Model) StartVisual(mode VisualMode) {
	if len(m.Items) == 0 || m.Cursor >= len(m.Items) {
		return
	}
	if m.Visual.Mode == mode {
		m.Visual = VisualState{}
		return
	}
	m.Visual = VisualState{Mode: mode, Anchor: m.Cursor, Base: m.Selected}
	m.applyVisual()
}

// SelectAll selects all currently loaded messages.
func (m *Model) SelectAll() {
	m.Visual = VisualState{}
	all := make(map[string]bool, len(m.Items))
	for _, item := range m.Items {
		all[item.ID] = true
	}
	m.Selected = all
}

// Escape clears visual mode first, then clears selection. Returns true if something was cleared.
func (m *Model) Escape() bool {
	switch {
	case m.Visual.Mode != VisualOff:
		m.Visual = VisualState{}
		return true
	case len(m.Selected) > 0:
		m.Selected = nil
		return true
	default:
		return false
	}
}

func (m *Model) applyVisual() {
	if m.Visual.Mode == VisualOff {
		return
	}
	lo, hi := min(m.Visual.Anchor, m.Cursor), max(m.Visual.Anchor, m.Cursor)
	sel := maps.Clone(m.Visual.Base)
	if sel == nil {
		sel = map[string]bool{}
	}
	for i := lo; i <= hi && i < len(m.Items); i++ {
		id := m.Items[i].ID
		if m.Visual.Mode == VisualSelect {
			sel[id] = true
		} else {
			delete(sel, id)
		}
	}
	m.Selected = sel
}

func withID(set map[string]bool, id string, on bool) map[string]bool {
	c := maps.Clone(set)
	if c == nil {
		c = map[string]bool{}
	}
	if on {
		c[id] = true
	} else {
		delete(c, id)
	}
	return c
}

func (m *Model) move(delta int) tea.Cmd {
	if len(m.Items) == 0 {
		return nil
	}
	target := min(max(m.Cursor+delta, 0), len(m.Items)-1)
	if target == m.Cursor && delta != 0 {
		if delta > 0 && !m.LoadingMore && (m.TotalCount == 0 || len(m.Items) < m.TotalCount) {
			m.LoadingMore = true
			return func() tea.Msg { return LoadMoreMessagesMsg{} }
		}
		return nil
	}
	m.Cursor = target
	m.applyVisual()

	cmds := []tea.Cmd{m.emitSelectMsg()}
	if m.Cursor >= len(m.Items)-5 && !m.LoadingMore && (m.TotalCount == 0 || len(m.Items) < m.TotalCount) {
		m.LoadingMore = true
		cmds = append(cmds, func() tea.Msg { return LoadMoreMessagesMsg{} })
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.Focused || len(m.Items) == 0 {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			cmd := m.move(1)
			return m, cmd

		case "k", "up":
			cmd := m.move(-1)
			return m, cmd

		case " ":
			cmd := m.ToggleHovered()
			return m, cmd

		case "v":
			m.StartVisual(VisualSelect)
			return m, nil

		case "V":
			m.StartVisual(VisualUnselect)
			return m, nil

		case "ctrl+a":
			m.SelectAll()
			return m, nil

		case "esc":
			if m.Escape() {
				return m, nil
			}

		case "g", "home":
			cmd := m.move(-m.Cursor)
			return m, cmd

		case "G", "end":
			cmd := m.move(len(m.Items) - 1 - m.Cursor)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) emitSelectMsg() tea.Cmd {
	if curr := m.CurrentMessage(); curr != nil {
		return func() tea.Msg {
			return SelectMessageMsg{MessageID: curr.ID}
		}
	}
	return nil
}

func (m Model) View() string {
	var lines []string

	if m.Loading && len(m.Items) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(panel.InfoColor).Render("Loading messages..."))
	} else if len(m.Items) == 0 {
		lines = append(lines, lipgloss.NewStyle().Faint(true).Render("No messages found."))
	} else {
		// Calculate scrolling window
		innerWidth, innerHeight := panel.InnerSize(m.Width, m.Height)
		visibleLines := innerHeight
		if m.LoadingMore {
			visibleLines--
		}
		if visibleLines < 1 {
			visibleLines = 1
		}

		scrollWindow := panel.CenteredScrollWindow
		if m.Focused {
			scrollWindow = panel.ScrollWindow
		}
		start, end := scrollWindow(m.Cursor, len(m.Items), visibleLines)

		unreadStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
		faintStyle := lipgloss.NewStyle().Faint(true)
		tagStyle := lipgloss.NewStyle().Foreground(panel.HighlightColor)
		markerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // Yazi yellow marker

		for i := start; i < end; i++ {
			item := m.Items[i]

			// Left marker bar (Yazi style)
			marker := " "
			if m.Selected != nil && m.Selected[item.ID] {
				marker = markerStyle.Render(markerBar)
			}

			// Cursor pointer
			pointer := " "
			if i == m.Cursor && m.Focused {
				pointer = "❯"
			}

			// Unsubscribe badge
			unsubBadge := " "
			if item.HasUnsubscribe {
				unsubBadge = tagStyle.Render("⚡")
			}

			// Clean sender name
			fromName, _ := gmail.ParseSender(item.From)
			if panel.StringWidth(fromName) > 18 {
				fromName = panel.Truncate(fromName, 18)
			}
			fromPad := 18 - panel.StringWidth(fromName)
			if fromPad < 0 {
				fromPad = 0
			}
			fromField := fromName + strings.Repeat(" ", fromPad)

			// Subject formatting
			subject := item.Subject
			if subject == "" {
				subject = "(no subject)"
			}
			maxSubjLen := innerWidth - 23
			if maxSubjLen < 1 {
				maxSubjLen = 1
			}
			if panel.StringWidth(subject) > maxSubjLen {
				subject = panel.Truncate(subject, maxSubjLen)
			}

			subjPad := innerWidth - 23 - panel.StringWidth(subject)
			if subjPad < 0 {
				subjPad = 0
			}

			rawLine := fmt.Sprintf("%s%s%s %s %s%s", marker, pointer, unsubBadge, fromField, subject, strings.Repeat(" ", subjPad))

			var itemLine string
			if i == m.Cursor && m.Focused {
				itemLine = panel.SelectedRowStyle.Render(rawLine)
			} else if item.IsUnread {
				itemLine = unreadStyle.Render(rawLine)
			} else {
				itemLine = faintStyle.Render(rawLine)
			}

			lines = append(lines, itemLine)
		}

		if m.LoadingMore {
			lines = append(lines, lipgloss.NewStyle().Faint(true).Render("  … loading older emails …"))
		}
	}

	content := strings.Join(lines, "\n")

	var statusParts []string
	if m.Visual.Mode == VisualSelect {
		statusParts = append(statusParts, "VISUAL")
	} else if m.Visual.Mode == VisualUnselect {
		statusParts = append(statusParts, "UNSELECT")
	}
	if n := len(m.Selected); n > 0 {
		statusParts = append(statusParts, fmt.Sprintf("%d sel", n))
	}

	title := "[2] Messages"
	if len(statusParts) > 0 {
		title = fmt.Sprintf("[2] Messages [%s]", strings.Join(statusParts, " "))
	}
	if m.Loading && len(m.Items) > 0 {
		title += fmt.Sprintf(" (%d - syncing...)", len(m.Items))
	} else if m.TotalCount > 0 {
		title += fmt.Sprintf(" (%d/%d)", len(m.Items), m.TotalCount)
	} else if len(m.Items) > 0 {
		title += fmt.Sprintf(" (%d)", len(m.Items))
	}
	footer := ""
	if len(m.Items) > 0 {
		footer = fmt.Sprintf("%d of %d", m.Cursor+1, len(m.Items))
	}
	return panel.Render(title, content, m.Width, m.Height, m.Focused, footer)
}

