package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tkilb/lazymail/internal/auth"
	"github.com/tkilb/lazymail/internal/cache"
	"github.com/tkilb/lazymail/internal/gmail"
	"github.com/tkilb/lazymail/internal/ui/folders"
	"github.com/tkilb/lazymail/internal/ui/helppopup"
	"github.com/tkilb/lazymail/internal/ui/messages"
	"github.com/tkilb/lazymail/internal/ui/panel"
	"github.com/tkilb/lazymail/internal/ui/preview"
	"github.com/tkilb/lazymail/internal/ui/readermodal"
	"github.com/tkilb/lazymail/internal/ui/senders"
	"github.com/tkilb/lazymail/internal/ui/status"
	"github.com/tkilb/lazymail/internal/ui/statusbar"
)

// AppModel is the root Bubble Tea model managing grid layout and panel coordination.
type AppModel struct {
	Client     *gmail.Client
	Cache      *cache.Cache
	Passphrase string
	previewSeq int

	// Panels
	StatusPanel   status.Model
	MessagesPanel messages.Model
	FoldersPanel  folders.Model
	SendersPanel  senders.Model
	PreviewPanel  preview.Model
	StatusBar     statusbar.Model
	ReaderModal   readermodal.Model

	ActivePanel int
	ShowHelp    bool

	// Permanent delete confirmation state
	ConfirmDelete   bool
	PendingPurgeIDs []string

	CurrentFolder string
	CurrentQuery  string

	Width  int
	Height int
}

// Messages for async operations
type MessagesLoadedMsg struct {
	Mailbox    string
	Query      string
	Messages   []*gmail.MessageSummary
	TotalCount int
	Err        error
}

type MoreMessagesLoadedMsg struct {
	Mailbox    string
	Query      string
	Messages   []*gmail.MessageSummary
	TotalCount int
	Err        error
}

type FoldersDiscoveredMsg struct {
	Folders []folders.FolderItem
	Err     error
}

type SendersSampledMsg struct {
	Folder        string
	Senders       []*gmail.SenderSummary
	TotalMessages int
	Err           error
}

type MessageDetailLoadedMsg struct {
	Detail *gmail.MessageDetail
	Err    error
}

type OpenReaderMsg struct {
	Detail *gmail.MessageDetail
	Err    error
}

type ActionDoneMsg struct {
	Notification string
	Err          error
}

type DebouncePreviewMsg struct {
	MessageID string
	Seq       int
}

// NewAppModel creates a new root application model.
func NewAppModel(client *gmail.Client, cacheStore *cache.Cache, passphrase string) AppModel {
	m := AppModel{
		Client:        client,
		Cache:         cacheStore,
		Passphrase:    passphrase,
		StatusPanel:   status.New(),
		MessagesPanel: messages.New(),
		FoldersPanel:  folders.New(),
		SendersPanel:  senders.New(),
		PreviewPanel:  preview.New(),
		StatusBar:     statusbar.New(),
		ReaderModal:   readermodal.New(),
		ActivePanel:   panel.PanelMessages,
		CurrentFolder: "INBOX",
		CurrentQuery:  "",
	}

	if client != nil {
		m.StatusPanel.AccountEmail = client.Email
		m.StatusPanel.SyncState = "Syncing..."
	}

	// Pre-load from encrypted cache instantly (0ms)
	if cacheStore != nil {
		cachedMsgs := cacheStore.GetMessages("INBOX")
		if len(cachedMsgs) > 0 {
			if client != nil {
				for _, s := range cachedMsgs {
					client.WarmMIMEStructure("INBOX", s)
				}
			}
			m.MessagesPanel.SetMessages(cachedMsgs, 0)
			m.StatusPanel.TotalMessages = len(cachedMsgs)

			unread := 0
			for _, item := range cachedMsgs {
				if item.IsUnread {
					unread++
				}
			}
			m.StatusPanel.UnreadMessages = unread
			m.SendersPanel.SetSenders(gmail.AggregateSenders(cachedMsgs), len(cachedMsgs))

			// Load first cached message detail if available
			if detail := cacheStore.GetDetail("INBOX", cachedMsgs[0].ID); detail != nil {
				m.PreviewPanel.SetMessage(detail)
			}
		}
	}

	m.updatePanelFocus()
	return m
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadMessagesCmd(m.CurrentFolder, m.CurrentQuery),
		m.discoverFoldersCmd(),
		m.sampleSendersCmd(m.CurrentFolder),
	)
}

