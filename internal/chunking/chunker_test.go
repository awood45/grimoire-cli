package chunking

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSplit_SingleChunk verifies that content under maxBytes returns a single
// chunk with correct offsets (FR-2, NFR-1).
func TestSplit_SingleChunk(t *testing.T) {
	content := "Hello, world!"
	chunks := Split(content, 100, 10)

	require.Len(t, chunks, 1)
	assert.Equal(t, 0, chunks[0].Index)
	assert.Equal(t, content, chunks[0].Text)
	assert.Equal(t, 0, chunks[0].Start)
	assert.Equal(t, len(content), chunks[0].End)
}

// TestSplit_MultipleChunks verifies that content exceeding maxBytes splits into
// multiple chunks (FR-2, NFR-1).
func TestSplit_MultipleChunks(t *testing.T) {
	// Create content that exceeds maxBytes and has line breaks for splitting.
	line := "abcdefghij\n" // 11 bytes per line
	content := strings.Repeat(line, 10)
	// Total: 110 bytes. With maxBytes=50, overlap=5, expect multiple chunks.
	chunks := Split(content, 50, 5)

	assert.Greater(t, len(chunks), 1, "expected multiple chunks for content exceeding maxBytes")
	// All chunks should be non-empty.
	for _, c := range chunks {
		assert.NotEmpty(t, c.Text)
	}
	// Indices should be sequential.
	for i, c := range chunks {
		assert.Equal(t, i, c.Index)
	}
}

// TestSplit_OverlapPreserved verifies that adjacent chunks share overlapBytes of
// text (FR-2).
func TestSplit_OverlapPreserved(t *testing.T) {
	// Build content with clean line breaks for predictable splitting.
	// Each line is 20 bytes (19 chars + newline).
	line := strings.Repeat("a", 19) + "\n"
	content := strings.Repeat(line, 10) // 200 bytes total
	overlapBytes := 20

	chunks := Split(content, 60, overlapBytes)

	require.Greater(t, len(chunks), 1, "need at least 2 chunks to test overlap")

	for i := 1; i < len(chunks); i++ {
		prevEnd := chunks[i-1].Text
		currStart := chunks[i].Text

		// The last overlapBytes of the previous chunk should appear at the
		// beginning of the current chunk.
		overlapFromPrev := prevEnd[len(prevEnd)-overlapBytes:]
		overlapFromCurr := currStart[:overlapBytes]
		assert.Equal(t, overlapFromPrev, overlapFromCurr,
			"chunks %d and %d should share %d bytes of overlap", i-1, i, overlapBytes)
	}
}

// TestSplit_SplitAtHeader verifies that the splitter prefers splitting at
// markdown headers (\n## or \n# ) (FR-2).
func TestSplit_SplitAtHeader(t *testing.T) {
	// Content with a markdown header somewhere in the middle.
	part1 := strings.Repeat("x", 30)
	header := "\n## Section Two\n"
	part2 := strings.Repeat("y", 30)
	content := part1 + header + part2

	// maxBytes large enough to include part1 + header + some of part2,
	// but not the whole content.
	maxBytes := len(part1) + len(header) + 10
	chunks := Split(content, maxBytes, 5)

	require.Greater(t, len(chunks), 1, "should produce multiple chunks")
	// First chunk should end at the header boundary (before the header).
	// The header starts with \n## so the split should occur right before
	// the \n## marker.
	assert.True(t, strings.HasSuffix(chunks[0].Text, strings.Repeat("x", 30)),
		"first chunk should end before the header: got %q", chunks[0].Text)
	assert.True(t, strings.HasPrefix(chunks[1].Text, "\n## Section Two") ||
		strings.HasPrefix(strings.TrimLeft(chunks[1].Text, "x"), "\n## Section Two"),
		"second chunk should start at or near the header")
}

// TestSplit_SplitAtParagraph verifies that the splitter falls back to
// paragraph breaks (\n\n) when no header is available (FR-2).
func TestSplit_SplitAtParagraph(t *testing.T) {
	// Content with paragraph breaks but no headers.
	part1 := strings.Repeat("a", 30)
	paraBreak := "\n\n"
	part2 := strings.Repeat("b", 30)
	content := part1 + paraBreak + part2

	maxBytes := len(part1) + len(paraBreak) + 10
	chunks := Split(content, maxBytes, 5)

	require.Greater(t, len(chunks), 1)
	// First chunk should end at or include the paragraph break.
	assert.Contains(t, chunks[0].Text, part1,
		"first chunk should contain text before paragraph break")
}

// TestSplit_SplitAtNewline verifies that the splitter falls back to single
// newline when no header or paragraph break is available (FR-2).
func TestSplit_SplitAtNewline(t *testing.T) {
	// Content with only single newlines (no headers, no double newlines).
	lines := []string{
		strings.Repeat("a", 20),
		strings.Repeat("b", 20),
		strings.Repeat("c", 20),
		strings.Repeat("d", 20),
	}
	content := strings.Join(lines, "\n")

	// maxBytes allows roughly two lines.
	chunks := Split(content, 42, 5)

	require.Greater(t, len(chunks), 1)
	// First chunk should end at a newline boundary. The implementation splits
	// after the newline, so the chunk includes the trailing \n.
	assert.True(t, strings.HasSuffix(chunks[0].Text, "\n"),
		"first chunk should end at a newline boundary, got: %q", chunks[0].Text)
	// Verify it contains the expected lines.
	assert.Contains(t, chunks[0].Text, lines[0],
		"first chunk should contain first line")
	assert.Contains(t, chunks[0].Text, lines[1],
		"first chunk should contain second line")
}

