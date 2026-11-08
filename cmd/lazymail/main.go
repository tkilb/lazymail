package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tkilb/lazymail/internal/auth"
	"github.com/tkilb/lazymail/internal/cache"
	"github.com/tkilb/lazymail/internal/gmail"
	"github.com/tkilb/lazymail/internal/ui"
	"github.com/tkilb/lazymail/internal/ui/onboarding"
	"golang.org/x/term"
)

func main() {
	resetFlag := flag.Bool("reset", false, "Reset stored encrypted vault and re-run onboarding")
	flag.Parse()

	if *resetFlag {
		path, _ := auth.VaultPath()
		_ = os.Remove(path)
		fmt.Println("Cleared encrypted vault. Starting setup...")
	}

	vaultExists, err := auth.VaultExists()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error accessing config: %v\n", err)
		os.Exit(1)
	}

	var vault *auth.Vault
	var passphrase string
	var client *gmail.Client

	if !vaultExists {
		// Launch interactive TUI onboarding form
		onboardingModel := onboarding.New()
		prog := tea.NewProgram(onboardingModel, tea.WithAltScreen())
		finalModel, err := prog.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running onboarding setup: %v\n", err)
			os.Exit(1)
		}

		m, ok := finalModel.(onboarding.Model)
		if !ok || !m.Submitted {
			fmt.Println("Setup cancelled.")
			os.Exit(0)
		}

		email := strings.TrimSpace(m.Inputs[0].Value())
		appPassword := strings.TrimSpace(m.Inputs[1].Value())
		passphrase = strings.TrimSpace(m.Inputs[2].Value())

		fmt.Printf("\nConnecting to Gmail IMAP (%s)...\n", email)
		client, err = gmail.NewClient(email, appPassword)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nConnection failed: %v\n", err)
			fmt.Fprintln(os.Stderr, "Please verify your Gmail address and 16-character App Password, then run again.")
			os.Exit(1)
		}

		vault = &auth.Vault{
			Email:       email,
			AppPassword: appPassword,
		}

		// Save encrypted vault. No plaintext credentials touch the filesystem.
		if err := auth.SaveVault(vault, passphrase); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving encrypted vault: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Credentials encrypted and saved securely! Starting LazyMail...")
	} else {
		// Vault exists: prompt for master passphrase to unlock
		for {
			passphrase, err = promptPassword("Enter master password to unlock LazyMail: ")
			if err != nil || passphrase == "" {
				fmt.Fprintf(os.Stderr, "\nPassword cannot be empty.\n")
				os.Exit(1)
			}

			vault, err = auth.LoadVault(passphrase)
			if err == nil {
				break
			}

			if errors.Is(err, auth.ErrAuthenticationFailed) {
				fmt.Println("Incorrect password. Please try again.")
				continue
			}

			fmt.Fprintf(os.Stderr, "Error unlocking vault: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Connecting to Gmail (%s)...\n", vault.Email)
		client, err = gmail.NewClient(vault.Email, vault.AppPassword)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to Gmail: %v\n", err)
			fmt.Println("Tip: run './bin/lazymail --reset' if you need to update your password.")
			os.Exit(1)
		}
	}

	defer client.Close()

	// Initialize encrypted-at-rest cache
	cacheStore, _ := cache.New(passphrase)

	// Launch main 5-panel LazyMail Bubble Tea TUI
	app := ui.NewAppModel(client, cacheStore, passphrase)
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running LazyMail: %v\n", err)
		os.Exit(1)
	}
}

func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytePassword)), nil
}