func (m *AppModel) updatePanelFocus() {
	m.PreviewPanel.Focused = (m.ActivePanel == panel.PanelPreview)
	m.StatusPanel.Focused = (m.ActivePanel == panel.PanelStatus)
	m.MessagesPanel.Focused = (m.ActivePanel == panel.PanelMessages)
	m.FoldersPanel.Focused = (m.ActivePanel == panel.PanelFolders)
	m.SendersPanel.Focused = (m.ActivePanel == panel.PanelSenders)
	m.StatusBar.ActivePanel = m.ActivePanel
	m.StatusBar.CurrentFolder = m.CurrentFolder
	if m.ActivePanel == panel.PanelSenders {
		if curr := m.SendersPanel.CurrentSender(); curr != nil {
			if curr.Email != "" {
				m.StatusBar.Notification = fmt.Sprintf("Sender: %s <%s> • %d message(s)", curr.DisplayName, curr.Email, curr.TotalCount)
			} else {
				m.StatusBar.Notification = fmt.Sprintf("All Senders • %d total message(s)", curr.TotalCount)
			}
		}
	}
	m.recalculateDimensions()
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.recalculateDimensions()
		if m.ReaderModal.Active {
			m.ReaderModal.SetDimensions(m.Width, m.Height)
		}
		return m, nil

	case OpenReaderMsg:
		if msg.Err != nil {
			m.StatusBar.ErrorMessage = fmt.Sprintf("Failed to open email: %v", msg.Err)
			return m, nil
		}
		if msg.Detail != nil {
			m.PreviewPanel.SetMessage(msg.Detail)
			if m.ReaderModal.Active {
				m.ReaderModal.SetDetail(msg.Detail)
			} else {
				m.ReaderModal.Open(msg.Detail, m.Width, m.Height)
			}
		}
		return m, nil

	case MessagesLoadedMsg:
		// Drop stale messages from outdated mailbox or query
		if msg.Mailbox != "" && (msg.Mailbox != m.CurrentFolder || msg.Query != m.CurrentQuery) {
			return m, nil
		}
		m.MessagesPanel.Loading = false
		m.MessagesPanel.LoadingMore = false
		if msg.Err != nil {
			m.StatusBar.ErrorMessage = fmt.Sprintf("Failed to load: %v", msg.Err)
			return m, nil
		}
		m.MessagesPanel.SetMessages(msg.Messages, msg.TotalCount)
		m.StatusPanel.TotalMessages = msg.TotalCount

		unread := 0
		for _, item := range msg.Messages {
			if item.IsUnread {
				unread++
			}
		}
		m.StatusPanel.UnreadMessages = unread

		// Update senders aggregation only on unfiltered views
		if msg.Query == "" {
			m.SendersPanel.SetFolderTotal(msg.TotalCount)
			localAll := m.getLocalMessagesForFolder(m.CurrentFolder)
			if len(localAll) > 0 {
				m.SendersPanel.MergeSenders(gmail.AggregateSenders(localAll), msg.TotalCount)
			}
		} else {
			m.StatusBar.Notification = fmt.Sprintf("Filtered (%d found)", msg.TotalCount)
			if strings.HasPrefix(msg.Query, "from:") {
				senderEmail := strings.Trim(strings.TrimPrefix(msg.Query, "from:"), "\" ")
				m.SendersPanel.UpdateSenderCount(senderEmail, msg.TotalCount)
			}
		}

		// Automatically load preview for the first message
		if len(msg.Messages) > 0 {
			firstID := msg.Messages[0].ID
			if m.Cache != nil {
				if cached := m.Cache.GetDetail(m.CurrentFolder, firstID); cached != nil {
					m.PreviewPanel.SetMessage(cached)
					return m, tea.Batch(cmds...)
				}
			}
			m.previewSeq++
			seq := m.previewSeq
			cmds = append(cmds, tea.Tick(150*time.Millisecond, func(_ time.Time) tea.Msg {
				return DebouncePreviewMsg{MessageID: firstID, Seq: seq}
			}))
		} else {
			m.PreviewPanel.SetMessage(nil)
		}
		return m, tea.Batch(cmds...)

	case messages.LoadMoreMessagesMsg:
		m.MessagesPanel.LoadingMore = true
		return m, m.loadMoreMessagesCmd(m.CurrentFolder, len(m.MessagesPanel.Items), 50, m.CurrentQuery)

	case MoreMessagesLoadedMsg:
		// Drop stale more-messages responses
		if msg.Mailbox != "" && (msg.Mailbox != m.CurrentFolder || msg.Query != m.CurrentQuery) {
			return m, nil
		}
		m.MessagesPanel.LoadingMore = false
		if msg.Err != nil {
			m.StatusBar.ErrorMessage = fmt.Sprintf("Failed to load more: %v", msg.Err)
			return m, nil
		}
		if len(msg.Messages) > 0 {
			m.MessagesPanel.AppendMessages(msg.Messages, msg.TotalCount)
			m.StatusPanel.TotalMessages = msg.TotalCount
			if msg.Query == "" {
				m.SendersPanel.MergeSenders(gmail.AggregateSenders(m.MessagesPanel.Items), msg.TotalCount)
			}
			if m.Cache != nil && m.CurrentQuery == "" {
				_ = m.Cache.SetMessages(m.CurrentFolder, m.MessagesPanel.Items)
			}
		} else {
			m.MessagesPanel.TotalCount = len(m.MessagesPanel.Items)
		}
		return m, nil

	case MessageDetailLoadedMsg:
		if msg.Err != nil {
			m.StatusBar.ErrorMessage = fmt.Sprintf("Failed to load message: %v", msg.Err)
			return m, nil
		}
		m.PreviewPanel.SetMessage(msg.Detail)
		return m, nil

	case ActionDoneMsg:
		m.StatusPanel.OperationState = ""
		m.MessagesPanel.ClearSelection()
		if msg.Err != nil {
			m.StatusBar.ErrorMessage = msg.Err.Error()
		} else {
			m.StatusBar.ErrorMessage = ""
			m.StatusBar.Notification = msg.Notification
			cmds = append(cmds, m.loadMessagesCmd(m.CurrentFolder, m.CurrentQuery))
		}
		return m, tea.Batch(cmds...)

	case messages.SelectMessageMsg:
		m.previewSeq++
		seq := m.previewSeq
		msgID := msg.MessageID

		// If already in cache, display immediately (0ms)
		if m.Cache != nil {
			if cached := m.Cache.GetDetail(m.CurrentFolder, msgID); cached != nil {
				m.PreviewPanel.SetMessage(cached)
				return m, nil
			}
		}

		// Otherwise debounce 80ms so rapid scrolling doesn't fire network requests
		return m, tea.Tick(80*time.Millisecond, func(_ time.Time) tea.Msg {
			return DebouncePreviewMsg{
				MessageID: msgID,
				Seq:       seq,
			}
		})

	case DebouncePreviewMsg:
		// Only fetch if cursor has settled on this message
		if msg.Seq == m.previewSeq {
			return m, m.loadMessageDetailCmd(msg.MessageID)
		}
		return m, nil

	case FoldersDiscoveredMsg:
		if msg.Err == nil && len(msg.Folders) > 0 {
			m.FoldersPanel.SetFolders(msg.Folders)
		}
		return m, nil

	case SendersSampledMsg:
		if msg.Folder != "" && msg.Folder != m.CurrentFolder {
			return m, nil
		}
		if msg.Err == nil && len(msg.Senders) > 0 {
			m.SendersPanel.MergeSenders(msg.Senders, msg.TotalMessages)
		}
		return m, nil

	case folders.SelectFolderMsg:
		m.CurrentFolder = msg.Item.ID
		m.StatusBar.CurrentFolder = m.CurrentFolder
		m.CurrentQuery = ""
		m.MessagesPanel.Cursor = 0
		m.SendersPanel.SelectEmail("")
		m.StatusBar.Notification = fmt.Sprintf("Viewing %s", msg.Item.Title)

		// Option C - Hybrid: Instant load from cache if available
		if m.Cache != nil {
			cachedMsgs := m.Cache.GetMessages(m.CurrentFolder)
			if len(cachedMsgs) > 0 {
				if m.Client != nil {
					for _, s := range cachedMsgs {
						m.Client.WarmMIMEStructure(m.CurrentFolder, s)
					}
				}
				m.MessagesPanel.SetMessages(cachedMsgs, 0)
				m.StatusPanel.TotalMessages = len(cachedMsgs)
				m.SendersPanel.SetSenders(gmail.AggregateSenders(cachedMsgs), len(cachedMsgs))
				if cached := m.Cache.GetDetail(m.CurrentFolder, cachedMsgs[0].ID); cached != nil {
					m.PreviewPanel.SetMessage(cached)
				}
			} else {
				m.MessagesPanel.SetMessages(nil, 0)
			}
		} else {
			m.MessagesPanel.SetMessages(nil, 0)
		}
		m.MessagesPanel.Loading = true
		cmds = append(cmds, m.loadMessagesCmd(m.CurrentFolder, m.CurrentQuery))
		cmds = append(cmds, m.sampleSendersCmd(m.CurrentFolder))
		return m, tea.Batch(cmds...)

	case senders.SelectSenderMsg:
		if msg.Sender != nil && msg.Sender.Email != "" {
			senderEmail := strings.ToLower(msg.Sender.Email)
			m.CurrentQuery = gmail.SenderQuery(senderEmail)
			m.MessagesPanel.Cursor = 0
			m.SendersPanel.SelectEmail(senderEmail)

			// Option C - Hybrid: 0ms instant local filter
			localAll := m.getLocalMessagesForFolder(m.CurrentFolder)
			instantMatches := gmail.FilterMessagesBySender(localAll, senderEmail)
			if len(instantMatches) > 0 {
				m.MessagesPanel.SetMessages(instantMatches, len(instantMatches))
				m.StatusPanel.TotalMessages = len(instantMatches)
				m.StatusBar.Notification = fmt.Sprintf("Filtered by %s (%d cached, syncing...)", msg.Sender.DisplayName, len(instantMatches))
				if m.Cache != nil {
					if cached := m.Cache.GetDetail(m.CurrentFolder, instantMatches[0].ID); cached != nil {
						m.PreviewPanel.SetMessage(cached)
					}
				}
			} else {
				m.MessagesPanel.SetMessages(nil, 0)
				m.StatusBar.Notification = fmt.Sprintf("Filtered by %s (syncing...)", msg.Sender.DisplayName)
			}
			m.MessagesPanel.Loading = true

			cmds = append(cmds, m.loadMessagesCmd(m.CurrentFolder, m.CurrentQuery))
		} else {
			// "All" selected: reset query filter to show all senders
			m.CurrentQuery = ""
			m.MessagesPanel.Cursor = 0
			m.SendersPanel.SelectEmail("")
			m.StatusBar.Notification = "Viewing all senders"

			if m.Cache != nil {
				cachedMsgs := m.Cache.GetMessages(m.CurrentFolder)
				if len(cachedMsgs) > 0 {
					m.MessagesPanel.SetMessages(cachedMsgs, len(cachedMsgs))
					m.StatusPanel.TotalMessages = len(cachedMsgs)
					if cached := m.Cache.GetDetail(m.CurrentFolder, cachedMsgs[0].ID); cached != nil {
						m.PreviewPanel.SetMessage(cached)
					}
				} else {
					m.MessagesPanel.SetMessages(nil, 0)
				}
			} else {
				m.MessagesPanel.SetMessages(nil, 0)
			}
			m.MessagesPanel.Loading = true

			cmds = append(cmds, m.loadMessagesCmd(m.CurrentFolder, ""))
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		// If ReaderModal is active, route all keys to it
		if m.ReaderModal.Active {
			var cmd tea.Cmd
			m.ReaderModal, cmd = m.ReaderModal.Update(msg)
			return m, cmd
		}

		// Handle Delete confirmation modal
		if m.ConfirmDelete {
			switch msg.String() {
			case "y", "Y", "enter":
				m.ConfirmDelete = false
				m.MessagesPanel.ClearSelection()
				cmds = append(cmds, m.executePurgeCmd(m.PendingPurgeIDs))
				m.PendingPurgeIDs = nil
				return m, tea.Batch(cmds...)
			case "n", "N", "esc", "q":
				m.ConfirmDelete = false
				m.PendingPurgeIDs = nil
				m.StatusBar.Notification = "Permanent delete cancelled."
				return m, nil
			}
			return m, nil
		}

		// Handle Help overlay toggle
		if msg.String() == "?" {
			m.ShowHelp = !m.ShowHelp
			return m, nil
		}
		if m.ShowHelp {
			if msg.String() == "esc" || msg.String() == "q" {
				m.ShowHelp = false
			}
			return m, nil
		}

		// Escape clears visual mode/selection in messages panel, or active query filter
		if msg.String() == "esc" {
			if m.ActivePanel == panel.PanelMessages && m.MessagesPanel.Escape() {
				return m, nil
			}
			if m.CurrentQuery != "" {
				m.CurrentQuery = ""
				m.MessagesPanel.Cursor = 0
				m.SendersPanel.SelectEmail("")
				m.StatusBar.Notification = "Filter cleared"
				if m.Cache != nil {
					if cached := m.Cache.GetMessages(m.CurrentFolder); len(cached) > 0 {
						m.MessagesPanel.SetMessages(cached, len(cached))
						m.StatusPanel.TotalMessages = len(cached)
						if cDet := m.Cache.GetDetail(m.CurrentFolder, cached[0].ID); cDet != nil {
							m.PreviewPanel.SetMessage(cDet)
						}
					}
				}
				m.MessagesPanel.Loading = true
				return m, m.loadMessagesCmd(m.CurrentFolder, "")
			}
			return m, nil
		}

		// Global navigation keys
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		// Reader Mode full-screen toggle (o or enter in messages panel)
		case "o", "O":
			curr := m.MessagesPanel.CurrentMessage()
			if curr == nil && m.PreviewPanel.Message != nil {
				m.ReaderModal.Open(m.PreviewPanel.Message, m.Width, m.Height)
				return m, nil
			}
			if curr != nil {
				if m.PreviewPanel.Message != nil && m.PreviewPanel.Message.ID == curr.ID {
					m.ReaderModal.Open(m.PreviewPanel.Message, m.Width, m.Height)
					return m, nil
				}
				if m.Cache != nil {
					if cached := m.Cache.GetDetail(m.CurrentFolder, curr.ID); cached != nil {
						m.ReaderModal.Open(cached, m.Width, m.Height)
						return m, nil
					}
				}
				m.ReaderModal.OpenLoading(curr, m.Width, m.Height)
				return m, m.openReaderModalCmd(curr.ID)
			}
			return m, nil

		case "enter":
			if m.ActivePanel == panel.PanelMessages {
				curr := m.MessagesPanel.CurrentMessage()
				if curr != nil {
					if m.PreviewPanel.Message != nil && m.PreviewPanel.Message.ID == curr.ID {
						m.ReaderModal.Open(m.PreviewPanel.Message, m.Width, m.Height)
						return m, nil
					}
					if m.Cache != nil {
						if cached := m.Cache.GetDetail(m.CurrentFolder, curr.ID); cached != nil {
							m.ReaderModal.Open(cached, m.Width, m.Height)
							return m, nil
						}
					}
					m.ReaderModal.OpenLoading(curr, m.Width, m.Height)
					return m, m.openReaderModalCmd(curr.ID)
				}
			}
		case "tab":
			m.ActivePanel = (m.ActivePanel + 1) % 5
			m.updatePanelFocus()
			return m, nil
		case "shift+tab":
			m.ActivePanel = (m.ActivePanel + 4) % 5
			m.updatePanelFocus()
			return m, nil
		case "0":
			m.ActivePanel = panel.PanelPreview
			m.updatePanelFocus()
			return m, nil
		case "1":
			m.ActivePanel = panel.PanelStatus
			m.updatePanelFocus()
			return m, nil
		case "2":
			m.ActivePanel = panel.PanelMessages
			m.updatePanelFocus()
			return m, nil
		case "3":
			m.ActivePanel = panel.PanelFolders
			m.updatePanelFocus()
			return m, nil
		case "4":
			m.ActivePanel = panel.PanelSenders
			m.updatePanelFocus()
			return m, nil

		// Decluttering actions
		case "r":
			m.MessagesPanel.Loading = true
			m.StatusBar.Notification = "Refreshing..."
			return m, m.loadMessagesCmd(m.CurrentFolder, m.CurrentQuery)

		case "f":
			// Instant filter by sender of currently highlighted email
			if curr := m.MessagesPanel.CurrentMessage(); curr != nil {
				_, email := gmail.ParseSender(curr.From)
				m.CurrentQuery = gmail.SenderQuery(email)
				m.MessagesPanel.Cursor = 0
				m.SendersPanel.SelectEmail(email)

				// Option C - Hybrid: 0ms instant local filter
				localAll := m.getLocalMessagesForFolder(m.CurrentFolder)
				instantMatches := gmail.FilterMessagesBySender(localAll, email)
				if len(instantMatches) > 0 {
					m.MessagesPanel.SetMessages(instantMatches, len(instantMatches))
					m.StatusPanel.TotalMessages = len(instantMatches)
					m.StatusBar.Notification = fmt.Sprintf("Filtered by %s (%d cached, syncing...)", email, len(instantMatches))
					if m.Cache != nil {
						if cached := m.Cache.GetDetail(m.CurrentFolder, instantMatches[0].ID); cached != nil {
							m.PreviewPanel.SetMessage(cached)
						}
					}
				} else {
					m.MessagesPanel.SetMessages(nil, 0)
					m.StatusBar.Notification = fmt.Sprintf("Filtered by %s (syncing...)", email)
				}
				m.MessagesPanel.Loading = true

				return m, m.loadMessagesCmd(m.CurrentFolder, m.CurrentQuery)
			}

		case "e", "a":
			if m.CurrentFolder == "TRASH" {
				m.StatusBar.ErrorMessage = "Cannot archive messages from Trash. Restore them first."
				return m, nil
			}
			ids := m.MessagesPanel.GetSelectedOrCurrentIDs()
			if len(ids) == 0 {
				m.StatusBar.ErrorMessage = "No message selected to archive."
				return m, nil
			}
			m.MessagesPanel.ClearSelection()
			m.StatusPanel.OperationState = fmt.Sprintf("Archiving %d message(s)...", len(ids))
			return m, m.archiveMessagesCmd(ids)

		case "R":
			if m.CurrentFolder != "TRASH" {
				m.StatusBar.ErrorMessage = "Restore is only available in the Trash mailbox."
				return m, nil
			}
			ids := m.MessagesPanel.GetSelectedOrCurrentIDs()
			if len(ids) == 0 {
				m.StatusBar.ErrorMessage = "No message selected to restore."
				return m, nil
			}
			m.MessagesPanel.ClearSelection()
			m.StatusPanel.OperationState = fmt.Sprintf("Restoring %d message(s)...", len(ids))
			return m, m.restoreMessagesCmd(ids)

		case "d":
			ids := m.MessagesPanel.GetSelectedOrCurrentIDs()
			if len(ids) == 0 {
				m.StatusBar.ErrorMessage = "No message selected."
				return m, nil
			}
			m.MessagesPanel.ClearSelection()
			if m.CurrentFolder == "TRASH" {
				// Restore from trash back to INBOX
				m.StatusPanel.OperationState = fmt.Sprintf("Restoring %d message(s)...", len(ids))
				return m, m.restoreMessagesCmd(ids)
			}
			// Move to trash
			m.StatusPanel.OperationState = fmt.Sprintf("Trashing %d message(s)...", len(ids))
			return m, m.trashMessagesCmd(ids)

		case "D": // Shift+D: Permanent Purge
			if m.CurrentFolder != "TRASH" {
				m.StatusBar.ErrorMessage = "Permanent delete is only permitted inside the Trash mailbox"
				return m, nil
			}
			ids := m.MessagesPanel.GetSelectedOrCurrentIDs()
			if len(ids) == 0 {
				m.StatusBar.ErrorMessage = "No message selected to delete."
				return m, nil
			}
			m.ConfirmDelete = true
			m.PendingPurgeIDs = ids
			return m, nil

		case "u":
			// Unsubscribe action
			if m.PreviewPanel.Message != nil && m.PreviewPanel.Message.HasUnsubscribe {
				return m, m.executeUnsubscribeCmd(m.PreviewPanel.Message)
			} else if curr := m.MessagesPanel.CurrentMessage(); curr != nil && curr.HasUnsubscribe {
				// If message in list has unsubscribe, fetch its detail and execute
				m.StatusPanel.OperationState = "Triggering unsubscribe..."
				return m, m.executeUnsubscribeByIDCmd(curr.ID)
			} else {
				m.StatusBar.Notification = "No unsubscribe link found in this message."
				return m, nil
			}
		}
	}

	// Route updates to individual sub-panels based on focus
	var cmd tea.Cmd
	switch m.ActivePanel {
	case panel.PanelPreview:
		m.PreviewPanel, cmd = m.PreviewPanel.Update(msg)
		cmds = append(cmds, cmd)
	case panel.PanelStatus:
		m.StatusPanel, cmd = m.StatusPanel.Update(msg)
		cmds = append(cmds, cmd)
	case panel.PanelMessages:
		m.MessagesPanel, cmd = m.MessagesPanel.Update(msg)
		cmds = append(cmds, cmd)
	case panel.PanelFolders:
		m.FoldersPanel, cmd = m.FoldersPanel.Update(msg)
		cmds = append(cmds, cmd)
	case panel.PanelSenders:
		m.SendersPanel, cmd = m.SendersPanel.Update(msg)
		cmds = append(cmds, cmd)
		if curr := m.SendersPanel.CurrentSender(); curr != nil {
			if curr.Email != "" {
				m.StatusBar.Notification = fmt.Sprintf("Sender: %s <%s> • %d message(s)", curr.DisplayName, curr.Email, curr.TotalCount)
			} else {
				m.StatusBar.Notification = fmt.Sprintf("All Senders • %d total message(s)", curr.TotalCount)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

const (
	panelBorderRows    = 2
	minBottomHeight    = 5 + panelBorderRows // 7 (5 content rows + 2 borders)
	messagesMinRows    = 8
	messagesMinPercent = 40
)

// messagesMinHeight computes the smallest outer height Messages retains when another panel is focused.
func messagesMinHeight(total int) int {
	return max(messagesMinRows+panelBorderRows, total*messagesMinPercent/100)
}

// leftColumnHeights splits the left column's total rows between Status, Messages,
// and the bottom Folders/Senders row (matching lazytask's dynamic layout).
// When Folders or Senders is focused, the bottom row expands and Messages shrinks to its floor.
// Otherwise, the bottom row collapses to minBottomHeight (7 rows).
func leftColumnHeights(total int, activePanel int) (statusHeight, messagesHeight, bottomHeight int) {
	statusHeight = 3
	rest := max(total-statusHeight, 0)
	msgMin := messagesMinHeight(total)

	switch activePanel {
	case panel.PanelFolders, panel.PanelSenders:
		messagesHeight = msgMin
	default:
		messagesHeight = rest - minBottomHeight
	}
	messagesHeight = max(min(messagesHeight, rest-minBottomHeight), msgMin)
	messagesHeight = min(messagesHeight, rest)
	bottomHeight = rest - messagesHeight
	return statusHeight, messagesHeight, bottomHeight
}

func (m *AppModel) recalculateDimensions() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}

	footerHeight := 1
	bodyHeight := m.Height - footerHeight
	if bodyHeight < 10 {
		bodyHeight = 10
	}

	// 50% left column, 50% right column
	leftWidth := m.Width / 2
	rightWidth := m.Width - leftWidth

	statusHeight, messagesHeight, bottomHeight := leftColumnHeights(bodyHeight, m.ActivePanel)

	var foldersWidth, sendersWidth int
	if m.ActivePanel == panel.PanelFolders {
		foldersWidth = leftWidth / 2
		sendersWidth = leftWidth - foldersWidth
	} else {
		foldersWidth = leftWidth / 3
		if foldersWidth < 16 {
			foldersWidth = 16
		}
		if foldersWidth > 22 {
			foldersWidth = 22
		}
		if foldersWidth > leftWidth/2 {
			foldersWidth = leftWidth / 2
		}
		sendersWidth = leftWidth - foldersWidth
	}

	m.StatusPanel.Width = leftWidth
	m.StatusPanel.Height = statusHeight

	m.MessagesPanel.Width = leftWidth
	m.MessagesPanel.Height = messagesHeight

	m.FoldersPanel.Width = foldersWidth
	m.FoldersPanel.Height = bottomHeight

	m.SendersPanel.Width = sendersWidth
	m.SendersPanel.Height = bottomHeight

	m.PreviewPanel.SetDimensions(rightWidth, bodyHeight)
	m.StatusBar.Width = m.Width
	if m.ReaderModal.Active {
		m.ReaderModal.SetDimensions(m.Width, m.Height)
	}
}

func (m AppModel) View() string {
	if m.Width == 0 || m.Height == 0 {
		return "Initializing LazyMail..."
	}

	// Left Column: Status, Messages, Folders & Senders
	statusView := m.StatusPanel.View()
	messagesView := m.MessagesPanel.View()
	bottomLeftRow := lipgloss.JoinHorizontal(lipgloss.Top, m.FoldersPanel.View(), m.SendersPanel.View())
	leftColumn := lipgloss.JoinVertical(lipgloss.Left, statusView, messagesView, bottomLeftRow)

	// Right Column: Email Body Preview
	rightColumn := m.PreviewPanel.View()

	// 5-Panel Grid
	mainGrid := lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, rightColumn)

	// Footer status bar
	footer := m.StatusBar.View()

	content := lipgloss.JoinVertical(lipgloss.Left, mainGrid, footer)

	// Guarantee total content lines never exceed m.Height to prevent terminal scrolling
	if lines := strings.Split(content, "\n"); len(lines) > m.Height && m.Height > 0 {
		content = strings.Join(lines[:m.Height], "\n")
	}

	// Render Confirmation Modal if active
	if m.ConfirmDelete {
		return m.renderDeleteConfirmModal(content)
	}

	// Render Help Popup if active
	if m.ShowHelp {
		return helppopup.Render(m.Width, m.Height)
	}

	// Render Full-Screen Reader Modal if active
	if m.ReaderModal.Active {
		return m.ReaderModal.View()
	}

	return content
}

