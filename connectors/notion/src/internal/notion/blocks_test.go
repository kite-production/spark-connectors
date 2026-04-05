package notion

import "testing"

func TestExtractText_Paragraphs(t *testing.T) {
	blocks := []Block{
		{
			Type: "paragraph",
			Paragraph: &RichTextBlock{
				RichText: []RichText{
					{Type: "text", Text: TextBody{Content: "First paragraph."}},
				},
			},
		},
		{
			Type: "paragraph",
			Paragraph: &RichTextBlock{
				RichText: []RichText{
					{Type: "text", Text: TextBody{Content: "Second paragraph."}},
				},
			},
		},
	}

	got := ExtractText(blocks)
	want := "First paragraph.\nSecond paragraph."
	if got != want {
		t.Errorf("ExtractText = %q, want %q", got, want)
	}
}

func TestExtractText_Headings(t *testing.T) {
	blocks := []Block{
		{
			Type: "heading_1",
			Heading1: &RichTextBlock{
				RichText: []RichText{
					{Type: "text", Text: TextBody{Content: "Title"}},
				},
			},
		},
		{
			Type: "heading_2",
			Heading2: &RichTextBlock{
				RichText: []RichText{
					{Type: "text", Text: TextBody{Content: "Subtitle"}},
				},
			},
		},
		{
			Type: "heading_3",
			Heading3: &RichTextBlock{
				RichText: []RichText{
					{Type: "text", Text: TextBody{Content: "Section"}},
				},
			},
		},
		{
			Type: "paragraph",
			Paragraph: &RichTextBlock{
				RichText: []RichText{
					{Type: "text", Text: TextBody{Content: "Body text."}},
				},
			},
		},
	}

	got := ExtractText(blocks)
	want := "Title\nSubtitle\nSection\nBody text."
	if got != want {
		t.Errorf("ExtractText = %q, want %q", got, want)
	}
}

func TestExtractText_EmptyBlocks(t *testing.T) {
	blocks := []Block{
		{Type: "divider"},
		{Type: "image"},
	}

	got := ExtractText(blocks)
	if got != "" {
		t.Errorf("ExtractText = %q, want empty", got)
	}
}

func TestExtractText_NilRichText(t *testing.T) {
	blocks := []Block{
		{Type: "paragraph", Paragraph: nil},
	}

	got := ExtractText(blocks)
	if got != "" {
		t.Errorf("ExtractText = %q, want empty", got)
	}
}

func TestExtractText_MultipleRichTextItems(t *testing.T) {
	blocks := []Block{
		{
			Type: "paragraph",
			Paragraph: &RichTextBlock{
				RichText: []RichText{
					{Type: "text", Text: TextBody{Content: "Hello "}},
					{Type: "text", Text: TextBody{Content: "world!"}},
				},
			},
		},
	}

	got := ExtractText(blocks)
	want := "Hello world!"
	if got != want {
		t.Errorf("ExtractText = %q, want %q", got, want)
	}
}

func TestComposeTextBlocks_SingleParagraph(t *testing.T) {
	blocks := ComposeTextBlocks("Hello world")
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].Type != "paragraph" {
		t.Errorf("Type = %q, want %q", blocks[0].Type, "paragraph")
	}
	if blocks[0].Paragraph.RichText[0].Text.Content != "Hello world" {
		t.Errorf("Content = %q, want %q", blocks[0].Paragraph.RichText[0].Text.Content, "Hello world")
	}
}

func TestComposeTextBlocks_MultipleParagraphs(t *testing.T) {
	blocks := ComposeTextBlocks("First para\n\nSecond para\n\nThird para")
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(blocks))
	}
	if blocks[0].Paragraph.RichText[0].Text.Content != "First para" {
		t.Errorf("block[0] = %q, want %q", blocks[0].Paragraph.RichText[0].Text.Content, "First para")
	}
	if blocks[2].Paragraph.RichText[0].Text.Content != "Third para" {
		t.Errorf("block[2] = %q, want %q", blocks[2].Paragraph.RichText[0].Text.Content, "Third para")
	}
}

func TestComposeTextBlocks_EmptyText(t *testing.T) {
	blocks := ComposeTextBlocks("")
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1 (fallback)", len(blocks))
	}
}

func TestComposeTextBlocks_OnlyWhitespace(t *testing.T) {
	blocks := ComposeTextBlocks("   \n\n   ")
	// All paragraphs are whitespace-only, so they get trimmed to empty.
	// Fallback should produce 1 block.
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
}
