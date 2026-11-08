package gmail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseUnsubscribe_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		headers      map[string]string
		bodyHTML     string
		bodyPlain    string
		wantURL      string
		wantMailto   string
		wantOneClick bool
		wantInBody   bool
	}{
		{
			name: "rfc 2369 header with mailto and url",
			headers: map[string]string{
				"List-Unsubscribe": "<mailto:unsub@newsletter.com?subject=unsub>, <https://newsletter.com/optout?id=456>",
			},
			wantURL:      "https://newsletter.com/optout?id=456",
			wantMailto:   "mailto:unsub@newsletter.com?subject=unsub",
			wantOneClick: false,
			wantInBody:   false,
		},
		{
			name: "rfc 8058 one-click unsubscribe",
			headers: map[string]string{
				"List-Unsubscribe":      "<https://newsletter.com/api/unsubscribe/123>",
				"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
			},
			wantURL:      "https://newsletter.com/api/unsubscribe/123",
			wantMailto:   "",
			wantOneClick: true,
			wantInBody:   false,
		},
		{
			name:    "body html fallback with href",
			headers: map[string]string{},
			bodyHTML: `<div>Thank you for subscribing! <a href="https://service.example.com/unsubscribe?user=99">Click here to unsubscribe</a></div>`,
			wantURL:      "https://service.example.com/unsubscribe?user=99",
			wantMailto:   "",
			wantOneClick: false,
			wantInBody:   true,
		},
		{
			name:      "body plain text fallback",
			headers:   map[string]string{},
			bodyPlain: "To opt out of emails, go to: https://service.example.com/optout/direct",
			wantURL:      "https://service.example.com/optout/direct",
			wantMailto:   "",
			wantOneClick: false,
			wantInBody:   true,
		},
		{
			name:         "no unsubscribe available",
			headers:      map[string]string{"Subject": "Meeting notes"},
			bodyPlain:    "Here are the meeting notes from today.",
			wantURL:      "",
			wantMailto:   "",
			wantOneClick: false,
			wantInBody:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ParseUnsubscribe(tt.headers, tt.bodyHTML, tt.bodyPlain)
			if info.HTTPURL != tt.wantURL {
				t.Errorf("HTTPURL got %q, want %q", info.HTTPURL, tt.wantURL)
			}
			if info.Mailto != tt.wantMailto {
				t.Errorf("Mailto got %q, want %q", info.Mailto, tt.wantMailto)
			}
			if info.IsOneClick != tt.wantOneClick {
				t.Errorf("IsOneClick got %v, want %v", info.IsOneClick, tt.wantOneClick)
			}
			if info.FoundInBody != tt.wantInBody {
				t.Errorf("FoundInBody got %v, want %v", info.FoundInBody, tt.wantInBody)
			}
		})
	}
}

func TestExecuteOneClick(t *testing.T) {
	receivedBody := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 100)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := ExecuteOneClick(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedBody != "List-Unsubscribe=One-Click" {
		t.Errorf("expected body 'List-Unsubscribe=One-Click', got %q", receivedBody)
	}
}
