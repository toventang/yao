package discord

import (
	"regexp"
	"strings"
)

// FormatDiscordMarkdown converts standard Markdown to Discord-compatible Markdown.
//
// Discord supports most standard Markdown:
//   - **bold**, *italic*, ~~strikethrough~~
//   - `inline code`, ``` code blocks ```
//   - > blockquote
//   - - unordered list, 1. ordered list
//   - [link](url) (auto-embeds)
//   - # heading (rendered as large bold text)
//
// NOT supported (must be degraded):
//   - tables          → pre-formatted code block
//   - ![image](url)   → just the URL (Discord auto-embeds images from URLs)
//
// Discord has a 2000 character message limit; this function does not truncate.
func FormatDiscordMarkdown(md string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")

	var out strings.Builder
	lines := strings.Split(md, "\n")

	inCodeBlock := false
	inTable := false
	var tableRows [][]string

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
			out.WriteString(line + "\n")
			continue
		}
		if inCodeBlock {
			out.WriteString(line + "\n")
			continue
		}

		if dcIsTableRow(line) {
			if !inTable {
				inTable = true
				tableRows = nil
			}
			if dcIsTableSep(line) {
				continue
			}
			tableRows = append(tableRows, dcParseTableRow(line))
			continue
		}
		if inTable {
			dcFlushTable(&out, tableRows)
			inTable = false
			tableRows = nil
		}

		if m := dcReImage.FindStringSubmatch(line); m != nil {
			out.WriteString(m[2] + "\n")
			continue
		}

		out.WriteString(line + "\n")
	}

	if inTable {
		dcFlushTable(&out, tableRows)
	}

	return strings.TrimRight(out.String(), "\n")
}

var (
	dcReImage    = regexp.MustCompile(`^!\[([^\]]*)\]\(([^)]+)\)$`)
	dcReTableRow = regexp.MustCompile(`^\|.*\|$`)
	dcReTableSep = regexp.MustCompile(`^\|[\s\-:|]+\|$`)
)

func dcIsTableRow(line string) bool {
	return dcReTableRow.MatchString(strings.TrimSpace(line))
}

func dcIsTableSep(line string) bool {
	return dcReTableSep.MatchString(strings.TrimSpace(line))
}

func dcParseTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cells := strings.Split(line, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

func dcFlushTable(out *strings.Builder, rows [][]string) {
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
	out.WriteString("```\n")
	for ri, row := range rows {
		for ci, cell := range row {
			if ci > 0 {
				out.WriteString(" | ")
			}
			w := 0
			if ci < len(colWidths) {
				w = colWidths[ci]
			}
			out.WriteString(dcPadRight(cell, w))
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
	out.WriteString("```\n")
}

func dcPadRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}
