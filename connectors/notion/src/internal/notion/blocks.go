package notion

import "strings"

// ExtractText extracts plain text content from a slice of Notion blocks.
// It processes paragraphs, heading_1, heading_2, and heading_3 blocks.
func ExtractText(blocks []Block) string {
	var parts []string
	for _, b := range blocks {
		var rtb *RichTextBlock
		switch b.Type {
		case "paragraph":
			rtb = b.Paragraph
		case "heading_1":
			rtb = b.Heading1
		case "heading_2":
			rtb = b.Heading2
		case "heading_3":
			rtb = b.Heading3
		}
		if rtb == nil {
			continue
		}
		text := richTextToPlain(rtb.RichText)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// ComposeTextBlocks converts plain text into Notion paragraph blocks.
// Each paragraph in the input (separated by double newlines) becomes
// a separate paragraph block.
func ComposeTextBlocks(text string) []Block {
	paragraphs := strings.Split(text, "\n\n")
	blocks := make([]Block, 0, len(paragraphs))

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		blocks = append(blocks, Block{
			Type: "paragraph",
			Paragraph: &RichTextBlock{
				RichText: []RichText{
					{
						Type: "text",
						Text: TextBody{Content: p},
					},
				},
			},
		})
	}

	if len(blocks) == 0 {
		// At least one block with the full text.
		blocks = append(blocks, Block{
			Type: "paragraph",
			Paragraph: &RichTextBlock{
				RichText: []RichText{
					{
						Type: "text",
						Text: TextBody{Content: text},
					},
				},
			},
		})
	}

	return blocks
}

func richTextToPlain(items []RichText) string {
	var sb strings.Builder
	for _, item := range items {
		sb.WriteString(item.Text.Content)
	}
	return sb.String()
}