func (m AppModel) renderDeleteConfirmModal(background string) string {
	boxWidth := 50
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(panel.WarningColor)
	descStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))

	modalContent := fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		titleStyle.Render("⚠️  PERMANENT DELETE WARNING"),
		descStyle.Render(fmt.Sprintf("Permanently purge %d message(s)?\nThis action CANNOT be undone.", len(m.PendingPurgeIDs))),
		"Press [y/Enter] to Confirm | [n/Esc] to Cancel",
	)

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.DoubleBorder()).
		BorderForeground(panel.WarningColor).
		Padding(1, 2).
		Width(boxWidth).
		Render(modalContent)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, box)
}

func (m AppModel) getLocalMessagesForFolder(mailbox string) []*gmail.MessageSummary {
	seen := make(map[string]bool)
	var all []*gmail.MessageSummary

	if m.Cache != nil {
		for _, msg := range m.Cache.GetMessages(mailbox) {
			if !seen[msg.ID] {
				seen[msg.ID] = true
				all = append(all, msg)
			}
		}
	}
	if m.CurrentFolder == mailbox {
		for _, msg := range m.MessagesPanel.Items {
			if !seen[msg.ID] {
				seen[msg.ID] = true
				all = append(all, msg)
			}
		}
	}
	return all
}

