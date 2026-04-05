// Package formatter converts standard markdown to Slack mrkdwn format.
package formatter

import (
	"regexp"
	"strings"
)

var (
	// Bold: **text** or __text__
	boldDoubleAsteriskRe = regexp.MustCompile(`\*\*(.+?)\*\*`)
	boldDoubleUnderRe    = regexp.MustCompile(`__(.+?)__`)

	// Italic: *text* (single asterisk) → _text_
	// Matches a single * not preceded/followed by another *.
	singleAsteriskRe = regexp.MustCompile(`(?:^|[^*\x00])\*([^*\x00]+?)\*(?:[^*\x00]|$)`)

	// Strikethrough: ~~text~~ → ~text~
	strikethroughRe = regexp.MustCompile(`~~(.+?)~~`)

	// Links: [text](url) → <url|text>
	linkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

	// Headers: # Header → *Header*
	headerRe = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
)

const boldPlaceholderOpen = "\x00BOLD_OPEN\x00"
const boldPlaceholderClose = "\x00BOLD_CLOSE\x00"

// MarkdownToMrkdwn converts standard markdown text to Slack mrkdwn format.
func MarkdownToMrkdwn(md string) string {
	s := md

	// Step 1: Replace bold markers with placeholders to protect from italic conversion.
	s = boldDoubleAsteriskRe.ReplaceAllString(s, boldPlaceholderOpen+"$1"+boldPlaceholderClose)
	s = boldDoubleUnderRe.ReplaceAllString(s, boldPlaceholderOpen+"$1"+boldPlaceholderClose)

	// Step 2: Convert single-asterisk italics to underscores: *text* → _text_
	s = convertItalics(s)

	// Step 3: Restore bold placeholders to Slack bold markers (*text*).
	s = strings.ReplaceAll(s, boldPlaceholderOpen, "*")
	s = strings.ReplaceAll(s, boldPlaceholderClose, "*")

	// Step 4: Strikethrough ~~text~~ → ~text~
	s = strikethroughRe.ReplaceAllString(s, "~$1~")

	// Step 5: Links [text](url) → <url|text>
	s = linkRe.ReplaceAllString(s, "<$2|$1>")

	// Step 6: Headers # text → *text*
	s = headerRe.ReplaceAllString(s, "*$1*")

	return s
}

// convertItalics converts markdown single-asterisk italics (*text*) to
// Slack underscored italics (_text_), skipping code blocks and inline code.
func convertItalics(s string) string {
	var result strings.Builder
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		// Skip code blocks.
		if runes[i] == '`' {
			if i+2 < len(runes) && runes[i+1] == '`' && runes[i+2] == '`' {
				result.WriteRune('`')
				result.WriteRune('`')
				result.WriteRune('`')
				i += 3
				for i < len(runes) {
					if i+2 < len(runes) && runes[i] == '`' && runes[i+1] == '`' && runes[i+2] == '`' {
						result.WriteRune('`')
						result.WriteRune('`')
						result.WriteRune('`')
						i += 3
						break
					}
					result.WriteRune(runes[i])
					i++
				}
				continue
			}
			// Single backtick inline code.
			result.WriteRune('`')
			i++
			for i < len(runes) && runes[i] != '`' {
				result.WriteRune(runes[i])
				i++
			}
			if i < len(runes) {
				result.WriteRune('`')
				i++
			}
			continue
		}

		// Single * not adjacent to another * → convert to _
		if runes[i] == '*' {
			adjacent := false
			if i+1 < len(runes) && runes[i+1] == '*' {
				adjacent = true
			}
			if i > 0 && runes[i-1] == '*' {
				adjacent = true
			}
			if !adjacent {
				result.WriteRune('_')
				i++
				continue
			}
		}

		result.WriteRune(runes[i])
		i++
	}
	return result.String()
}
