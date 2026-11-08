package onboarding

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

// OnboardingCompleteMsg is sent when the user successfully fills out the form.
type OnboardingCompleteMsg struct {
	Email            string
	AppPassword      string
	MasterPassphrase string
}

const (
	inputEmail = iota
	inputAppPassword
	inputPassphrase
	inputConfirmPassphrase
	btnSubmit
	totalFocusable
)

// Model represents the interactive setup and credentials form.
type Model struct {
	Inputs       []textinput.Model
	FocusIndex   int
	ErrorMessage string
	Width        int
	Height       int
	Submitted    bool
}

// New creates an initialized onboarding form model with lazytask colors.
func New() Model {
	inputs := make([]textinput.Model, 4)

	inputs[inputEmail] = textinput.New()
	inputs[inputEmail].Placeholder = "you@gmail.com"
	inputs[inputEmail].Focus()
	inputs[inputEmail].Width = 56
	inputs[inputEmail].Prompt = "  Gmail Address:   "

	inputs[inputAppPassword] = textinput.New()
	inputs[inputAppPassword].Placeholder = "abcd efgh ijkl mnop"
	inputs[inputAppPassword].EchoMode = textinput.EchoPassword
	inputs[inputAppPassword].EchoCharacter = '•'
	inputs[inputAppPassword].Width = 56
	inputs[inputAppPassword].Prompt = "  App Password:    "

	inputs[inputPassphrase] = textinput.New()
	inputs[inputPassphrase].Placeholder = "Choose a master password to encrypt vault"
	inputs[inputPassphrase].EchoMode = textinput.EchoPassword
	inputs[inputPassphrase].EchoCharacter = '•'
	inputs[inputPassphrase].Width = 56
	inputs[inputPassphrase].Prompt = "  Master Password: "

	inputs[inputConfirmPassphrase] = textinput.New()
	inputs[inputConfirmPassphrase].Placeholder = "Re-enter master password"
	inputs[inputConfirmPassphrase].EchoMode = textinput.EchoPassword
	inputs[inputConfirmPassphrase].EchoCharacter = '•'
	inputs[inputConfirmPassphrase].Width = 56
	inputs[inputConfirmPassphrase].Prompt = "  Confirm Password:"

	return Model{
		Inputs:     inputs,
		FocusIndex: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit

		case "ctrl+s":
			return m.attemptSubmit()

		case "tab", "down":
			m.nextInput()
			return m, nil

		case "shift+tab", "up":
			m.prevInput()
			return m, nil

		case "enter":
			if m.FocusIndex == btnSubmit || m.FocusIndex == inputConfirmPassphrase {
				return m.attemptSubmit()
			}
			m.nextInput()
			return m, nil
		}
	}

	if m.FocusIndex < len(m.Inputs) {
		var cmd tea.Cmd
		m.Inputs[m.FocusIndex], cmd = m.Inputs[m.FocusIndex].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) attemptSubmit() (tea.Model, tea.Cmd) {
	email := strings.TrimSpace(m.Inputs[inputEmail].Value())
	appPassword := strings.TrimSpace(m.Inputs[inputAppPassword].Value())
	passphrase := strings.TrimSpace(m.Inputs[inputPassphrase].Value())
	confirm := strings.TrimSpace(m.Inputs[inputConfirmPassphrase].Value())

	if email == "" || !strings.Contains(email, "@") {
		m.ErrorMessage = "Please enter a valid Gmail address."
		m.focusInput(inputEmail)
		return m, nil
	}
	if appPassword == "" {
		m.ErrorMessage = "App Password cannot be empty."
		m.focusInput(inputAppPassword)
		return m, nil
	}
	if passphrase == "" {
		m.ErrorMessage = "Master Password cannot be empty."
		m.focusInput(inputPassphrase)
		return m, nil
	}
	if passphrase != confirm {
		m.ErrorMessage = "Master Passwords do not match."
		m.focusInput(inputConfirmPassphrase)
		return m, nil
	}

	m.Submitted = true
	return m, tea.Quit
}

func (m *Model) focusInput(index int) {
	for i := range m.Inputs {
		m.Inputs[i].Blur()
	}
	m.FocusIndex = index
	if m.FocusIndex < len(m.Inputs) {
		m.Inputs[m.FocusIndex].Focus()
	}
}

func (m *Model) nextInput() {
	m.focusInput((m.FocusIndex + 1) % totalFocusable)
}

func (m *Model) prevInput() {
	m.focusInput((m.FocusIndex + totalFocusable - 1) % totalFocusable)
}

func (m Model) View() string {
	boxWidth := 74
	if m.Width > 0 && boxWidth > m.Width-4 {
		boxWidth = m.Width - 4
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(panel.FocusedColor)
	guideTitle := lipgloss.NewStyle().Bold(true).Foreground(panel.InfoColor)
	guideStep := lipgloss.NewStyle().Foreground(lipgloss.Color(""))
	hintStyle := lipgloss.NewStyle().Faint(true)
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(panel.WarningColor)

	var lines []string
	lines = append(lines,
		titleStyle.Render("╭────────────────────────────────────────────────────────────────────────╮"),
		titleStyle.Render("│                   Welcome to LazyMail Setup                            │"),
		titleStyle.Render("╰────────────────────────────────────────────────────────────────────────╯"),
		"",
		guideTitle.Render("Quick Setup via Google App Password (Zero Google Cloud needed):"),
		guideStep.Render(" 1. Visit: https://myaccount.google.com/apppasswords"),
		guideStep.Render(" 2. Enter app name 'LazyMail' and click Create."),
		guideStep.Render(" 3. Copy the 16-character password into the field below."),
		guideStep.Render(" 4. Choose a Master Password to encrypt your vault (AES-256-GCM)."),
		guideStep.Render("    * No plaintext passwords ever touch your filesystem."),
		"",
	)

	if m.ErrorMessage != "" {
		lines = append(lines, errStyle.Render(" ⚠️  "+m.ErrorMessage), "")
	}

	for i := range m.Inputs {
		lines = append(lines, m.Inputs[i].View())
	}

	lines = append(lines, "")

	// Submit button
	btnText := " [ Connect & Launch LazyMail ] "
	if m.FocusIndex == btnSubmit {
		btnRendered := lipgloss.NewStyle().
			Bold(true).
			Background(panel.SelectedRowBackground).
			Foreground(lipgloss.Color("15")).
			Render(btnText)
		lines = append(lines, "  "+btnRendered)
	} else {
		btnRendered := lipgloss.NewStyle().
			Bold(true).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(panel.UnfocusedColor).
			Render(btnText)
		lines = append(lines, "  "+btnRendered)
	}

	lines = append(lines,
		"",
		hintStyle.Render(" [Tab / Down] Next field   [Shift+Tab / Up] Prev field   [Enter] Submit   [Esc] Quit"),
	)

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(panel.FocusedColor).
		Padding(1, 2).
		Width(boxWidth).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}
