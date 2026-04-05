package chunker

import (
	"strings"
	"testing"
)

func TestChunkMessage(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxLen   int
		wantN    int
		checkAll bool // if true, verify all chunks are <= maxLen
	}{
		{
			name:   "short message no chunking",
			text:   "Hello world",
			maxLen: 4000,
			wantN:  1,
		},
		{
			name:   "exact max length",
			text:   strings.Repeat("a", 4000),
			maxLen: 4000,
			wantN:  1,
		},
		{
			name:     "split at paragraph boundary",
			text:     strings.Repeat("a", 30) + "\n\n" + strings.Repeat("b", 30),
			maxLen:   50,
			wantN:    2,
			checkAll: true,
		},
		{
			name:     "split at newline when no paragraph boundary",
			text:     strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30),
			maxLen:   50,
			wantN:    2,
			checkAll: true,
		},
		{
			name:     "split at space",
			text:     strings.Repeat("a", 30) + " " + strings.Repeat("b", 30),
			maxLen:   50,
			wantN:    2,
			checkAll: true,
		},
		{
			name:     "hard split when no boundaries",
			text:     strings.Repeat("x", 120),
			maxLen:   50,
			wantN:    3,
			checkAll: true,
		},
		{
			name:   "empty string",
			text:   "",
			maxLen: 100,
			wantN:  1,
		},
		{
			name:   "default max length used when zero",
			text:   strings.Repeat("a", MaxSlackMessageLength+1),
			maxLen: 0,
			wantN:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkMessage(tt.text, tt.maxLen)
			if len(chunks) != tt.wantN {
				t.Errorf("got %d chunks, want %d", len(chunks), tt.wantN)
			}
			if tt.checkAll {
				effectiveMax := tt.maxLen
				if effectiveMax <= 0 {
					effectiveMax = MaxSlackMessageLength
				}
				for i, c := range chunks {
					if len(c) > effectiveMax {
						t.Errorf("chunk %d has length %d, exceeds max %d", i, len(c), effectiveMax)
					}
				}
			}
			// Verify content is preserved (concatenated chunks contain all content).
			joined := strings.Join(chunks, "")
			// Remove whitespace for comparison since chunker trims.
			original := strings.ReplaceAll(tt.text, " ", "")
			original = strings.ReplaceAll(original, "\n", "")
			result := strings.ReplaceAll(joined, " ", "")
			result = strings.ReplaceAll(result, "\n", "")
			if result != original {
				t.Errorf("content not preserved: got %d chars, want %d chars", len(result), len(original))
			}
		})
	}
}

func TestChunkMessage_ParagraphPreference(t *testing.T) {
	// Build text with a paragraph boundary at position 40 and newline at 70.
	text := strings.Repeat("a", 38) + "\n\n" + strings.Repeat("b", 28) + "\n" + strings.Repeat("c", 30)
	chunks := ChunkMessage(text, 50)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// First chunk should end at the paragraph boundary.
	if !strings.HasSuffix(chunks[0], strings.Repeat("a", 38)) {
		t.Errorf("first chunk should end at paragraph boundary, got %q", chunks[0])
	}
}
