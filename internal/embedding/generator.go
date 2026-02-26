package embedding

import (
	"context"
	"fmt"
	"os"

	"github.com/awood45/grimoire-cli/internal/chunking"
	"github.com/awood45/grimoire-cli/internal/store"
)

// Generator orchestrates chunking, embedding, and storage for file content.
// It replaces duplicated embedding logic that previously lived in both
// the metadata Manager and the maintenance Service.
type Generator struct {
	embedder      Provider
	embRepo       store.EmbeddingRepository
	docPrefix     string
	maxChunkBytes int
	overlapBytes  int
}

// NewGenerator creates a Generator with the given configuration.
func NewGenerator(
	embedder Provider,
	embRepo store.EmbeddingRepository,
	docPrefix string,
	maxChunkBytes int,
	overlapBytes int,
) *Generator {
	return &Generator{
		embedder:      embedder,
		embRepo:       embRepo,
		docPrefix:     docPrefix,
		maxChunkBytes: maxChunkBytes,
		overlapBytes:  overlapBytes,
	}
}

// GenerateForFile reads a file, chunks it, embeds each chunk, and stores
// the results. All existing embeddings for the file are deleted first.
// Returns nil immediately if the embedder is a NoopProvider (ModelID == "none").
func (g *Generator) GenerateForFile(ctx context.Context, filepath, absPath string) error {
	if g.IsNoop() {
		return nil
	}

	chunks, err := g.readAndChunk(absPath)
	if err != nil {
		return err
	}

	if err := g.embRepo.DeleteForFile(ctx, filepath); err != nil {
		return fmt.Errorf("deleting old embeddings for %s: %w", filepath, err)
	}

	return g.embedAndStore(ctx, filepath, chunks)
}

// GenerateForFileWithSummary does the same as GenerateForFile, and additionally
// embeds the provided summaryText as a summary chunk (chunk_index = -1,
// is_summary = true) -- but only if the file produces more than one chunk (FR-8).
func (g *Generator) GenerateForFileWithSummary(ctx context.Context, filepath, absPath, summaryText string) error {
	if g.IsNoop() {
		return nil
	}

	chunks, err := g.readAndChunk(absPath)
	if err != nil {
		return err
	}

	if err := g.embRepo.DeleteForFile(ctx, filepath); err != nil {
		return fmt.Errorf("deleting old embeddings for %s: %w", filepath, err)
	}

	if err := g.embedAndStore(ctx, filepath, chunks); err != nil {
		return err
	}

	// Only store a summary embedding for multi-chunk files with non-empty summary text.
	if len(chunks) > 1 && summaryText != "" {
		prefixed := g.docPrefix + summaryText
		vector, embErr := g.embedder.GenerateEmbedding(ctx, prefixed)
		if embErr != nil {
			return fmt.Errorf("generating summary embedding for %s: %w", filepath, embErr)
		}

		emb := store.Embedding{
			Filepath:   filepath,
			ChunkIndex: -1,
			Vector:     vector,
			ModelID:    g.embedder.ModelID(),
			IsSummary:  true,
		}
		if err := g.embRepo.Upsert(ctx, emb); err != nil {
			return fmt.Errorf("storing summary embedding for %s: %w", filepath, err)
		}
	}

	return nil
}

// IsNoop returns true if the underlying embedder is the NoopProvider.
func (g *Generator) IsNoop() bool {
	return g.embedder.ModelID() == "none"
}

// ModelID delegates to the underlying embedder.
func (g *Generator) ModelID() string {
	return g.embedder.ModelID()
}

// readAndChunk reads the file at absPath and splits it into chunks.
func (g *Generator) readAndChunk(absPath string) ([]chunking.Chunk, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", absPath, err)
	}

	chunks := chunking.Split(string(data), g.maxChunkBytes, g.overlapBytes)
	return chunks, nil
}

// embedAndStore generates an embedding for each chunk and stores it.
func (g *Generator) embedAndStore(ctx context.Context, filepath string, chunks []chunking.Chunk) error {
	for _, chunk := range chunks {
		prefixed := g.docPrefix + chunk.Text
		vector, err := g.embedder.GenerateEmbedding(ctx, prefixed)
		if err != nil {
			return fmt.Errorf("generating embedding for %s chunk %d: %w", filepath, chunk.Index, err)
		}

		emb := store.Embedding{
			Filepath:   filepath,
			ChunkIndex: chunk.Index,
			Vector:     vector,
			ModelID:    g.embedder.ModelID(),
			ChunkStart: chunk.Start,
			ChunkEnd:   chunk.End,
		}
		if err := g.embRepo.Upsert(ctx, emb); err != nil {
			return fmt.Errorf("storing embedding for %s chunk %d: %w", filepath, chunk.Index, err)
		}
	}

	return nil
}
