// Package chunker splits long messages at paragraph boundaries.
package chunker

import "strings"

const MaxSlackMessageLength = 4000

// ChunkMessage splits text into chunks that fit within maxLen characters.
// It splits at paragraph boundaries (double newlines) first, then at single
// newlines, and finally at word boundaries as a last resort.
func ChunkMessage(text string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = MaxSlackMessageLength
	}
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= maxLen {
			chunks = append(chunks, remaining)
			break
		}

		chunk := remaining[:maxLen]
		cutAt := -1

		// Try to split at paragraph boundary (double newline).
		if idx := strings.LastIndex(chunk, "\n\n"); idx > 0 {
			cutAt = idx + 2
		}

		// Fall back to single newline.
		if cutAt < 0 {
			if idx := strings.LastIndex(chunk, "\n"); idx > 0 {
				cutAt = idx + 1
			}
		}

		// Fall back to space.
		if cutAt < 0 {
			if idx := strings.LastIndex(chunk, " "); idx > 0 {
				cutAt = idx + 1
			}
		}

		// Last resort: hard cut.
		if cutAt < 0 {
			cutAt = maxLen
		}

		chunks = append(chunks, strings.TrimRight(remaining[:cutAt], " \n"))
		remaining = remaining[cutAt:]
	}

	return chunks
}
