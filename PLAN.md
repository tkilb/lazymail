---
uid: 7198a9b4-c3a1-47a7-8219-bac9e1d116f8
title: PLAN
kind: ""
source: ""
date: ""
tags: []
people: []
related: []
---

# LazyMail: Gmail TUI Decluttering & Client Specification

An efficient, keyboard-driven Terminal User Interface (TUI) email client optimized for inbox zero, bulk decluttering, subscription unenrollment, and safe email management.

---

## 1. System Architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│                             Bubble Tea TUI                             │
│  ┌───────────────────────┐ ┌────────────────────────────────────────┐  │
│  │ [1] Status            │ │ [0] Email Body (Preview)               │  │
│  ├───────────────────────┤ │                                        │  │
│  │ [2] Messages          │ │                                        │  │
│  ├───────────┬───────────┤ │                                        │  │
│  │[3] Folders│[4] Senders│ │                                        │  │
│  └───────────┴───────────┘ └────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                     Status Bar & Key Hints                       │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ tea.Cmd / Messages
┌───────────────────────────────────▼────────────────────────────────────┐
│                         Application Controller                         │
│  • Panel Focus (0-4 / Tab)       • Background Sync & Task Manager      │
│  • Batch Selection State         • Gmail Query & Unsubscribe Pipeline  │
└─────────────────┬──────────────────────────────────┬───────────────────┘
                  │                                  │
┌─────────────────▼───────────┐    ┌─────────────────▼──────────────────┐
│      GPG Secret Store       │    │          Gmail API Client          │
│ • GPG Symmetric AES-256     │    │ • OAuth2 Loopback Flow             │
│ • ~/.config/lazymail/       │    │ • Batch Trash / Batch Delete       │
│   token.json.gpg            │    │ • List-Unsubscribe Engine          │
│ • Memory-only Passphrase    │    │ • Server-side Query Engine         │
└─────────────────────────────┘    └────────────────────────────────────┘
```

---

## 2. Layout & UI Components (`lazytask` 5-Panel Grid)

### Visual Layout

```
┌─ [1] Status ──────────────┐┌─ [0] Email Body (Preview) ──────────────────────┐
│ (Top Left - Compact)      ││ (Right Column - Full Height)                   │
├─ [2] Messages ────────────┤│                                                │
│ (Middle Left - Email List)││                                                │
│                           ││                                                │
├─────────────┬─────────────┤│                                                │
│ [3] Folders │ [4] Senders ││                                                │
│ (Bottom L1) │ (Bottom L2) ││                                                │
└─────────────┴─────────────┘└────────────────────────────────────────────────┘
 [0-4 / tab / shift+tab] Panels   [?] Help   [q] Quit
```

### Panel Responsibilities

1. **Panel 1: Status (`1`)**
   - Active account, sync status, network/auth state, and decluttering operation progress.
2. **Panel 2: Messages (`2`)**
   - Email list for the currently selected folder/sender/filter with multi-selection (`[x]`).
3. **Panel 3: Folders (`3`)**
   - Mailboxes and smart views (`Inbox`, `Subscriptions`, `Sent`, `Trash`, custom labels).
4. **Panel 4: Senders (`4`)**
   - List of top / high-volume senders for targeted bulk triage.
5. **Panel 0: Email Body (`0`)**
   - Full message preview, header inspection, and one-click unsubscribe action.
6. **Bottom Bar / Footer**
   - Global and local keybindings, status notifications, and error alerts.
   - Active operation progress (e.g. `Trashing 42 messages...`).
   - Error notification banner (rendered with distinct styling).
   - Context-sensitive keybinding hints.

---

## 3. Security & GPG Symmetric Credential Store

### Storage Design

- Config directory: `~/.config/lazymail/`
- Encrypted file: `~/.config/lazymail/token.json.gpg`
- OAuth2 Client configuration: `~/.config/lazymail/credentials.json` (or interactive CLI setup).

### Encryption / Decryption Pipeline

- **Encryption**:
  ```bash
  gpg --batch --yes --symmetric --cipher-algo AES256 --passphrase-fd 0 -o ~/.config/lazymail/token.json.gpg
  ```
- **Decryption**:
  ```bash
  gpg --batch --decrypt --passphrase-fd 0 ~/.config/lazymail/token.json.gpg
  ```
- **Security Rules**:
  - Passphrase is kept **in-memory only** inside the Go runtime for the lifetime of the session.
  - No plaintext tokens ever touch the filesystem.
  - Tokens are refreshed in-memory using Google OAuth2 client library; updated refresh tokens are immediately re-encrypted to disk.

---

## 4. Decluttering Engine & Action Workflows

### 1. Subscription & Unsubscribe Engine

- Reads RFC 2369 / RFC 8058 headers:
  - `List-Unsubscribe: <mailto:...>, <https://...>`
  - `List-Unsubscribe-Post: List-Unsubscribe=One-Click`
