package chunking

import (
	"strings"
	"unicode/utf8"
)

// Chunk represents a segment of text from a source document.
type Chunk struct {
	Index int    // 0-based position in the document
	Text  string // The chunk content
	Start int    // Byte offset of chunk start in source
	End   int    // Byte offset of chunk end in source (exclusive)
}

// Split divides content into overlapping chunks that fit within maxBytes.
// It splits at semantic boundaries in priority order:
//  1. Markdown section headers (\n## ... or \n# ...)
//  2. Double newlines (paragraph breaks)
//  3. Single newlines (line breaks)
//  4. Hard cut at maxBytes (last resort)
//
// overlapBytes specifies how many bytes from the end of one chunk are
// repeated at the start of the next. If content fits in a single chunk,
// it is returned as-is with no overlap.
//
// Empty content returns an empty (nil) slice.
func Split(content string, maxBytes, overlapBytes int) []Chunk {
	if content == "" {
		return nil
	}

	if len(content) <= maxBytes {
		return []Chunk{
			{
				Index: 0,
				Text:  content,
				Start: 0,
				End:   len(content),
			},
		}
	}

	var chunks []Chunk
	pos := 0
	index := 0

	for pos < len(content) {
		end := pos + maxBytes
		if end >= len(content) {
			// Remaining content fits in one final chunk.
			chunks = append(chunks, Chunk{
				Index: index,
				Text:  content[pos:],
				Start: pos,
				End:   len(content),
			})
			break
		}

		// Find the best split point at or before end.
		splitAt := findSplitPoint(content, pos, end)

		// Guard against zero-length chunks: if the split point did not
		// advance past pos (e.g. a boundary marker sits right at pos),
		// fall back to a hard cut at end to guarantee progress.
		if splitAt <= pos {
			splitAt = alignToRuneBoundaryBackward(content, end)
			if splitAt <= pos {
				splitAt = alignToRuneBoundary(content, end)
			}
		}

		chunks = append(chunks, Chunk{
			Index: index,
			Text:  content[pos:splitAt],
			Start: pos,
			End:   splitAt,
		})

		// Advance position with overlap.
		nextPos := splitAt - overlapBytes
		if nextPos <= pos {
			// Ensure forward progress: at minimum, advance past current pos.
			nextPos = splitAt
		}
		// Ensure we do not start mid-rune after applying overlap.
		nextPos = alignToRuneBoundary(content, nextPos)
		pos = nextPos
		index++
	}

	return chunks
}

// findSplitPoint searches backward from end for the best semantic boundary.
// It returns a byte offset in content where the chunk should end (exclusive).
// The search region is [pos, end).
func findSplitPoint(content string, pos, end int) int {
	region := content[pos:end]

	// Priority 1: Markdown header (\n## or \n# ).
	if idx := lastIndexOfHeader(region); idx >= 0 {
		splitAt := pos + idx
		return alignToRuneBoundary(content, splitAt)
	}

	// Priority 2: Paragraph break (\n\n).
	if idx := strings.LastIndex(region, "\n\n"); idx >= 0 {
		splitAt := pos + idx
		return alignToRuneBoundary(content, splitAt)
	}

	// Priority 3: Line break (\n).
	if idx := strings.LastIndex(region, "\n"); idx >= 0 {
		// Split after the newline (include it in the current chunk).
		splitAt := pos + idx + 1
		return alignToRuneBoundary(content, splitAt)
	}

	// Priority 4: Hard cut at end, but ensure UTF-8 safety.
	return alignToRuneBoundaryBackward(content, end)
}

// lastIndexOfHeader returns the index of the last occurrence of a markdown
// header marker (\n## or \n# ) within s. Returns -1 if not found.
// The returned index points to the \n that starts the header.
func lastIndexOfHeader(s string) int {
	best := -1

	// Search for \n## (h2 headers).
	if idx := strings.LastIndex(s, "\n## "); idx >= 0 {
		best = idx
	}

	// Search for \n# (h1 headers). Only use if it is later than \n##.
	if idx := strings.LastIndex(s, "\n# "); idx >= 0 && idx > best {
		best = idx
	}

	return best
}

// alignToRuneBoundary ensures offset is at a valid rune start in content.
// If offset is mid-rune, it advances to the next rune boundary.
func alignToRuneBoundary(content string, offset int) int {
	if offset >= len(content) {
		return len(content)
	}
	if offset <= 0 {
		return 0
	}
	for offset < len(content) && !utf8.RuneStart(content[offset]) {
		offset++
	}
	return offset
}

// alignToRuneBoundaryBackward ensures offset is at a valid rune start in
// content. If offset is mid-rune, it moves backward to the start of the rune.
func alignToRuneBoundaryBackward(content string, offset int) int {
	if offset >= len(content) {
		return len(content)
	}
	if offset <= 0 {
		return 0
	}
	for offset > 0 && !utf8.RuneStart(content[offset]) {
		offset--
	}
	return offset
}
