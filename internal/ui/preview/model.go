package preview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tkilb/lazymail/internal/gmail"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

// Model represents Panel 0: Email Body Preview
type Model struct {
	Message  *gmail.MessageDetail
	Viewport viewport.Model
	Width    int
	Height   int
	Focused  bool
	Ready    bool
}

// New initializes an empty preview model.
func New() Model {
	return Model{}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m *Model) SetMessage(msg *gmail.MessageDetail) {
	m.Message = msg
	if m.Ready {
		m.Viewport.SetContent(m.formatBody())
		m.Viewport.GotoTop()
	}
}

func (m *Model) SetDimensions(width, height int) {
	m.Width = width
	m.Height = height

	vpWidth := width - 4
	if vpWidth < 10 {
		vpWidth = 10
	}
	vpHeight := height - 9 // Reserve space for headers and panel borders
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

	if m.Message != nil {
		m.Viewport.SetContent(m.formatBody())
	}
}

func (m Model) formatBody() string {
	if m.Message == nil {
		return "No message selected."
	}

	body := m.Message.BodyPlain
	if body == "" && m.Message.BodyHTML != "" {
		body = gmail.HTMLToPlainText(m.Message.BodyHTML)
	}
	if body == "" {
		body = m.Message.Snippet
	}

	// Handle previously cached raw HTML bodies
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(strings.ToLower(trimmed), "<!doctype") ||
		strings.HasPrefix(strings.ToLower(trimmed), "<html") ||
		strings.HasPrefix(strings.ToLower(trimmed), "<div") ||
		strings.HasPrefix(strings.ToLower(trimmed), "<head") ||
		strings.HasPrefix(strings.ToLower(trimmed), "<table") {
		body = gmail.HTMLToPlainText(body)
	}

	return body
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.Focused {
		return m, nil
	}

	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.Viewport.LineDown(1)
		case "k", "up":
			m.Viewport.LineUp(1)
		case "d":
			m.Viewport.HalfViewDown()
		case "u":
			// If not captured by unsubscribe at parent level, scroll up
			m.Viewport.HalfViewUp()
		}
	}

	m.Viewport, cmd = m.Viewport.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	var headerLines []string

	if m.Message == nil {
		headerLines = append(headerLines, lipgloss.NewStyle().Faint(true).Render("Select a message to view its contents."))
		return panel.Render("[0] Email Body", strings.Join(headerLines, "\n"), m.Width, m.Height, m.Focused)
	}

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(panel.InfoColor)
	headerLines = append(headerLines, fmt.Sprintf("%s %s", labelStyle.Render("Subject:"), m.Message.Subject))
	headerLines = append(headerLines, fmt.Sprintf("%s %s", labelStyle.Render("From:   "), m.Message.From))
	headerLines = append(headerLines, fmt.Sprintf("%s %s", labelStyle.Render("Date:   "), m.Message.Date))

	if m.Message.HasUnsubscribe {
		unsubStyle := lipgloss.NewStyle().Bold(true).Foreground(panel.HighlightColor)
		unsubType := "URL"
		if m.Message.Unsubscribe.IsOneClick {
			unsubType = "One-Click RFC 8058"
		} else if m.Message.Unsubscribe.Mailto != "" {
			unsubType = "Mailto"
		}
		headerLines = append(headerLines, fmt.Sprintf("%s %s [press 'u' to trigger]", unsubStyle.Render("⚡ Unsubscribe:"), unsubType))
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
		headerLines = append(headerLines, fmt.Sprintf("%s %s", labelStyle.Render("Attachments:"), attachStyle.Render(strings.Join(attachStrs, ", "))))
	}

	divider := lipgloss.NewStyle().Foreground(panel.NormalBorderColor).Render(strings.Repeat("─", m.Width-4))
	headerLines = append(headerLines, divider)

	fullContent := strings.Join(headerLines, "\n") + "\n" + m.Viewport.View()
	return panel.Render("[0] Email Body", fullContent, m.Width, m.Height, m.Focused)
}