// TestSplit_HardCut verifies that content with no newlines at all is hard-cut
// at maxBytes (FR-2).
func TestSplit_HardCut(t *testing.T) {
	// A long string with no newlines.
	content := strings.Repeat("x", 100)

	chunks := Split(content, 40, 10)

	require.Greater(t, len(chunks), 1, "should produce multiple chunks via hard cut")
	// First chunk should be exactly maxBytes long.
	assert.Len(t, chunks[0].Text, 40, "first chunk should be exactly maxBytes")
}

// TestSplit_EmptyContent verifies that empty input returns an empty slice (FR-2).
func TestSplit_EmptyContent(t *testing.T) {
	chunks := Split("", 100, 10)
	assert.Empty(t, chunks, "empty content should produce empty slice")
}

// TestSplit_ExactlyMaxBytes verifies that content at exactly maxBytes returns a
// single chunk (FR-2).
func TestSplit_ExactlyMaxBytes(t *testing.T) {
	content := strings.Repeat("a", 50)
	chunks := Split(content, 50, 10)

	require.Len(t, chunks, 1)
	assert.Equal(t, content, chunks[0].Text)
	assert.Equal(t, 0, chunks[0].Start)
	assert.Equal(t, 50, chunks[0].End)
}

// TestSplit_ChunkOffsets verifies that Start/End offsets are correct for all
// chunks, covering the entire content (FR-2).
func TestSplit_ChunkOffsets(t *testing.T) {
	line := strings.Repeat("m", 19) + "\n" // 20 bytes per line
	content := strings.Repeat(line, 10)    // 200 bytes total
	overlapBytes := 10

	chunks := Split(content, 50, overlapBytes)
	require.Greater(t, len(chunks), 1)

	// First chunk starts at 0.
	assert.Equal(t, 0, chunks[0].Start, "first chunk should start at 0")

	// Last chunk ends at len(content).
	assert.Equal(t, len(content), chunks[len(chunks)-1].End,
		"last chunk should end at len(content)")

	for i, c := range chunks {
		// Text should match the byte range in the original content.
		assert.Equal(t, content[c.Start:c.End], c.Text,
			"chunk %d text should match content[%d:%d]", i, c.Start, c.End)
		// Length of text should match End - Start.
		assert.Len(t, c.Text, c.End-c.Start,
			"chunk %d: End-Start should equal len(Text)", i)
		// Start should be less than End.
		assert.Less(t, c.Start, c.End, "chunk %d: Start should be less than End", i)
	}

	// Adjacent chunks should have overlapping byte ranges.
	for i := 1; i < len(chunks); i++ {
		assert.Less(t, chunks[i].Start, chunks[i-1].End,
			"chunk %d should start before chunk %d ends (overlap)", i, i-1)
	}
}

// TestSplit_OverlapAtBoundary verifies that the splitter does not get stuck in
// an infinite loop when overlap positions the cursor exactly at a boundary
// marker (e.g. \n\n). This is an edge case regression test.
func TestSplit_OverlapAtBoundary(t *testing.T) {
	// Construct content where a paragraph break sits at exactly the overlap
	// position. part1 fills most of maxBytes, the \n\n is right where overlap
	// would place the next start.
	part1 := strings.Repeat("a", 40)
	boundary := "\n\n"
	part2 := strings.Repeat("b", 40)
	content := part1 + boundary + part2

	// maxBytes=45 means the split happens at the \n\n (index 40 in region).
	// overlapBytes=42 means next pos = 40 - 42 = -2, clamped to splitAt=40,
	// which is right at the \n\n.
	chunks := Split(content, 45, 42)

	require.Greater(t, len(chunks), 1, "should produce multiple chunks without infinite loop")
	// All content should be covered.
	assert.Equal(t, 0, chunks[0].Start)
	assert.Equal(t, len(content), chunks[len(chunks)-1].End)
	for _, c := range chunks {
		assert.NotEmpty(t, c.Text, "no chunk should be empty")
	}
}

// TestSplit_UTF8Safety verifies that the splitter does not split mid-rune in
// multibyte UTF-8 sequences (NFR-1).
func TestSplit_UTF8Safety(t *testing.T) {
	// Use multibyte characters. Each emoji is 4 bytes.
	// Build a string: 10 ASCII chars + a 4-byte emoji repeated.
	// This ensures a hard cut could land mid-rune if not careful.
	emojis := "Hello" + strings.Repeat("\U0001F600", 20) // 5 + 80 = 85 bytes
	content := emojis

	chunks := Split(content, 30, 5)

	require.Greater(t, len(chunks), 1, "should produce multiple chunks")

	for i, c := range chunks {
		assert.True(t, utf8.ValidString(c.Text),
			"chunk %d should be valid UTF-8, got: %q", i, c.Text)
		// Verify offsets also align to rune boundaries.
		assert.True(t, utf8.RuneStart(content[c.Start]),
			"chunk %d Start offset %d should be at a rune boundary", i, c.Start)
		if c.End < len(content) {
			assert.True(t, utf8.RuneStart(content[c.End]),
				"chunk %d End offset %d should be at a rune boundary", i, c.End)
		}
	}
}
