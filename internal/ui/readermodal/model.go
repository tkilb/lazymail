package readermodal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tkilb/lazymail/internal/gmail"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

// Model represents the full-screen reader modal.
type Model struct {
	Active   bool
	Loading  bool
	Message  *gmail.MessageDetail
	Viewport viewport.Model
	Width    int
	Height   int
	Ready    bool
}

// New initializes a new full-screen reader modal.
func New() Model {
	return Model{}
}

// Open activates the reader modal with the given message detail.
func (m *Model) Open(msg *gmail.MessageDetail, width, height int) {
	if msg == nil {
		return
	}
	m.Active = true
	m.Loading = false
	m.Message = msg
	m.SetDimensions(width, height)
	m.renderContent()
	m.Viewport.GotoTop()
}

// OpenLoading activates the reader modal instantly in a loading state using summary metadata.
func (m *Model) OpenLoading(summary *gmail.MessageSummary, width, height int) {
	if summary == nil {
		return
	}
	m.Active = true
	m.Loading = true
	m.Message = &gmail.MessageDetail{
		MessageSummary: *summary,
	}
	m.SetDimensions(width, height)
	loadingStyle := lipgloss.NewStyle().Faint(true).Foreground(panel.InfoColor)
	m.Viewport.SetContent(loadingStyle.Render("\n  ⏳ Fetching email contents from server..."))
	m.Viewport.GotoTop()
}

// SetDetail updates the active modal with the loaded message detail and renders its content.
func (m *Model) SetDetail(msg *gmail.MessageDetail) {
	if msg == nil {
		return
	}
	m.Loading = false
	m.Message = msg
	m.renderContent()
}

// Close deactivates the reader modal.
func (m *Model) Close() {
	m.Active = false
	m.Loading = false
	m.Message = nil
}

// SetDimensions configures viewport dimensions to occupy the full modal interior.
func (m *Model) SetDimensions(width, height int) {
	m.Width = width
	m.Height = height

	// Inner content space: subtract 4 for double borders and padding
	vpWidth := width - 6
	if vpWidth < 20 {
		vpWidth = 20
	}
	// Subtract header (subject, from, date, divider) and footer
	vpHeight := height - 8
	if vpHeight < 5 {
		vpHeight = 5
	}

	if !m.Ready {
		m.Viewport = viewport.New(vpWidth, vpHeight)
		m.Ready = true
	} else {
		m.Viewport.Width = vpWidth
		m.Viewport.Height = vpHeight
	}
}

func (m *Model) renderContent() {
	if m.Message == nil {
		m.Viewport.SetContent("No message to display.")
		return
	}

	// Prefer converting BodyHTML to Markdown for rich terminal formatting
	var md string
	if m.Message.BodyHTML != "" {
		md = gmail.HTMLToMarkdown(m.Message.BodyHTML)
	}
	if md == "" && m.Message.BodyPlain != "" {
		// If BodyPlain is already HTML, convert it
		trimmed := strings.TrimSpace(m.Message.BodyPlain)
		if strings.HasPrefix(strings.ToLower(trimmed), "<html") || strings.HasPrefix(strings.ToLower(trimmed), "<div") {
			md = gmail.HTMLToMarkdown(m.Message.BodyPlain)
		} else {
			md = m.Message.BodyPlain
		}
	}
	if md == "" {
		md = m.Message.Snippet
	}

	// Render through Glamour with word wrap matching modal viewport width
	wrapWidth := m.Viewport.Width
	if wrapWidth > 120 {
		wrapWidth = 120 // Prevent excessive line stretch on ultra-wide monitors
	}
	formatted := gmail.RenderGlamour(md, wrapWidth)
	m.Viewport.SetContent(formatted)
}

// Update handles scroll and close keys inside the reader modal.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "o":
			m.Close()
			return m, nil
		case "j", "down":
			m.Viewport.LineDown(1)
		case "k", "up":
			m.Viewport.LineUp(1)
		case "d", "pgdown":
			m.Viewport.HalfViewDown()
		case "u", "pgup":
			m.Viewport.HalfViewUp()
		case "g", "home":
			m.Viewport.GotoTop()
		case "G", "end":
			m.Viewport.GotoBottom()
		}
	}

	var cmd tea.Cmd
	m.Viewport, cmd = m.Viewport.Update(msg)
	return m, cmd
}

// View renders the full-screen modal overlay.
func (m Model) View() string {
	if !m.Active || m.Message == nil {
		return ""
	}

	boxWidth := m.Width - 4
	if boxWidth < 30 {
		boxWidth = 30
	}
	boxHeight := m.Height - 2
	if boxHeight < 10 {
		boxHeight = 10
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(panel.HighlightColor)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(panel.InfoColor)
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))
	faintStyle := lipgloss.NewStyle().Faint(true)

	// Modal Header
	subject := m.Message.Subject
	if subject == "" {
		subject = "(no subject)"
	}
	headerLine1 := fmt.Sprintf("%s %s", labelStyle.Render("Subject:"), titleStyle.Render(subject))
	headerLine2 := fmt.Sprintf(
		"%s %s   %s %s",
		labelStyle.Render("From:"), metaStyle.Render(m.Message.From),
		labelStyle.Render("Date:"), faintStyle.Render(m.Message.Date),
	)

	var headerLines []string
	headerLines = append(headerLines, headerLine1, headerLine2)

	if m.Message.HasUnsubscribe {
		unsubStyle := lipgloss.NewStyle().Bold(true).Foreground(panel.HighlightColor)
		headerLines = append(headerLines, fmt.Sprintf("%s %s", unsubStyle.Render("⚡ Unsubscribe available"), faintStyle.Render("(press 'u' in main view)")))
	}

	if len(m.Message.Attachments) > 0 {
		var attachStrs []string
		for _, a := range m.Message.Attachments {
			name := a.Filename
			if name == "" {
				name = "attachment"
			}
			if a.Size > 0 {
				attachStrs = append(attachStrs, fmt.Sprintf("%s (%s)", name, gmail.FormatByteSize(a.Size)))
			} else {
				attachStrs = append(attachStrs, name)
			}
		}
		attachStyle := lipgloss.NewStyle().Foreground(panel.HighlightColor)
		headerLines = append(headerLines, fmt.Sprintf("%s %s", labelStyle.Render("📎 Attachments:"), attachStyle.Render(strings.Join(attachStrs, ", "))))
	}

	divider := lipgloss.NewStyle().Foreground(panel.FocusBorderColor).Render(strings.Repeat("━", boxWidth-4))
	headerLines = append(headerLines, divider)

	// Viewport content
	bodyView := m.Viewport.View()

	// Modal Footer with scroll progress
	percent := int(m.Viewport.ScrollPercent() * 100)
	footerText := fmt.Sprintf(
		" [Reader Mode] [Esc/q/o: Close | j/k: Scroll | d/u: Page | g/G: Top/Bottom] %d%% ",
		percent,
	)
	footer := faintStyle.Render(footerText)

	innerContent := strings.Join(headerLines, "\n") + "\n" + bodyView + "\n" + footer

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.DoubleBorder()).
		BorderForeground(panel.FocusBorderColor).
		Padding(0, 1).
		Width(boxWidth).
		Height(boxHeight).
		Render(innerContent)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}