// Commands for async Gmail operations
func (m AppModel) loadMessagesCmd(mailbox, query string) tea.Cmd {
	return func() tea.Msg {
		if m.Client == nil {
			return MessagesLoadedMsg{Mailbox: mailbox, Query: query, Messages: []*gmail.MessageSummary{}}
		}
		items, total, err := m.Client.ListMessages(context.Background(), mailbox, 0, 50, query)
		if err == nil && m.Cache != nil && query == "" {
			_ = m.Cache.SetMessages(mailbox, items)
		}
		return MessagesLoadedMsg{Mailbox: mailbox, Query: query, Messages: items, TotalCount: total, Err: err}
	}
}

func (m AppModel) loadMoreMessagesCmd(mailbox string, offset, limit int, query string) tea.Cmd {
	return func() tea.Msg {
		if m.Client == nil {
			return MoreMessagesLoadedMsg{Mailbox: mailbox, Query: query}
		}
		items, total, err := m.Client.ListMessages(context.Background(), mailbox, offset, limit, query)
		return MoreMessagesLoadedMsg{Mailbox: mailbox, Query: query, Messages: items, TotalCount: total, Err: err}
	}
}

func (m AppModel) discoverFoldersCmd() tea.Cmd {
	return func() tea.Msg {
		if m.Client == nil {
			return FoldersDiscoveredMsg{}
		}
		discovered, err := m.Client.DiscoverCleanMailboxes()
		if err != nil {
			return FoldersDiscoveredMsg{Err: err}
		}
		var items []folders.FolderItem
		for _, d := range discovered {
			items = append(items, folders.FolderItem{
				ID:    d.ID,
				Title: d.Title,
				Query: d.Query,
			})
		}
		return FoldersDiscoveredMsg{Folders: items}
	}
}

