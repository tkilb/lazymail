package gmail

import (
	"strings"
	"testing"
	"time"
)

func TestHTMLToPlainText_Basic(t *testing.T) {
	input := `
<!DOCTYPE html>
<html>
<head>
	<title>Newsletter</title>
	<style>
		body { font-family: sans-serif; color: #333; }
		.btn { background: blue; color: white; }
	</style>
	<script>
		console.log("tracking script");
	</script>
</head>
<body>
	<!-- Header comment -->
	<h1>Welcome to LazyMail!</h1>
	<p>This is a fast TUI email client for <b>Gmail</b>.</p>
	<br>
	<div>Here are the features:</div>
	<ul>
		<li>Vim keybindings (j/k)</li>
		<li>Zero-latency encrypted cache</li>
		<li>One-click unsubscribe</li>
	</ul>
	<hr>
	<p>Questions? Contact us &amp; say hello &copy; 2026.</p>
</body>
</html>
`

	got := HTMLToPlainText(input)

	// Verify style and script were stripped
	if strings.Contains(got, "font-family") || strings.Contains(got, "tracking script") {
		t.Errorf("expected style and script to be stripped, got:\n%s", got)
	}

	// Verify comments stripped
	if strings.Contains(got, "Header comment") {
		t.Errorf("expected HTML comment to be stripped, got:\n%s", got)
	}

	// Verify content and bullets
	if !strings.Contains(got, "Welcome to LazyMail!") {
		t.Errorf("missing header in text:\n%s", got)
	}
	if !strings.Contains(got, "• Vim keybindings (j/k)") {
		t.Errorf("missing bullet item in text:\n%s", got)
	}

	// Verify entity decoding
	if !strings.Contains(got, "Contact us & say hello © 2026.") {
		t.Errorf("expected unescaped entities, got:\n%s", got)
	}
}

func TestHTMLToPlainText_Performance(t *testing.T) {
	// Construct a large 100KB HTML payload with heavy style and markup
	var sb strings.Builder
	sb.WriteString("<html><style>")
	for range 500 {
		sb.WriteString(".class-name { margin: 10px; padding: 5px; color: red; }\n")
	}
	sb.WriteString("</style><body>")
	for i := range 1000 {
		sb.WriteString("<p>This is paragraph number <b>")
		sb.WriteString(string(rune('0' + (i % 10))))
		sb.WriteString("</b> with <a href='https://example.com'>a link</a> &amp; special chars.</p>\n")
	}
	sb.WriteString("</body></html>")

	largeHTML := sb.String()
	if len(largeHTML) < 50000 {
		t.Fatalf("test payload too small: %d bytes", len(largeHTML))
	}

	start := time.Now()
	res := HTMLToPlainText(largeHTML)
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Errorf("HTMLToPlainText took too long: %v (expected < 10ms for 100KB)", elapsed)
	}
	if len(res) == 0 || strings.Contains(res, "<style>") || strings.Contains(res, "margin:") {
		t.Errorf("unexpected output from large payload")
	}
}

func TestRenderGlamour_Speed(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("## Heading number\n\nThis is a **paragraph** with [link](https://example.com) and some text.\n\n- item 1\n- item 2\n\n")
	}
	md := sb.String()
	start := time.Now()
	res := RenderGlamour(md, 80)
	elapsed := time.Since(start)
	t.Logf("RenderGlamour on %d bytes took: %v (len %d)", len(md), elapsed, len(res))
}


