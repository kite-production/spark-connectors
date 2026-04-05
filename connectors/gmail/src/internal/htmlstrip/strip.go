// Package htmlstrip provides a simple HTML-to-plain-text converter.
package htmlstrip

import (
	"regexp"
	"strings"
)

var (
	scriptRe     = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe      = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	tagRe        = regexp.MustCompile(`<[^>]+>`)
	whitespaceRe = regexp.MustCompile(`[ \t]+`)
	blankLinesRe = regexp.MustCompile(`\n{3,}`)
)

// htmlEntities maps common HTML entities to their text equivalents.
var htmlEntities = map[string]string{
	"&amp;":  "&",
	"&lt;":   "<",
	"&gt;":   ">",
	"&quot;": `"`,
	"&#39;":  "'",
	"&apos;": "'",
	"&nbsp;": " ",
}

// Strip removes HTML tags and returns plain text.
func Strip(html string) string {
	// Remove script and style blocks.
	s := scriptRe.ReplaceAllString(html, "")
	s = styleRe.ReplaceAllString(s, "")

	// Replace <br> and <p> with newlines before stripping tags.
	s = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</li>`).ReplaceAllString(s, "\n")
	s = regexp.MustCompile(`(?i)<p[^>]*>|<div[^>]*>|<li[^>]*>`).ReplaceAllString(s, "\n")

	// Strip remaining tags.
	s = tagRe.ReplaceAllString(s, "")

	// Decode common HTML entities.
	for entity, replacement := range htmlEntities {
		s = strings.ReplaceAll(s, entity, replacement)
	}

	// Collapse horizontal whitespace.
	s = whitespaceRe.ReplaceAllString(s, " ")

	// Collapse excessive blank lines.
	s = blankLinesRe.ReplaceAllString(s, "\n\n")

	return strings.TrimSpace(s)
}
