package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

func TestLeftColumnHeights(t *testing.T) {
	tests := []struct {
		name         string
		total        int
		activePanel  int
		wantStatus   int
		wantMessages int
		wantBottom   int
	}{
		{
			name:         "Messages focused at 39 rows",
			total:        39,
			activePanel:  panel.PanelMessages,
			wantStatus:   3,
			wantMessages: 29, // 36 - 7
			wantBottom:   7,
		},
		{
			name:         "Folders focused at 39 rows expands bottom row",
			total:        39,
			activePanel:  panel.PanelFolders,
			wantStatus:   3,
			wantMessages: 15, // max(10, 39*0.4) = 15
			wantBottom:   21, // 36 - 15
		},
		{
			name:         "Senders focused at 39 rows expands bottom row",
			total:        39,
			activePanel:  panel.PanelSenders,
			wantStatus:   3,
			wantMessages: 15,
			wantBottom:   21,
		},
		{
			name:         "Preview focused keeps bottom collapsed",
			total:        39,
			activePanel:  panel.PanelPreview,
			wantStatus:   3,
			wantMessages: 29,
			wantBottom:   7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, messages, bottom := leftColumnHeights(tt.total, tt.activePanel)
			if status != tt.wantStatus {
				t.Errorf("statusHeight = %d, want %d", status, tt.wantStatus)
			}
			if messages != tt.wantMessages {
				t.Errorf("messagesHeight = %d, want %d", messages, tt.wantMessages)
			}
			if bottom != tt.wantBottom {
				t.Errorf("bottomHeight = %d, want %d", bottom, tt.wantBottom)
			}
			if status+messages+bottom != tt.total {
				t.Errorf("sum = %d, want %d", status+messages+bottom, tt.total)
			}
		})
	}
}

func TestDynamicLayoutFocusExpansion(t *testing.T) {
	m := NewAppModel(nil, nil, "")
	// Resize window to 100x40 (bodyHeight = 39, leftWidth = 50)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	app := model.(AppModel)

	// Default focus is PanelMessages
	if app.ActivePanel != panel.PanelMessages {
		t.Fatalf("expected ActivePanel to be PanelMessages, got %d", app.ActivePanel)
	}
	if app.FoldersPanel.Height != 7 {
		t.Errorf("expected FoldersPanel.Height == 7 when collapsed, got %d", app.FoldersPanel.Height)
	}
	if app.SendersPanel.Height != 7 {
		t.Errorf("expected SendersPanel.Height == 7 when collapsed, got %d", app.SendersPanel.Height)
	}
	if app.MessagesPanel.Height != 29 {
		t.Errorf("expected MessagesPanel.Height == 29 when expanded, got %d", app.MessagesPanel.Height)
	}

	// Switch focus to Folders (key "3")
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	app = model.(AppModel)

	if app.ActivePanel != panel.PanelFolders {
		t.Fatalf("expected ActivePanel to be PanelFolders, got %d", app.ActivePanel)
	}
	if app.FoldersPanel.Height != 21 {
		t.Errorf("expected FoldersPanel.Height to expand to 21, got %d", app.FoldersPanel.Height)
	}
	if app.SendersPanel.Height != 21 {
		t.Errorf("expected SendersPanel.Height to expand to 21, got %d", app.SendersPanel.Height)
	}
	if app.MessagesPanel.Height != 15 {
		t.Errorf("expected MessagesPanel.Height to shrink to floor 15, got %d", app.MessagesPanel.Height)
	}
	// Folders width should be 50% of leftWidth (50 / 2 = 25)
	if app.FoldersPanel.Width != 25 {
		t.Errorf("expected FoldersPanel.Width == 25, got %d", app.FoldersPanel.Width)
	}

	// Switch focus to Senders (key "4")
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	app = model.(AppModel)

	if app.ActivePanel != panel.PanelSenders {
		t.Fatalf("expected ActivePanel to be PanelSenders, got %d", app.ActivePanel)
	}
	if app.FoldersPanel.Height != 21 {
		t.Errorf("expected FoldersPanel.Height to remain expanded at 21, got %d", app.FoldersPanel.Height)
	}
	if app.SendersPanel.Height != 21 {
		t.Errorf("expected SendersPanel.Height to remain expanded at 21, got %d", app.SendersPanel.Height)
	}
	// Senders width should expand to take remaining width (50 - 16 = 34)
	if app.SendersPanel.Width <= app.FoldersPanel.Width {
		t.Errorf("expected SendersPanel.Width (%d) to be larger than FoldersPanel.Width (%d)", app.SendersPanel.Width, app.FoldersPanel.Width)
	}

	// Switch focus back to Messages (key "2")
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	app = model.(AppModel)

	if app.ActivePanel != panel.PanelMessages {
		t.Fatalf("expected ActivePanel to be PanelMessages, got %d", app.ActivePanel)
	}
	if app.FoldersPanel.Height != 7 {
		t.Errorf("expected FoldersPanel.Height to collapse back to 7, got %d", app.FoldersPanel.Height)
	}
	if app.SendersPanel.Height != 7 {
		t.Errorf("expected SendersPanel.Height to collapse back to 7, got %d", app.SendersPanel.Height)
	}
	if app.MessagesPanel.Height != 29 {
		t.Errorf("expected MessagesPanel.Height to expand back to 29, got %d", app.MessagesPanel.Height)
	}
}