func (m AppModel) sampleSendersCmd(folder string) tea.Cmd {
	return func() tea.Msg {
		if m.Client == nil {
			return SendersSampledMsg{Folder: folder}
		}
		sampled, total, err := m.Client.SampleFolderSenders(context.Background(), folder, 100)
		return SendersSampledMsg{Folder: folder, Senders: sampled, TotalMessages: total, Err: err}
	}
}

func (m AppModel) loadMessageDetailCmd(id string) tea.Cmd {
	return func() tea.Msg {
		// Fast return from encrypted cache if available
		if m.Cache != nil {
			if cached := m.Cache.GetDetail(m.CurrentFolder, id); cached != nil {
				return MessageDetailLoadedMsg{Detail: cached, Err: nil}
			}
		}

		if m.Client == nil {
			return nil
		}
		detail, err := m.Client.GetMessageDetail(context.Background(), m.CurrentFolder, id)
		if err == nil && m.Cache != nil {
			_ = m.Cache.SetDetail(m.CurrentFolder, id, detail)
		}
		return MessageDetailLoadedMsg{Detail: detail, Err: err}
	}
}

func (m AppModel) archiveMessagesCmd(ids []string) tea.Cmd {
	return func() tea.Msg {
		if m.Client == nil {
			return ActionDoneMsg{Err: fmt.Errorf("client not connected")}
		}
		err := m.Client.BatchArchive(context.Background(), m.CurrentFolder, ids)
		if err != nil {
			return ActionDoneMsg{Err: fmt.Errorf("failed to archive messages: %w", err)}
		}
		return ActionDoneMsg{Notification: fmt.Sprintf("Archived %d message(s)", len(ids))}
	}
}

