package gmail

import (
	"testing"

	"github.com/emersion/go-imap"
)

func TestClient_ResolveMailbox(t *testing.T) {
	c := &Client{
		trashBox: "[Gmail]/Trash",
		sentBox:  "[Gmail]/Sent Mail",
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"INBOX", "INBOX"},
		{"inbox", "INBOX"},
		{"SUBSCRIPTIONS", "INBOX"},
		{"TRASH", "[Gmail]/Trash"},
		{"trash", "[Gmail]/Trash"},
		{"SENT", "[Gmail]/Sent Mail"},
		{"CustomFolder", "CustomFolder"},
	}

	for _, tt := range tests {
		got := c.ResolveMailbox(tt.input)
		if got != tt.expected {
			t.Errorf("ResolveMailbox(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFindMIMEBodyParts(t *testing.T) {
	tests := []struct {
		name          string
		structure     *imap.BodyStructure
		wantHTMLPath  []int
		wantPlainPath []int
	}{
		{
			name: "single-part plain text",
			structure: &imap.BodyStructure{
				MIMEType:    "text",
				MIMESubType: "plain",
			},
			wantHTMLPath:  nil,
			wantPlainPath: []int{},
		},
		{
			name: "multipart/alternative with plain (1) and html (2)",
			structure: &imap.BodyStructure{
				MIMEType:    "multipart",
				MIMESubType: "alternative",
				Parts: []*imap.BodyStructure{
					{MIMEType: "text", MIMESubType: "plain"},
					{MIMEType: "text", MIMESubType: "html"},
				},
			},
			wantHTMLPath:  []int{2},
			wantPlainPath: []int{1},
		},
		{
			name: "multipart/mixed with sub-alternative (1.1 plain, 1.2 html) and attachment (2 pdf)",
			structure: &imap.BodyStructure{
				MIMEType:    "multipart",
				MIMESubType: "mixed",
				Parts: []*imap.BodyStructure{
					{
						MIMEType:    "multipart",
						MIMESubType: "alternative",
						Parts: []*imap.BodyStructure{
							{MIMEType: "text", MIMESubType: "plain"},
							{MIMEType: "text", MIMESubType: "html"},
						},
					},
					{
						MIMEType:    "application",
						MIMESubType: "pdf",
					},
				},
			},
			wantHTMLPath:  []int{1, 2},
			wantPlainPath: []int{1, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHTML, gotPlain := findMIMEBodyParts(tt.structure, nil)
			if !slicesEqual(gotHTML, tt.wantHTMLPath) {
				t.Errorf("gotHTML = %v, want %v", gotHTML, tt.wantHTMLPath)
			}
			if !slicesEqual(gotPlain, tt.wantPlainPath) {
				t.Errorf("gotPlain = %v, want %v", gotPlain, tt.wantPlainPath)
			}
		})
	}
}

func slicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseMIMEStructure_Attachments(t *testing.T) {
	// Mixed message with alternative body (plain 1.1, html 1.2) and a 15MB PDF attachment (2)
	structure := &imap.BodyStructure{
		MIMEType:    "multipart",
		MIMESubType: "mixed",
		Parts: []*imap.BodyStructure{
			{
				MIMEType:    "multipart",
				MIMESubType: "alternative",
				Parts: []*imap.BodyStructure{
					{
						MIMEType:    "text",
						MIMESubType: "plain",
						Encoding:    "quoted-printable",
						Params:      map[string]string{"charset": "utf-8"},
					},
					{
						MIMEType:    "text",
						MIMESubType: "html",
						Encoding:    "base64",
						Params:      map[string]string{"charset": "utf-8"},
					},
				},
			},
			{
				MIMEType:          "application",
				MIMESubType:       "pdf",
				Encoding:          "base64",
				Disposition:       "attachment",
				DispositionParams: map[string]string{"filename": "contract.pdf"},
				Size:              15 * 1024 * 1024,
			},
		},
	}

	info := parseMIMEStructure(structure, nil)

	if !info.HasAttachments {
		t.Errorf("expected HasAttachments to be true")
	}
	if len(info.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(info.Attachments))
	}
	if info.Attachments[0].Filename != "contract.pdf" {
		t.Errorf("attachment filename = %q, want 'contract.pdf'", info.Attachments[0].Filename)
	}
	if info.Attachments[0].Size != 15*1024*1024 {
		t.Errorf("attachment size = %d, want 15MB", info.Attachments[0].Size)
	}

	// Verify that HTML and Plain paths target the body leaves and exclude the attachment
	if !slicesEqual(info.HTMLPart.Path, []int{1, 2}) {
		t.Errorf("HTMLPart.Path = %v, want [1, 2]", info.HTMLPart.Path)
	}
	if info.HTMLPart.Encoding != "base64" {
		t.Errorf("HTMLPart.Encoding = %q, want 'base64'", info.HTMLPart.Encoding)
	}

	if !slicesEqual(info.PlainPart.Path, []int{1, 1}) {
		t.Errorf("PlainPart.Path = %v, want [1, 1]", info.PlainPart.Path)
	}
	if info.PlainPart.Encoding != "quoted-printable" {
		t.Errorf("PlainPart.Encoding = %q, want 'quoted-printable'", info.PlainPart.Encoding)
	}
}

func TestDecodeBodyBytes(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		encoding string
		charset  string
		want     string
	}{
		{
			name:     "quoted-printable text",
			raw:      "Hello=20World=21=0D=0AThis=20is=20a=20test=3Dvalue.",
			encoding: "quoted-printable",
			charset:  "utf-8",
			want:     "Hello World!\r\nThis is a test=value.",
		},
		{
			name:     "base64 standard with spaces and newlines",
			raw:      " SGVsbG8g \r\n V29ybGQh ",
			encoding: "base64",
			charset:  "utf-8",
			want:     "Hello World!",
		},
		{
			name:     "base64 without padding",
			raw:      "SGVsbG8", // "Hello" in base64 is "SGVsbG8="
			encoding: "base64",
			charset:  "utf-8",
			want:     "Hello",
		},
		{
			name:     "plain raw 7bit",
			raw:      "Simple plain message",
			encoding: "7bit",
			charset:  "utf-8",
			want:     "Simple plain message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeBodyBytes([]byte(tt.raw), tt.encoding, tt.charset)
			if got != tt.want {
				t.Errorf("decodeBodyBytes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatByteSize(t *testing.T) {
	tests := []struct {
		bytes uint32
		want  string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{15 * 1024 * 1024, "15.0 MB"},
	}

	for _, tt := range tests {
		if got := FormatByteSize(tt.bytes); got != tt.want {
			t.Errorf("FormatByteSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

