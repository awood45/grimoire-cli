package embedding_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awood45/grimoire-cli/internal/embedding"
	embtesting "github.com/awood45/grimoire-cli/internal/embedding/testing"
	"github.com/awood45/grimoire-cli/internal/store"
	storetesting "github.com/awood45/grimoire-cli/internal/store/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDocPrefix = "search_document: "

// writeTestFile creates a temporary file with the given content and returns the path.
func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	err := os.WriteFile(p, []byte(content), 0o644)
	require.NoError(t, err)
	return p
}

// TestGenerator_NoopSkips verifies that a NoopProvider returns nil without
// reading the file (FR-11: noop embedder should be a silent no-op).
func TestGenerator_NoopSkips(t *testing.T) {
	noop := &embedding.NoopProvider{}
	embRepo := storetesting.NewFakeEmbeddingRepository()

	gen := embedding.NewGenerator(noop, embRepo, testDocPrefix, 1024, 128)

	// Use a non-existent file path. If the generator tries to read the file,
	// it will fail with an error.
	err := gen.GenerateForFile(context.Background(), "some/file.md", "/nonexistent/path/file.md")
	require.NoError(t, err)

	// No embeddings should be stored.
	assert.Empty(t, embRepo.Data)
	assert.True(t, gen.IsNoop())
}

// TestGenerator_SingleChunk verifies that a small file produces exactly one
// chunk stored with the correct prefix (FR-3, FR-4).
func TestGenerator_SingleChunk(t *testing.T) {
	provider := embtesting.NewFakeProvider()
	embRepo := storetesting.NewFakeEmbeddingRepository()

	gen := embedding.NewGenerator(provider, embRepo, testDocPrefix, 4096, 128)

	dir := t.TempDir()
	content := "Hello, this is a small file."
	absPath := writeTestFile(t, dir, "small.md", content)

	err := gen.GenerateForFile(context.Background(), "small.md", absPath)
	require.NoError(t, err)

	// Exactly one embedding should be stored.
	chunks := embRepo.Data["small.md"]
	require.Len(t, chunks, 1)
	assert.Equal(t, 0, chunks[0].ChunkIndex)
	assert.Equal(t, provider.FixedModelID, chunks[0].ModelID)
	assert.Equal(t, provider.FixedVector, chunks[0].Vector)
	assert.Equal(t, 0, chunks[0].ChunkStart)
	assert.Equal(t, len(content), chunks[0].ChunkEnd)
	assert.False(t, chunks[0].IsSummary)
}

// TestGenerator_MultipleChunks verifies that a large file produces multiple
// chunks, each embedded and stored independently (FR-1, FR-3).
func TestGenerator_MultipleChunks(t *testing.T) {
	provider := embtesting.NewFakeProvider()
	embRepo := storetesting.NewFakeEmbeddingRepository()

	// Use a very small maxChunkBytes to force multiple chunks.
	gen := embedding.NewGenerator(provider, embRepo, testDocPrefix, 50, 10)

	dir := t.TempDir()
	// Create content that will require multiple chunks.
	content := strings.Repeat("Line of text here.\n", 10) // 190 bytes total
	absPath := writeTestFile(t, dir, "large.md", content)

	err := gen.GenerateForFile(context.Background(), "large.md", absPath)
	require.NoError(t, err)

	chunks := embRepo.Data["large.md"]
	require.Greater(t, len(chunks), 1, "expected multiple chunks for large file")

	// Verify chunk indices are sequential.
	for i, chunk := range chunks {
		assert.Equal(t, i, chunk.ChunkIndex)
		assert.Equal(t, "large.md", chunk.Filepath)
		assert.Equal(t, provider.FixedModelID, chunk.ModelID)
	}
}

// TestGenerator_DeletesOldChunks verifies that DeleteForFile is called before
// inserting new chunks, so stale data is cleared (FR-3).
func TestGenerator_DeletesOldChunks(t *testing.T) {
	provider := embtesting.NewFakeProvider()
	embRepo := storetesting.NewFakeEmbeddingRepository()

	// Pre-populate with old embeddings.
	embRepo.Data["file.md"] = []store.Embedding{
		{Filepath: "file.md", ChunkIndex: 0, Vector: []float32{0.9, 0.8}},
		{Filepath: "file.md", ChunkIndex: 1, Vector: []float32{0.7, 0.6}},
	}

	gen := embedding.NewGenerator(provider, embRepo, testDocPrefix, 4096, 128)

	dir := t.TempDir()
	absPath := writeTestFile(t, dir, "file.md", "new content")

	err := gen.GenerateForFile(context.Background(), "file.md", absPath)
	require.NoError(t, err)

	// DeleteForFile should have been called.
	assert.Contains(t, embRepo.DeleteCalls, "file.md")

	// The old embeddings should be replaced with a single new chunk.
	chunks := embRepo.Data["file.md"]
	require.Len(t, chunks, 1)
	assert.Equal(t, provider.FixedVector, chunks[0].Vector)
}

