package onboarding

import (
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

func TestOnboardingModel_ValidationAndSubmit(t *testing.T) {
	m := New()

	// Initially, pressing Enter on the submit button with empty inputs triggers an error
	m.FocusIndex = btnSubmit
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := newModel.(Model)

	if updated.ErrorMessage == "" {
		t.Errorf("expected error message when submitting empty fields, got none")
	}
	if updated.Submitted {
		t.Errorf("expected form not to be submitted")
	}

	// Fill valid fields
	updated.Inputs[inputEmail].SetValue("test@gmail.com")
	updated.Inputs[inputAppPassword].SetValue("abcd efgh ijkl mnop")
	updated.Inputs[inputPassphrase].SetValue("pass123")
	updated.Inputs[inputConfirmPassphrase].SetValue("pass123")
	updated.FocusIndex = btnSubmit

	newModel, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	final := newModel.(Model)

	if !final.Submitted {
		t.Errorf("expected form to be submitted with valid inputs")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command on submit")
	}
}