func (m AppModel) trashMessagesCmd(ids []string) tea.Cmd {
	return func() tea.Msg {
		if m.Client == nil {
			return ActionDoneMsg{Err: fmt.Errorf("client not connected")}
		}
		err := m.Client.BatchTrash(context.Background(), m.CurrentFolder, ids)
		if err != nil {
			return ActionDoneMsg{Err: fmt.Errorf("failed to trash messages: %w", err)}
		}
		return ActionDoneMsg{Notification: fmt.Sprintf("Moved %d message(s) to Trash", len(ids))}
	}
}

func (m AppModel) restoreMessagesCmd(ids []string) tea.Cmd {
	return func() tea.Msg {
		if m.Client == nil {
			return ActionDoneMsg{Err: fmt.Errorf("client not connected")}
		}
		err := m.Client.BatchRestore(context.Background(), ids)
		if err != nil {
			return ActionDoneMsg{Err: fmt.Errorf("failed to restore messages: %w", err)}
		}
		return ActionDoneMsg{Notification: fmt.Sprintf("Restored %d message(s) to Inbox", len(ids))}
	}
}

func (m AppModel) executePurgeCmd(ids []string) tea.Cmd {
	return func() tea.Msg {
		if m.Client == nil {
			return ActionDoneMsg{Err: fmt.Errorf("client not connected")}
		}
		err := m.Client.BatchDelete(context.Background(), ids)
		if err != nil {
			return ActionDoneMsg{Err: fmt.Errorf("failed to permanently purge messages: %w", err)}
		}
		return ActionDoneMsg{Notification: fmt.Sprintf("Permanently deleted %d message(s)", len(ids))}
	}
}

