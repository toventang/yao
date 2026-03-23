package telegram

import (
	"html"
	"regexp"
	"strings"
)

// FormatTelegramHTML converts standard Markdown to the HTML subset supported by
// Telegram's Bot API. Unsupported constructs (tables, images, etc.) are
// gracefully degraded to plain text.
//
// Supported Telegram HTML tags: <b>, <i>, <u>, <s>, <code>, <pre>, <a>, <blockquote>
func FormatTelegramHTML(md string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")

	var out strings.Builder
	lines := strings.Split(md, "\n")

	inCodeBlock := false
	var codeLang string
	var codeLines []string

	inTable := false
	var tableRows [][]string

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, "```") {
			if !inCodeBlock {
				inCodeBlock = true
				codeLang = strings.TrimSpace(strings.TrimPrefix(line, "```"))
				codeLines = nil
			} else {
				inCodeBlock = false
				if codeLang != "" {
					out.WriteString("<pre><code class=\"language-" + html.EscapeString(codeLang) + "\">")
				} else {
					out.WriteString("<pre>")
				}
				out.WriteString(html.EscapeString(strings.Join(codeLines, "\n")))
				if codeLang != "" {
					out.WriteString("</code></pre>\n")
				} else {
					out.WriteString("</pre>\n")
				}
			}
			continue
		}
		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		if isTableRow(line) {
			if !inTable {
				inTable = true
				tableRows = nil
			}
			if isTableSeparator(line) {
				continue
			}
			tableRows = append(tableRows, parseTableRow(line))
			continue
		}
		if inTable {
			flushTable(&out, tableRows)
			inTable = false
			tableRows = nil
		}

		if line == "---" || line == "***" || line == "___" {
			out.WriteString("——————\n")
			continue
		}

		if m := reHeading.FindStringSubmatch(line); m != nil {
			out.WriteString("<b>" + formatInline(html.EscapeString(m[2])) + "</b>\n")
			continue
		}

		if m := reBlockquote.FindStringSubmatch(line); m != nil {
			out.WriteString("<blockquote>" + formatInline(html.EscapeString(m[1])) + "</blockquote>\n")
			continue
		}

		if m := reUnorderedList.FindStringSubmatch(line); m != nil {
			out.WriteString("• " + formatInline(html.EscapeString(m[1])) + "\n")
			continue
		}

		if m := reOrderedList.FindStringSubmatch(line); m != nil {
			out.WriteString(m[1] + ". " + formatInline(html.EscapeString(m[2])) + "\n")
			continue
		}

		out.WriteString(formatInline(html.EscapeString(line)) + "\n")
	}

	if inCodeBlock && len(codeLines) > 0 {
		out.WriteString("<pre>" + html.EscapeString(strings.Join(codeLines, "\n")) + "</pre>\n")
	}
	if inTable {
		flushTable(&out, tableRows)
	}

	return strings.TrimRight(out.String(), "\n")
}

var (
	reHeading       = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	reBlockquote    = regexp.MustCompile(`^>\s*(.*)$`)
	reUnorderedList = regexp.MustCompile(`^[\s]*[-*+]\s+(.+)$`)
	reOrderedList   = regexp.MustCompile(`^[\s]*(\d+)[.)]\s+(.+)$`)

	reBold          = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reBoldAlt       = regexp.MustCompile(`__(.+?)__`)
	reItalic        = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
	reItalicAlt     = regexp.MustCompile(`(?:^|[^_])_([^_]+?)_(?:[^_]|$)`)
	reStrikethrough = regexp.MustCompile(`~~(.+?)~~`)
	reCode          = regexp.MustCompile("`([^`]+)`")
	reLink          = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

	reTableRow = regexp.MustCompile(`^\|.*\|$`)
	reTableSep = regexp.MustCompile(`^\|[\s\-:|]+\|$`)
)

// formatInline applies inline Markdown formatting to already HTML-escaped text.
// Order matters: code first (to protect its content), then links, bold, italic, etc.
func formatInline(escaped string) string {
	escaped = reCode.ReplaceAllString(escaped, "<code>$1</code>")

	escaped = reLink.ReplaceAllStringFunc(escaped, func(match string) string {
		m := reLink.FindStringSubmatch(match)
		if len(m) < 3 {
			return match
		}
		return `<a href="` + unescapeHTML(m[2]) + `">` + m[1] + `</a>`
	})

	escaped = reBold.ReplaceAllString(escaped, "<b>$1</b>")
	escaped = reBoldAlt.ReplaceAllString(escaped, "<b>$1</b>")
	escaped = reStrikethrough.ReplaceAllString(escaped, "<s>$1</s>")

	return escaped
}

func unescapeHTML(s string) string {
	return html.UnescapeString(s)
}

func isTableRow(line string) bool {
	return reTableRow.MatchString(strings.TrimSpace(line))
}

func isTableSeparator(line string) bool {
	return reTableSep.MatchString(strings.TrimSpace(line))
}

func parseTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cells := strings.Split(line, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

func flushTable(out *strings.Builder, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	out.WriteString("<pre>")
	colWidths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}
	for ri, row := range rows {
		for ci, cell := range row {
			if ci > 0 {
				out.WriteString(" | ")
			}
			w := 0
			if ci < len(colWidths) {
				w = colWidths[ci]
			}
			out.WriteString(html.EscapeString(padRight(cell, w)))
		}
		out.WriteString("\n")
		if ri == 0 && len(rows) > 1 {
			for ci := range row {
				if ci > 0 {
					out.WriteString("-+-")
				}
				w := 0
				if ci < len(colWidths) {
					w = colWidths[ci]
				}
				out.WriteString(strings.Repeat("-", w))
			}
			out.WriteString("\n")
		}
	}
	out.WriteString("</pre>\n")
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
