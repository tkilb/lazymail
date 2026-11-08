package gmail

import (
	"html"
	"strings"
	"unicode"

	"github.com/charmbracelet/glamour"
)

// HTMLToPlainText converts an HTML string into clean, readable plain text in <1ms.
// It strips scripts, styles, and comments, replaces structural tags with newlines/bullets,
// unescapes HTML entities, and collapses excessive whitespace.
func HTMLToPlainText(rawHTML string) string {
	if rawHTML == "" {
		return ""
	}

	var sb strings.Builder
	sb.Grow(len(rawHTML) / 2)

	inTag := false
	inScript := false
	inStyle := false
	inComment := false

	var currentTag strings.Builder

	n := len(rawHTML)
	for i := 0; i < n; i++ {
		ch := rawHTML[i]

		// Handle comments <!-- ... -->
		if inComment {
			if ch == '>' && i >= 2 && rawHTML[i-1] == '-' && rawHTML[i-2] == '-' {
				inComment = false
			}
			continue
		}

		// Handle <style>...</style>
		if inStyle {
			if ch == '<' && i+7 < n && strings.EqualFold(rawHTML[i:i+8], "</style>") {
				inStyle = false
				i += 7
			}
			continue
		}

		// Handle <script>...</script>
		if inScript {
			if ch == '<' && i+8 < n && strings.EqualFold(rawHTML[i:i+9], "</script>") {
				inScript = false
				i += 8
			}
			continue
		}

		if ch == '<' {
			inTag = true
			currentTag.Reset()

			// Check for comment start <!--
			if i+3 < n && rawHTML[i+1] == '!' && rawHTML[i+2] == '-' && rawHTML[i+3] == '-' {
				inComment = true
				i += 3
				inTag = false
			}
			continue
		}

		if inTag {
			if ch == '>' {
				inTag = false
				tagName := strings.ToLower(strings.TrimSpace(currentTag.String()))

				// Check tag types
				if strings.HasPrefix(tagName, "style") {
					inStyle = true
				} else if strings.HasPrefix(tagName, "script") {
					inScript = true
				} else if strings.HasPrefix(tagName, "br") {
					sb.WriteByte('\n')
				} else if strings.HasPrefix(tagName, "p") || strings.HasPrefix(tagName, "/p") ||
					strings.HasPrefix(tagName, "div") || strings.HasPrefix(tagName, "/div") ||
					strings.HasPrefix(tagName, "tr") || strings.HasPrefix(tagName, "/tr") ||
					strings.HasPrefix(tagName, "h1") || strings.HasPrefix(tagName, "/h1") ||
					strings.HasPrefix(tagName, "h2") || strings.HasPrefix(tagName, "/h2") ||
					strings.HasPrefix(tagName, "h3") || strings.HasPrefix(tagName, "/h3") ||
					strings.HasPrefix(tagName, "/li") {
					sb.WriteByte('\n')
				} else if strings.HasPrefix(tagName, "li") {
					sb.WriteString("\n• ")
				} else if strings.HasPrefix(tagName, "hr") {
					sb.WriteString("\n───\n")
				} else if strings.HasPrefix(tagName, "td") || strings.HasPrefix(tagName, "th") {
					sb.WriteByte(' ')
				}
				continue
			}
			currentTag.WriteByte(ch)
			continue
		}

		sb.WriteByte(ch)
	}

	// Unescape HTML entities (&nbsp;, &amp;, &lt;, &gt;, &quot;, &#39;, etc.)
	unescaped := html.UnescapeString(sb.String())

	return cleanWhitespace(unescaped)
}

func cleanWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	consecutiveBlank := 0

	for _, line := range lines {
		trimmed := strings.TrimRightFunc(line, unicode.IsSpace)
		if strings.TrimSpace(trimmed) == "" {
			consecutiveBlank++
			if consecutiveBlank <= 1 {
				result = append(result, "")
			}
		} else {
			consecutiveBlank = 0
			result = append(result, trimmed)
		}
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

func extractHref(tag string) string {
	lower := strings.ToLower(tag)
	idx := strings.Index(lower, "href=")
	if idx == -1 {
		return ""
	}
	rem := strings.TrimSpace(tag[idx+5:])
	if len(rem) == 0 {
		return ""
	}
	quote := rem[0]
	if quote == '"' || quote == '\'' {
		rem = rem[1:]
		end := strings.IndexByte(rem, quote)
		if end != -1 {
			return rem[:end]
		}
	} else {
		end := strings.IndexAny(rem, " >")
		if end != -1 {
			return rem[:end]
		}
		return rem
	}
	return ""
}

// HTMLToMarkdown converts HTML into clean Markdown suitable for terminal rendering.
func HTMLToMarkdown(rawHTML string) string {
	if rawHTML == "" {
		return ""
	}

	var sb strings.Builder
	sb.Grow(len(rawHTML) / 2)

	inTag := false
	inScript := false
	inStyle := false
	inComment := false

	var currentTag strings.Builder
	var activeHref string

	n := len(rawHTML)
	for i := 0; i < n; i++ {
		ch := rawHTML[i]

		// Handle comments <!-- ... -->
		if inComment {
			if ch == '>' && i >= 2 && rawHTML[i-1] == '-' && rawHTML[i-2] == '-' {
				inComment = false
			}
			continue
		}

		// Handle <style>...</style>
		if inStyle {
			if ch == '<' && i+7 < n && strings.EqualFold(rawHTML[i:i+8], "</style>") {
				inStyle = false
				i += 7
			}
			continue
		}

		// Handle <script>...</script>
		if inScript {
			if ch == '<' && i+8 < n && strings.EqualFold(rawHTML[i:i+9], "</script>") {
				inScript = false
				i += 8
			}
			continue
		}

		if ch == '<' {
			inTag = true
			currentTag.Reset()

			// Check for comment start <!--
			if i+3 < n && rawHTML[i+1] == '!' && rawHTML[i+2] == '-' && rawHTML[i+3] == '-' {
				inComment = true
				i += 3
				inTag = false
			}
			continue
		}

		if inTag {
			if ch == '>' {
				inTag = false
				tagStr := currentTag.String()
				tagLower := strings.ToLower(strings.TrimSpace(tagStr))

				// Check tag types
				if strings.HasPrefix(tagLower, "style") {
					inStyle = true
				} else if strings.HasPrefix(tagLower, "script") {
					inScript = true
				} else if strings.HasPrefix(tagLower, "br") {
					sb.WriteString("  \n")
				} else if tagLower == "p" || tagLower == "/p" ||
					tagLower == "div" || tagLower == "/div" ||
					strings.HasPrefix(tagLower, "p ") || strings.HasPrefix(tagLower, "div ") {
					sb.WriteString("\n\n")
				} else if tagLower == "h1" || strings.HasPrefix(tagLower, "h1 ") {
					sb.WriteString("\n\n# ")
				} else if tagLower == "/h1" {
					sb.WriteString("\n\n")
				} else if tagLower == "h2" || strings.HasPrefix(tagLower, "h2 ") {
					sb.WriteString("\n\n## ")
				} else if tagLower == "/h2" {
					sb.WriteString("\n\n")
				} else if tagLower == "h3" || strings.HasPrefix(tagLower, "h3 ") {
					sb.WriteString("\n\n### ")
				} else if tagLower == "/h3" {
					sb.WriteString("\n\n")
				} else if tagLower == "b" || tagLower == "/b" || tagLower == "strong" || tagLower == "/strong" {
					sb.WriteString("**")
				} else if tagLower == "i" || tagLower == "/i" || tagLower == "em" || tagLower == "/em" {
					sb.WriteString("*")
				} else if tagLower == "code" || tagLower == "/code" {
					sb.WriteString("`")
				} else if tagLower == "pre" || strings.HasPrefix(tagLower, "pre ") {
					sb.WriteString("\n```\n")
				} else if tagLower == "/pre" {
					sb.WriteString("\n```\n")
				} else if tagLower == "blockquote" || strings.HasPrefix(tagLower, "blockquote ") {
					sb.WriteString("\n> ")
				} else if tagLower == "/blockquote" {
					sb.WriteString("\n")
				} else if tagLower == "li" || strings.HasPrefix(tagLower, "li ") {
					sb.WriteString("\n- ")
				} else if tagLower == "/li" {
					sb.WriteString("\n")
				} else if tagLower == "hr" || strings.HasPrefix(tagLower, "hr ") {
					sb.WriteString("\n\n---\n\n")
				} else if strings.HasPrefix(tagLower, "a ") {
					href := extractHref(tagStr)
					if href != "" && !strings.HasPrefix(href, "javascript:") {
						activeHref = href
						sb.WriteByte('[')
					}
				} else if tagLower == "/a" {
					if activeHref != "" {
						sb.WriteString("](")
						sb.WriteString(activeHref)
						sb.WriteByte(')')
						activeHref = ""
					}
				}
				continue
			}
			currentTag.WriteByte(ch)
			continue
		}

		sb.WriteByte(ch)
	}

	unescaped := html.UnescapeString(sb.String())
	return cleanWhitespace(unescaped)
}

// RenderGlamour formats markdown into terminal ANSI text using Glamour.
func RenderGlamour(markdown string, width int) string {
	if width < 20 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return markdown
	}
	rendered, err := r.Render(markdown)
	if err != nil {
		return markdown
	}
	return strings.TrimSpace(rendered)
}

