package dingtalk

import (
	"regexp"
	"strings"
)

// FormatDingTalkMarkdown converts standard Markdown to DingTalk's Markdown subset.
//
// DingTalk webhook Markdown supports:
//   - # headings (1-6)
//   - **bold**, *italic*
//   - > blockquote
//   - - unordered list
//   - [link](url)
//   - ![image](url)
//   - ---  divider
//
// NOT supported (must be degraded):
//   - ~~strikethrough~~  → plain text
//   - ``` code blocks    → indented text
//   - `inline code`      → plain text
//   - tables             → pre-formatted text
//   - ordered lists      → "N. " text (passthrough, may not render)
func FormatDingTalkMarkdown(md string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")

	var out strings.Builder
	lines := strings.Split(md, "\n")

	inCodeBlock := false
	var codeLines []string

	inTable := false
	var tableRows [][]string

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, "```") {
			if !inCodeBlock {
				inCodeBlock = true
				codeLines = nil
			} else {
				inCodeBlock = false
				out.WriteString("\n")
				for _, cl := range codeLines {
					out.WriteString("    " + cl + "\n")
				}
				out.WriteString("\n")
			}
			continue
		}
		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		if dtIsTableRow(line) {
			if !inTable {
				inTable = true
				tableRows = nil
			}
			if dtIsTableSep(line) {
				continue
			}
			tableRows = append(tableRows, dtParseTableRow(line))
			continue
		}
		if inTable {
			dtFlushTable(&out, tableRows)
			inTable = false
			tableRows = nil
		}

		line = dtReStrikethrough.ReplaceAllString(line, "$1")
		line = dtReInlineCode.ReplaceAllString(line, "$1")

		out.WriteString(line + "\n")
	}

	if inCodeBlock && len(codeLines) > 0 {
		out.WriteString("\n")
		for _, cl := range codeLines {
			out.WriteString("    " + cl + "\n")
		}
		out.WriteString("\n")
	}
	if inTable {
		dtFlushTable(&out, tableRows)
	}

	return strings.TrimRight(out.String(), "\n")
}

var (
	dtReStrikethrough = regexp.MustCompile(`~~(.+?)~~`)
	dtReInlineCode    = regexp.MustCompile("`([^`]+)`")
	dtReTableRow      = regexp.MustCompile(`^\|.*\|$`)
	dtReTableSep      = regexp.MustCompile(`^\|[\s\-:|]+\|$`)
)

func dtIsTableRow(line string) bool {
	return dtReTableRow.MatchString(strings.TrimSpace(line))
}

func dtIsTableSep(line string) bool {
	return dtReTableSep.MatchString(strings.TrimSpace(line))
}

func dtParseTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cells := strings.Split(line, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

func dtFlushTable(out *strings.Builder, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	colWidths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len([]rune(cell)) > colWidths[i] {
				colWidths[i] = len([]rune(cell))
			}
		}
	}
	out.WriteString("\n")
	for ri, row := range rows {
		for ci, cell := range row {
			if ci > 0 {
				out.WriteString(" | ")
			}
			w := 0
			if ci < len(colWidths) {
				w = colWidths[ci]
			}
			out.WriteString(dtPadRight(cell, w))
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
	out.WriteString("\n")
}

func dtPadRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}
