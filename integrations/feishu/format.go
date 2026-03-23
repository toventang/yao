package feishu

import (
	"regexp"
	"strings"
)

// FormatFeishuMarkdown converts standard Markdown to Feishu's lark_md subset.
//
// Feishu lark_md (in card div elements) supports:
//   - **bold**, *italic*, ~~strikethrough~~
//   - `inline code`
//   - [link](url)
//   - ---  (divider)
//
// NOT supported (must be converted/degraded):
//   - # headings        → **bold text**
//   - ``` code blocks   → plain text indented
//   - > blockquotes     → text with "│ " prefix
//   - tables            → pre-formatted text
//   - - list items      → "• " prefixed text
//   - 1. ordered list   → "N. " prefixed text
//   - ![](url) images   → [image](url) link
func FormatFeishuMarkdown(md string) string {
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
				for _, cl := range codeLines {
					out.WriteString("    " + cl + "\n")
				}
			}
			continue
		}
		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		if fmtIsTableRow(line) {
			if !inTable {
				inTable = true
				tableRows = nil
			}
			if fmtIsTableSep(line) {
				continue
			}
			tableRows = append(tableRows, fmtParseTableRow(line))
			continue
		}
		if inTable {
			fmtFlushTable(&out, tableRows)
			inTable = false
			tableRows = nil
		}

		if line == "---" || line == "***" || line == "___" {
			out.WriteString("---\n")
			continue
		}

		if m := fmtReHeading.FindStringSubmatch(line); m != nil {
			out.WriteString("**" + m[2] + "**\n")
			continue
		}

		if m := fmtReBlockquote.FindStringSubmatch(line); m != nil {
			out.WriteString("│ " + m[1] + "\n")
			continue
		}

		if m := fmtReUnorderedList.FindStringSubmatch(line); m != nil {
			out.WriteString("• " + m[1] + "\n")
			continue
		}

		if m := fmtReOrderedList.FindStringSubmatch(line); m != nil {
			out.WriteString(m[1] + ". " + m[2] + "\n")
			continue
		}

		if m := fmtReImage.FindStringSubmatch(line); m != nil {
			out.WriteString("[" + m[1] + "](" + m[2] + ")\n")
			continue
		}

		out.WriteString(line + "\n")
	}

	if inCodeBlock && len(codeLines) > 0 {
		for _, cl := range codeLines {
			out.WriteString("    " + cl + "\n")
		}
	}
	if inTable {
		fmtFlushTable(&out, tableRows)
	}

	return strings.TrimRight(out.String(), "\n")
}

var (
	fmtReHeading       = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	fmtReBlockquote    = regexp.MustCompile(`^>\s*(.*)$`)
	fmtReUnorderedList = regexp.MustCompile(`^[\s]*[-*+]\s+(.+)$`)
	fmtReOrderedList   = regexp.MustCompile(`^[\s]*(\d+)[.)]\s+(.+)$`)
	fmtReImage         = regexp.MustCompile(`^!\[([^\]]*)\]\(([^)]+)\)$`)
	fmtReTableRow      = regexp.MustCompile(`^\|.*\|$`)
	fmtReTableSep      = regexp.MustCompile(`^\|[\s\-:|]+\|$`)
)

func fmtIsTableRow(line string) bool {
	return fmtReTableRow.MatchString(strings.TrimSpace(line))
}

func fmtIsTableSep(line string) bool {
	return fmtReTableSep.MatchString(strings.TrimSpace(line))
}

func fmtParseTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cells := strings.Split(line, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

func fmtFlushTable(out *strings.Builder, rows [][]string) {
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
	for ri, row := range rows {
		for ci, cell := range row {
			if ci > 0 {
				out.WriteString(" | ")
			}
			w := 0
			if ci < len(colWidths) {
				w = colWidths[ci]
			}
			out.WriteString(fmtPadRight(cell, w))
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
}

func fmtPadRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}