func (m AppModel) executeUnsubscribeCmd(detail *gmail.MessageDetail) tea.Cmd {
	return func() tea.Msg {
		if detail == nil || detail.Unsubscribe == nil {
			return ActionDoneMsg{Err: fmt.Errorf("no unsubscribe method available")}
		}

		unsub := detail.Unsubscribe
		if unsub.IsOneClick && unsub.HTTPURL != "" {
			err := gmail.ExecuteOneClick(context.Background(), nil, unsub.HTTPURL)
			if err != nil {
				return ActionDoneMsg{Err: fmt.Errorf("one-click unsubscribe failed: %w", err)}
			}
			return ActionDoneMsg{Notification: "Successfully sent One-Click unsubscribe POST."}
		}

		if unsub.HTTPURL != "" {
			_ = auth.OpenBrowser(unsub.HTTPURL)
			return ActionDoneMsg{Notification: fmt.Sprintf("Opened unsubscribe URL in browser: %s", unsub.HTTPURL)}
		}

		if unsub.Mailto != "" {
			err := gmail.SendUnsubscribeEmail(context.Background(), m.Client, unsub.Mailto)
			if err != nil {
				return ActionDoneMsg{Err: fmt.Errorf("failed to send unsubscribe email: %w", err)}
			}
			return ActionDoneMsg{Notification: "Sent unsubscribe email to mailing list."}
		}

		return ActionDoneMsg{Err: fmt.Errorf("no valid unsubscribe method")}
	}
}

func (m AppModel) executeUnsubscribeByIDCmd(id string) tea.Cmd {
	return func() tea.Msg {
		detail, err := m.Client.GetMessageDetail(context.Background(), m.CurrentFolder, id)
		if err != nil {
			return ActionDoneMsg{Err: fmt.Errorf("fetch message for unsubscribe: %w", err)}
		}
		return m.executeUnsubscribeCmd(detail)()
	}
}

func (m AppModel) openReaderModalCmd(id string) tea.Cmd {
	return func() tea.Msg {
		if m.Cache != nil {
			if cached := m.Cache.GetDetail(m.CurrentFolder, id); cached != nil {
				return OpenReaderMsg{Detail: cached}
			}
		}
		if m.Client == nil {
			return OpenReaderMsg{}
		}
		detail, err := m.Client.GetMessageDetail(context.Background(), m.CurrentFolder, id)
		if err == nil && m.Cache != nil {
			_ = m.Cache.SetDetail(m.CurrentFolder, id, detail)
		}
		return OpenReaderMsg{Detail: detail, Err: err}
	}
}

