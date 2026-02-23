package search

import (
	"context"
	"sort"

	"github.com/awood45/grimoire-cli/internal/embedding"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
)

// Engine handles metadata queries and similarity search.
type Engine struct {
	fileRepo store.FileRepository
	embRepo  store.EmbeddingRepository
	embedder embedding.Provider
}

// NewEngine creates a new search Engine with the given dependencies.
func NewEngine(
	fileRepo store.FileRepository,
	embRepo store.EmbeddingRepository,
	embedder embedding.Provider,
) *Engine {
	return &Engine{
		fileRepo: fileRepo,
		embRepo:  embRepo,
		embedder: embedder,
	}
}

// Search delegates metadata filtering to the file repository.
func (e *Engine) Search(ctx context.Context, filters store.SearchFilters) ([]store.FileMetadata, error) { //nolint:gocritic // hugeParam: interface requires value type.
	return e.fileRepo.Search(ctx, filters)
}

// Similar finds files similar to a query vector, either from text or an existing file.
func (e *Engine) Similar(ctx context.Context, input SimilarInput) ([]store.SimilarityResult, error) {
	queryVector, err := e.resolveQueryVector(ctx, &input)
	if err != nil {
		return nil, err
	}

	allEmbeddings, err := e.embRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	scored := e.computeScores(queryVector, allEmbeddings, input.FilePath)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if input.Limit > 0 && len(scored) > input.Limit {
		scored = scored[:input.Limit]
	}

	return e.fetchMetadata(ctx, scored)
}

// ListTags delegates tag listing to the file repository.
func (e *Engine) ListTags(ctx context.Context, sortOrder string) ([]store.TagCount, error) {
	return e.fileRepo.ListTags(ctx, sortOrder)
}

// scoredEntry holds a filepath and its similarity score before metadata enrichment.
type scoredEntry struct {
	filepath string
	score    float64
}

// resolveQueryVector obtains the query embedding vector from either text or a file.
func (e *Engine) resolveQueryVector(ctx context.Context, input *SimilarInput) ([]float32, error) {
	if input.Text == "" && input.FilePath == "" {
		return nil, sberrors.New(sberrors.ErrCodeInvalidInput, "either Text or FilePath must be provided")
	}

	if input.Text != "" {
		return e.resolveFromText(ctx, input.Text)
	}

	emb, err := e.embRepo.Get(ctx, input.FilePath)
	if err != nil {
		return nil, err
	}
	return emb.Vector, nil
}

// resolveFromText generates an embedding for the given text.
func (e *Engine) resolveFromText(ctx context.Context, text string) ([]float32, error) {
	vec, err := e.embedder.GenerateEmbedding(ctx, text)
	if err != nil {
		return nil, err
	}

	if vec == nil {
		return nil, sberrors.New(
			sberrors.ErrCodeNoEmbeddingProvider,
			"no embedding provider configured; cannot perform similarity search by text",
		)
	}

	return vec, nil
}

// computeScores calculates cosine similarity for all embeddings, excluding the query file if present.
func (e *Engine) computeScores(queryVector []float32, allEmbeddings []store.Embedding, excludePath string) []scoredEntry {
	scored := make([]scoredEntry, 0, len(allEmbeddings))
	for i := range allEmbeddings {
		emb := &allEmbeddings[i]
		if emb.Filepath == excludePath {
			continue
		}
		score := CosineSimilarity(queryVector, emb.Vector)
		scored = append(scored, scoredEntry{
			filepath: emb.Filepath,
			score:    score,
		})
	}
	return scored
}

// fetchMetadata enriches scored entries with file metadata.
func (e *Engine) fetchMetadata(ctx context.Context, scored []scoredEntry) ([]store.SimilarityResult, error) {
	results := make([]store.SimilarityResult, 0, len(scored))
	for i := range scored {
		meta, err := e.fileRepo.Get(ctx, scored[i].filepath)
		if err != nil {
			return nil, sberrors.Wrapf(err, sberrors.ErrCodeInternalError,
				"failed to fetch metadata for %s", scored[i].filepath)
		}
		results = append(results, store.SimilarityResult{
			Filepath: scored[i].filepath,
			Score:    scored[i].score,
			Metadata: meta,
		})
	}
	return results, nil
}