- Fallback: Scans HTML/Plain body text for regex matches on `href=".*unsubscribe.*"`.
- **Action**:
  - If One-Click POST is supported, performs an authenticated HTTP POST directly.
  - If URL is provided, opens the browser via system handler (`xdg-open`).
  - If `mailto:` is provided, sends the required unsubscribe email via Gmail API.

### 2. Guarded Deletion Workflow

- **From `Inbox` / any standard mailbox**:
  - `d`: Batch moves selected (or currently focused) messages to `[TRASH]` using `users.messages.batchModify(ids, addLabels=["TRASH"], removeLabels=["INBOX"])`.
  - `Shift+D`: **Disabled** (displays message: _"Permanent delete is only permitted inside the Trash mailbox"_).
- **Inside `Trash` mailbox**:
  - `d`: Restore message back to `INBOX`.
  - `Shift+D`: Triggers modal confirmation: _"Permanently delete N messages? This action cannot be undone."_ Upon typing `yes` or pressing `Enter`, calls `users.messages.batchDelete(ids)`.

### 3. Filter by Sender Shortcut

- Pressing `f` on any message immediately filters Panel 2 by `from:<sender_email>`, allowing instant bulk review and trashing of all emails from that sender.

---

## 5. Proposed Go Project Structure

```
lazymail/
├── cmd/
│   └── lazymail/
│       └── main.go              # Entrypoint, grid layout coordination, focus router
├── internal/
│   ├── auth/
│   │   ├── oauth.go             # Google OAuth2 loopback webserver
│   │   ├── gpg.go               # GPG symmetric AES-256 encryption/decryption
│   │   └── gpg_test.go          # Tests for encryption/decryption roundtrip
│   ├── gmail/
│   │   ├── client.go            # Gmail service wrapper
│   │   ├── messages.go          # List, batch trash, batch delete, get message details
│   │   ├── unsubscribe.go       # RFC 2369/8058 parsing and one-click action handler
│   │   ├── senders.go           # Senders aggregation & volume analysis
│   │   └── query.go             # Gmail query builder (sender, unsubscribe, labels)
│   └── ui/
│       ├── panel/
│       │   └── panel.go         # Border styling, focus highlight, title helpers
│       ├── status/
│       │   └── model.go         # Panel 1: Status, account state & operation progress
│       ├── messages/
│       │   └── model.go         # Panel 2: Message table with multi-select checkboxes
│       ├── folders/
│       │   └── model.go         # Panel 3: Folder tree & smart declutter views
│       ├── senders/
│       │   └── model.go         # Panel 4: Top senders list for rapid sender triage
│       ├── preview/
│       │   └── model.go         # Panel 0: Email body viewport & header inspector
│       ├── statusbar/
│       │   └── model.go         # Bottom bar key hints & error banner
│       └── helppopup/
│           └── helppopup.go     # '?' modal popup with context-sensitive keybindings
├── Makefile
├── go.mod
└── go.sum
```

---

## 6. Phased Implementation Roadmap

1. **Phase 1: Security & Auth Foundation**
   - Implement `internal/auth/gpg.go` for symmetric AES-256 encryption/decryption with table-driven tests.
   - Implement `internal/auth/oauth.go` with loopback redirect listener.
   - Onboarding flow: master passphrase prompt on boot to decrypt `token.json.gpg` in memory.

2. **Phase 2: Gmail Core Services & Senders Analytics**
   - Setup Gmail API client with batch operations (`batchModify` for trash, `batchDelete` for purge).
   - Implement `unsubscribe.go` to parse headers (`List-Unsubscribe`, `List-Unsubscribe-Post`).
   - Implement `senders.go` for sender frequency and volume aggregation.

3. **Phase 3: 5-Panel Grid UI & Navigation**
   - Implement the `lazytask` 5-panel layout using Bubble Tea + Lipgloss:
     - Left: `[1] Status`, `[2] Messages`, `[3] Folders` / `[4] Senders`
     - Right: `[0] Email Body`
   - Implement direct jumps (`0-4`), cycling (`Tab` / `Shift+Tab`), and responsive resizing.

4. **Phase 4: Decluttering Actions & Guarded Deletion**
   - Multi-selection: `Space` to toggle, `A` to select all in current view.
   - Guarded deletion:
     - `d` moves selected to `[TRASH]`.
     - `Shift+D` is disabled in normal folders; enabled **only inside Trash** with explicit confirmation modal.

5. **Phase 5: Sender Triage & One-Click Unsubscribe**
   - `f`: Instant filter messages by sender of currently selected email.
   - `u`: Trigger one-click RFC 8058 POST or open unsubscribe URL in browser.

6. **Phase 6: Polish, Help Overlay & Table-Driven Tests**
   - Neovim navigation (`h/j/k/l`), viewport paging (`d`/`u`).
   - `?` help popup displaying contextual local/global keybindings.
   - Full test suite across auth, unsubscribe header parsing, query generation, and panel sizing.
