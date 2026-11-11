package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tkilb/lazymail/internal/gmail"
	"github.com/tkilb/lazymail/internal/ui/folders"
	"github.com/tkilb/lazymail/internal/ui/panel"
)

func TestAppModel_ViewLineLengths(t *testing.T) {
	widths := []int{80, 100, 108, 120, 140, 160}
	heights := []int{30, 33, 40, 50}

	for _, w := range widths {
		for _, h := range heights {
			app := NewAppModel(nil, nil, "")
			m, _ := app.Update(tea.WindowSizeMsg{Width: w, Height: h})
			app = m.(AppModel)

			// Switch to PanelSenders
			m, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
			app = m.(AppModel)

			// Populate Folders
			app.FoldersPanel.SetFolders([]folders.FolderItem{
				{ID: "INBOX", Title: "Inbox"},
				{ID: "SUBSCRIPTIONS", Title: "Subscriptions"},
				{ID: "ALL", Title: "All Mail"},
				{ID: "SENT", Title: "Sent"},
				{ID: "TRASH", Title: "Trash"},
			})

			// Populate Senders with screenshot senders
			app.SendersPanel.SetSenders([]*gmail.SenderSummary{
				{DisplayName: "Amazon Health", Email: "hello@email.health.amazon.com", TotalCount: 2},
				{DisplayName: "Amazon Web Services", Email: "aws@amazon.com", TotalCount: 2},
				{DisplayName: "Amazon.com", Email: "auto-confirm@amazon.com", TotalCount: 8},
				{DisplayName: "Amazon.com", Email: "order-update@amazon.com", TotalCount: 9},
				{DisplayName: "Amazon.com", Email: "shipment-tracking@amazon.com", TotalCount: 1},
				{DisplayName: "Apple", Email: "apple@apple.com", TotalCount: 1},
				{DisplayName: "Betterment Statements", Email: "statements@betterment.com", TotalCount: 14},
				{DisplayName: "Bitwarden", Email: "no-reply@bitwarden.com", TotalCount: 1},
				{DisplayName: "blizzard entertainment, inc.", Email: "noreply@blizzard.com", TotalCount: 1},
				{DisplayName: "Colleen A Collier", Email: "colleen@collier.com", TotalCount: 3},
			}, 17105)
			app.SendersPanel.Cursor = 1
			app.SendersPanel.Selected = 1

			// Populate Messages
			app.MessagesPanel.SetMessages([]*gmail.MessageSummary{
				{ID: "1", From: "Amazon Health <hello@email.health.amazon.com>", Subject: "Expert GLP-1 support from home"},
				{ID: "2", From: "Amazon Health <hello@email.health.amazon.com>", Subject: "Special Prime member benefit: on-demand care"},
			}, 2)

			// Populate Preview
			app.PreviewPanel.SetMessage(&gmail.MessageDetail{
				MessageSummary: gmail.MessageSummary{
					ID:             "1",
					From:           "Amazon Health <hello@email.health.amazon.com>",
					Subject:        "Expert GLP-1 support from home",
					Date:           "Tue, 29 Sep 2026 10:35:27 -0400",
					HasUnsubscribe: true,
				},
				BodyPlain: "Expert GLP-1 support from home\n•\n•\nUse On-Demand Care for weight loss support\n$14.99/mo\n=C2=A0\nOn-Demand Care for weight loss\nWe make it easy to manage your weight\nGet started\nHealth AI makes it easy for you to consult a clinician",
				Unsubscribe: &gmail.UnsubscribeInfo{
					IsOneClick: true,
					HTTPURL:    "https://amazon.com/unsub",
				},
			})

			view := app.View()
			if w == 108 && h == 33 {
				t.Logf("=== VIEW w=108 h=33 ===\n%s\n=== END VIEW ===", view)
			}
			lines := strings.Split(view, "\n")
			if len(lines) > h {
				t.Errorf("w=%d h=%d: total lines = %d > height %d", w, h, len(lines), h)
			}
			for i, line := range lines {
				lw := panel.StringWidth(line)
				if w == 108 && h == 33 {
					t.Logf("Line %2d: lw=%d (w=%d) %q", i, lw, w, line)
				}
				if lw > w {
					t.Errorf("w=%d h=%d line %d: width = %d > terminal width %d (line: %s)", w, h, i, lw, w, line)
				}
			}
		}
	}
}
