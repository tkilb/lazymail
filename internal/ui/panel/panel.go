package panel

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Panel identifiers matching the specification grid
const (
	PanelPreview  = 0
	PanelStatus   = 1
	PanelMessages = 2
	PanelFolders  = 3
	PanelSenders  = 4
)

// Lazytask ANSI color palette
const (
	paletteDefault = lipgloss.Color("")
	paletteGreen   = lipgloss.Color("2") // lazytask activeBorderColor
	paletteBlue    = lipgloss.Color("4") // lazytask selectedLineBgColor
	paletteRed     = lipgloss.Color("1") // error / warning
	paletteYellow  = lipgloss.Color("3") // notice
	paletteCyan    = lipgloss.Color("6") // info / tags
)

var (
	UnfocusedColor        = paletteDefault
	FocusedColor          = paletteGreen
	SelectedRowBackground = paletteBlue
	WarningColor          = paletteRed
	InfoColor             = paletteCyan
	HighlightColor        = paletteGreen
	NoticeColor           = paletteYellow

	NormalBorderColor = UnfocusedColor
	FocusBorderColor  = FocusedColor

	SelectedRowStyle = lipgloss.NewStyle().
				Background(SelectedRowBackground).
				Foreground(lipgloss.Color("15")).
				Bold(true)
)

// Truncate truncates a string (handling ANSI escape codes, wide chars, and emojis)
// to width cells, appending "…" if truncated.
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

// StringWidth returns the visual cell width of a string, ignoring ANSI codes.
func StringWidth(s string) int {
	return ansi.StringWidth(s)
}

// TitleStyle returns the title style for a panel.
func TitleStyle(focused bool) lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true)
	if focused {
		return style.Foreground(FocusedColor)
	}
	return style.Foreground(UnfocusedColor)
}

func borderColor(focused bool) lipgloss.Color {
	if focused {
		return FocusedColor
	}
	return UnfocusedColor
}

// InnerSize converts a panel's total outer footprint (border included) into
// the content width/height available inside the border.
func InnerSize(outerWidth, outerHeight int) (width, height int) {
	width = outerWidth - 2
	if width < 4 {
		width = 4
	}
	height = outerHeight - 2
	if height < 1 {
		height = 1
	}
	return width, height
}

// ScrollWindow returns the [start, end) slice bounds of a scrollable list of
// totalItems entries, sized so exactly visibleRows are shown, such that
// cursor always falls within [start, end). It matches lazytask's ScrollWindow.
func ScrollWindow(cursor, totalItems, visibleRows int) (start, end int) {
	if visibleRows < 1 {
		visibleRows = 1
	}
	if totalItems <= visibleRows {
		return 0, totalItems
	}

	maxStart := totalItems - visibleRows
	start = cursor - visibleRows + 1
	if start < 0 {
		start = 0
	}
	if start > maxStart {
		start = maxStart
	}
	end = start + visibleRows
	return start, end
}

// CenteredScrollWindow keeps cursor in the middle of visible rows (clamped
// to the list's ends). It matches lazytask's CenteredScrollWindow, used by
// minimized panels so context above and below the selection is visible.
func CenteredScrollWindow(cursor, totalItems, visibleRows int) (start, end int) {
	if visibleRows < 1 {
		visibleRows = 1
	}
	if totalItems <= visibleRows {
		return 0, totalItems
	}
	start = min(max(cursor-visibleRows/2, 0), totalItems-visibleRows)
	return start, start + visibleRows
}

// Render renders a bordered panel with the title embedded into the top border line
// and an optional scroll position footer in the bottom border (lazytask style).
// Every rendered line is strictly guaranteed to be exactly outerWidth cells wide.
func Render(title string, body string, outerWidth, outerHeight int, focused bool, footer ...string) string {
	width, height := InnerSize(outerWidth, outerHeight)

	lineStyle := lipgloss.NewStyle().Foreground(borderColor(focused))
	top := topBorder(title, width, focused, lineStyle)
	bottom := bottomBorder(firstFooter(footer), width, lineStyle)

	content := lipgloss.NewStyle().Width(width).Height(height).Render(body)

	var b strings.Builder
	b.WriteString(top)
	b.WriteString("\n")
	side := lineStyle.Render("│")
	lines := strings.Split(content, "\n")
	for i := 0; i < height; i++ {
		b.WriteString(side)
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		lw := ansi.StringWidth(line)
		if lw < width {
			line += strings.Repeat(" ", width-lw)
		} else if lw > width {
			line = ansi.Truncate(line, width, "")
			if lw2 := ansi.StringWidth(line); lw2 < width {
				line += strings.Repeat(" ", width-lw2)
			}
		}
		b.WriteString(line)
		b.WriteString(side)
		b.WriteString("\n")
	}
	b.WriteString(bottom)
	return b.String()
}

func firstFooter(footer []string) string {
	if len(footer) == 0 {
		return ""
	}
	return footer[0]
}

func bottomBorder(footer string, width int, lineStyle lipgloss.Style) string {
	const trailDashes = 1
	maxFooter := width - trailDashes
	if footer == "" || maxFooter < 1 || ansi.StringWidth(footer) > maxFooter {
		return lineStyle.Render("╰" + strings.Repeat("─", max(width, 0)) + "╯")
	}

	footerWidth := ansi.StringWidth(footer)
	fill := width - trailDashes - footerWidth
	if fill < 0 {
		fill = 0
	}

	var b strings.Builder
	b.WriteString(lineStyle.Render("╰" + strings.Repeat("─", fill)))
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("")).Render(footer))
	b.WriteString(lineStyle.Render(strings.Repeat("─", trailDashes) + "╯"))
	return b.String()
}

func topBorder(title string, width int, focused bool, lineStyle lipgloss.Style) string {
	const leadDashes = 2
	maxLabel := width - leadDashes
	if maxLabel < 1 {
		return lineStyle.Render("╭" + strings.Repeat("─", max(width, 0)) + "╮")
	}

	label := titleLabel(title)
	if ansi.StringWidth(label) > maxLabel {
		label = ansi.Truncate(label, maxLabel, "…")
	}

	labelWidth := ansi.StringWidth(label)
	fill := width - leadDashes - labelWidth
	if fill < 0 {
		fill = 0
	}

	var b strings.Builder
	b.WriteString(lineStyle.Render("╭" + strings.Repeat("─", leadDashes)))
	b.WriteString(TitleStyle(focused).Render(label))
	b.WriteString(lineStyle.Render(strings.Repeat("─", fill) + "╮"))
	return b.String()
}

func titleLabel(title string) string {
	title = strings.TrimSpace(title)
	if strings.HasPrefix(title, "[") && strings.Contains(title, "] ") {
		idx := strings.Index(title, "] ")
		return title[:idx+1] + "-" + title[idx+2:]
	}
	if len(title) > 2 && title[1] == ' ' && title[0] >= '0' && title[0] <= '9' {
		return "[" + string(title[0]) + "]-" + title[2:]
	}
	return title
}