// TestGenerator_DocumentPrefix verifies that the document prefix is prepended
// to each chunk text sent to the embedder (FR-4).
func TestGenerator_DocumentPrefix(t *testing.T) {
	provider := embtesting.NewFakeProvider()
	embRepo := storetesting.NewFakeEmbeddingRepository()

	gen := embedding.NewGenerator(provider, embRepo, testDocPrefix, 4096, 128)

	dir := t.TempDir()
	content := "Some document content."
	absPath := writeTestFile(t, dir, "doc.md", content)

	err := gen.GenerateForFile(context.Background(), "doc.md", absPath)
	require.NoError(t, err)

	// The embedder should have been called with the prefix prepended.
	require.Len(t, provider.GenerateCalls, 1)
	assert.Equal(t, testDocPrefix+content, provider.GenerateCalls[0])
}

// TestGenerator_WithSummary_MultiChunk verifies that multi-chunk files store
// a summary embedding at chunk_index=-1 with is_summary=true (FR-7).
func TestGenerator_WithSummary_MultiChunk(t *testing.T) {
	provider := embtesting.NewFakeProvider()
	embRepo := storetesting.NewFakeEmbeddingRepository()

	// Small maxChunkBytes to force multiple chunks.
	gen := embedding.NewGenerator(provider, embRepo, testDocPrefix, 50, 10)

	dir := t.TempDir()
	content := strings.Repeat("Line of text here.\n", 10) // 190 bytes
	absPath := writeTestFile(t, dir, "multi.md", content)

	summaryText := "This is a summary of the document."
	err := gen.GenerateForFileWithSummary(context.Background(), "multi.md", absPath, summaryText)
	require.NoError(t, err)

	chunks := embRepo.Data["multi.md"]
	require.Greater(t, len(chunks), 1, "expected multiple chunks + summary")

	// Find the summary chunk.
	var summaryFound bool
	for _, chunk := range chunks {
		if chunk.ChunkIndex == -1 {
			summaryFound = true
			assert.True(t, chunk.IsSummary)
			assert.Equal(t, "multi.md", chunk.Filepath)
			assert.Equal(t, provider.FixedModelID, chunk.ModelID)
		}
	}
	assert.True(t, summaryFound, "expected summary chunk at index -1")

	// Verify the summary text was embedded with the prefix.
	assert.Contains(t, provider.GenerateCalls, testDocPrefix+summaryText)
}

// TestGenerator_WithSummary_SingleChunk verifies that single-chunk files skip
// the summary embedding even when summaryText is provided (FR-8).
func TestGenerator_WithSummary_SingleChunk(t *testing.T) {
	provider := embtesting.NewFakeProvider()
	embRepo := storetesting.NewFakeEmbeddingRepository()

	gen := embedding.NewGenerator(provider, embRepo, testDocPrefix, 4096, 128)

	dir := t.TempDir()
	content := "Short document that fits in one chunk."
	absPath := writeTestFile(t, dir, "short.md", content)

	summaryText := "This is a summary."
	err := gen.GenerateForFileWithSummary(context.Background(), "short.md", absPath, summaryText)
	require.NoError(t, err)

	// Only one chunk should be stored, no summary.
	chunks := embRepo.Data["short.md"]
	require.Len(t, chunks, 1)
	assert.Equal(t, 0, chunks[0].ChunkIndex)
	assert.False(t, chunks[0].IsSummary)

	// The summary text should NOT have been embedded.
	for _, call := range provider.GenerateCalls {
		assert.NotContains(t, call, summaryText)
	}
}

// TestGenerator_EmbeddingError verifies that provider errors are propagated
// to the caller (FR-11).
func TestGenerator_EmbeddingError(t *testing.T) {
	provider := embtesting.NewFakeProvider()
	provider.GenerateErr = errors.New("embedding service unavailable")
	embRepo := storetesting.NewFakeEmbeddingRepository()

	gen := embedding.NewGenerator(provider, embRepo, testDocPrefix, 4096, 128)

	dir := t.TempDir()
	absPath := writeTestFile(t, dir, "err.md", "Some content.")

	err := gen.GenerateForFile(context.Background(), "err.md", absPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embedding service unavailable")
}

// TestGenerator_FileReadError verifies that a missing file returns an error
// (FR-11).
func TestGenerator_FileReadError(t *testing.T) {
	provider := embtesting.NewFakeProvider()
	embRepo := storetesting.NewFakeEmbeddingRepository()

	gen := embedding.NewGenerator(provider, embRepo, testDocPrefix, 4096, 128)

	err := gen.GenerateForFile(context.Background(), "missing.md", "/nonexistent/path/missing.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading file")
}

// TestGenerator_IsNoop verifies IsNoop returns true for NoopProvider and false
// for real providers.
func TestGenerator_IsNoop(t *testing.T) {
	noopGen := embedding.NewGenerator(&embedding.NoopProvider{}, storetesting.NewFakeEmbeddingRepository(), "", 1024, 128)
	assert.True(t, noopGen.IsNoop())

	fakeGen := embedding.NewGenerator(embtesting.NewFakeProvider(), storetesting.NewFakeEmbeddingRepository(), "", 1024, 128)
	assert.False(t, fakeGen.IsNoop())
}

// TestGenerator_ModelID verifies ModelID delegates to the underlying embedder.
func TestGenerator_ModelID(t *testing.T) {
	provider := embtesting.NewFakeProvider()
	provider.FixedModelID = "test-model-v1"

	gen := embedding.NewGenerator(provider, storetesting.NewFakeEmbeddingRepository(), "", 1024, 128)
	assert.Equal(t, "test-model-v1", gen.ModelID())
}
