package search

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/awood45/grimoire-cli/internal/embedding"
	embtest "github.com/awood45/grimoire-cli/internal/embedding/testing"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
	storetest "github.com/awood45/grimoire-cli/internal/store/testing"
)

// TestSearch_delegatesToRepo verifies that Search passes filters through to FileRepository (FR-3.3.1).
func TestSearch_delegatesToRepo(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()

	now := time.Now()
	fileRepo.Data["notes/meeting.md"] = store.FileMetadata{
		Filepath:    "notes/meeting.md",
		SourceAgent: "claude",
		Tags:        []string{"meeting", "project"},
		Summary:     "Weekly standup notes",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	fileRepo.Data["notes/other.md"] = store.FileMetadata{
		Filepath:    "notes/other.md",
		SourceAgent: "gpt",
		Tags:        []string{"other"},
		Summary:     "Other notes",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	filters := store.SearchFilters{
		SourceAgent: "claude",
	}

	results, err := eng.Search(ctx, filters)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "notes/meeting.md", results[0].Filepath)
}

// TestSearch_delegatesToRepo_error verifies that Search propagates errors from FileRepository (FR-3.3.1).
func TestSearch_delegatesToRepo_error(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()

	fileRepo.SearchErr = sberrors.New(sberrors.ErrCodeDatabaseError, "db error")

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	_, err := eng.Search(ctx, store.SearchFilters{})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestSimilar_byText verifies similarity search by text generates embedding and ranks results (FR-3.3.2).
func TestSimilar_byText(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()

	// Set the query embedding that will be generated.
	queryVec := []float32{1.0, 0.0, 0.0}
	provider.FixedVector = queryVec

	now := time.Now()

	// Add two files with embeddings. File A is more similar to query than file B.
	fileRepo.Data["notes/a.md"] = store.FileMetadata{
		Filepath:    "notes/a.md",
		SourceAgent: "claude",
		Tags:        []string{"topic-a"},
		Summary:     "File A",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	fileRepo.Data["notes/b.md"] = store.FileMetadata{
		Filepath:    "notes/b.md",
		SourceAgent: "claude",
		Tags:        []string{"topic-b"},
		Summary:     "File B",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Vector close to query [1,0,0].
	embRepo.Data["notes/a.md"] = store.Embedding{
		Filepath: "notes/a.md",
		Vector:   []float32{0.9, 0.1, 0.0},
		ModelID:  "fake-model",
	}
	// Vector far from query [1,0,0].
	embRepo.Data["notes/b.md"] = store.Embedding{
		Filepath: "notes/b.md",
		Vector:   []float32{0.0, 0.0, 1.0},
		ModelID:  "fake-model",
	}

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	results, err := eng.Similar(ctx, SimilarInput{
		Text:  "find similar",
		Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)

	// First result should be notes/a.md (higher similarity).
	assert.Equal(t, "notes/a.md", results[0].Filepath)
	assert.Greater(t, results[0].Score, results[1].Score)

	// Verify the embedding was generated for the input text.
	assert.Equal(t, []string{"find similar"}, provider.GenerateCalls)
}

// TestSimilar_byFile verifies similarity search using an existing file embedding (FR-3.3.2).
func TestSimilar_byFile(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()

	now := time.Now()

	// The query file and two candidate files.
	fileRepo.Data["notes/query.md"] = store.FileMetadata{
		Filepath:    "notes/query.md",
		SourceAgent: "claude",
		Summary:     "Query file",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	fileRepo.Data["notes/a.md"] = store.FileMetadata{
		Filepath:    "notes/a.md",
		SourceAgent: "claude",
		Summary:     "File A",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	fileRepo.Data["notes/b.md"] = store.FileMetadata{
		Filepath:    "notes/b.md",
		SourceAgent: "claude",
		Summary:     "File B",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	embRepo.Data["notes/query.md"] = store.Embedding{
		Filepath: "notes/query.md",
		Vector:   []float32{1.0, 0.0, 0.0},
		ModelID:  "fake-model",
	}
	embRepo.Data["notes/a.md"] = store.Embedding{
		Filepath: "notes/a.md",
		Vector:   []float32{0.9, 0.1, 0.0},
		ModelID:  "fake-model",
	}
	embRepo.Data["notes/b.md"] = store.Embedding{
		Filepath: "notes/b.md",
		Vector:   []float32{0.0, 0.0, 1.0},
		ModelID:  "fake-model",
	}

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	results, err := eng.Similar(ctx, SimilarInput{
		FilePath: "notes/query.md",
		Limit:    10,
	})
	require.NoError(t, err)
	// Should not include the query file itself in results.
	require.Len(t, results, 2)
	assert.Equal(t, "notes/a.md", results[0].Filepath)
	assert.Greater(t, results[0].Score, results[1].Score)

	// Should not have called GenerateEmbedding since we used a file.
	assert.Empty(t, provider.GenerateCalls)
}

// TestSimilar_noEmbeddingProvider verifies NO_EMBEDDING_PROVIDER error when using NoopProvider with text (FR-3.3.2).
func TestSimilar_noEmbeddingProvider(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	noopProvider := &embedding.NoopProvider{}

	eng := NewEngine(fileRepo, embRepo, noopProvider)
	ctx := context.Background()

	_, err := eng.Similar(ctx, SimilarInput{
		Text:  "find similar",
		Limit: 10,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeNoEmbeddingProvider))
}

// TestSimilar_limit verifies that the limit parameter caps results (FR-3.3.2).
func TestSimilar_limit(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()
	provider.FixedVector = []float32{1.0, 0.0, 0.0}

	now := time.Now()

	// Add 5 files with embeddings.
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		fp := "notes/" + name + ".md"
		fileRepo.Data[fp] = store.FileMetadata{
			Filepath:    fp,
			SourceAgent: "claude",
			Summary:     "File " + name,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		embRepo.Data[fp] = store.Embedding{
			Filepath: fp,
			Vector:   []float32{0.5, 0.5, 0.0},
			ModelID:  "fake-model",
		}
	}

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	results, err := eng.Similar(ctx, SimilarInput{
		Text:  "query",
		Limit: 3,
	})
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

// TestListTags_delegatesToRepo verifies that ListTags passes sort through to FileRepository (FR-3.3.3).
func TestListTags_delegatesToRepo(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()

	now := time.Now()
	fileRepo.Data["notes/a.md"] = store.FileMetadata{
		Filepath:  "notes/a.md",
		Tags:      []string{"alpha", "beta"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	fileRepo.Data["notes/b.md"] = store.FileMetadata{
		Filepath:  "notes/b.md",
		Tags:      []string{"beta", "gamma"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	tags, err := eng.ListTags(ctx, "name")
	require.NoError(t, err)
	require.Len(t, tags, 3)
	assert.Equal(t, "alpha", tags[0].Name)
	assert.Equal(t, 1, tags[0].Count)
	assert.Equal(t, "beta", tags[1].Name)
	assert.Equal(t, 2, tags[1].Count)
	assert.Equal(t, "gamma", tags[2].Name)
	assert.Equal(t, 1, tags[2].Count)
}

// TestListTags_delegatesToRepo_error verifies that ListTags propagates errors from FileRepository (FR-3.3.3).
func TestListTags_delegatesToRepo_error(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()

	fileRepo.ListTagsErr = sberrors.New(sberrors.ErrCodeDatabaseError, "db error")

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	_, err := eng.ListTags(ctx, "name")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestSimilar_embeddingGenerationError verifies error propagation from embedding provider (FR-3.3.2).
func TestSimilar_embeddingGenerationError(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()
	provider.GenerateErr = sberrors.New(sberrors.ErrCodeEmbeddingError, "embedding failed")

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	_, err := eng.Similar(ctx, SimilarInput{
		Text:  "query",
		Limit: 10,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeEmbeddingError))
}

// TestSimilar_fileEmbeddingNotFound verifies error when file embedding does not exist (FR-3.3.2).
func TestSimilar_fileEmbeddingNotFound(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	_, err := eng.Similar(ctx, SimilarInput{
		FilePath: "notes/nonexistent.md",
		Limit:    10,
	})
	require.Error(t, err)
}

// TestSimilar_getAllError verifies error propagation from GetAll (FR-3.3.2).
func TestSimilar_getAllError(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()
	provider.FixedVector = []float32{1.0, 0.0, 0.0}

	embRepo.GetAllErr = sberrors.New(sberrors.ErrCodeDatabaseError, "getall failed")

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	_, err := eng.Similar(ctx, SimilarInput{
		Text:  "query",
		Limit: 10,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestSimilar_noEmbeddings verifies empty result when no embeddings exist (FR-3.3.2).
func TestSimilar_noEmbeddings(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()
	provider.FixedVector = []float32{1.0, 0.0, 0.0}

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	results, err := eng.Similar(ctx, SimilarInput{
		Text:  "query",
		Limit: 10,
	})
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestSimilar_metadataFetchError verifies that errors fetching metadata for a result are propagated (FR-3.3.2).
func TestSimilar_metadataFetchError(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()
	provider.FixedVector = []float32{1.0, 0.0, 0.0}

	// Add embedding but no metadata.
	embRepo.Data["notes/a.md"] = store.Embedding{
		Filepath: "notes/a.md",
		Vector:   []float32{0.9, 0.1, 0.0},
		ModelID:  "fake-model",
	}

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	_, err := eng.Similar(ctx, SimilarInput{
		Text:  "query",
		Limit: 10,
	})
	require.Error(t, err)
}

// TestSimilar_emptyInput verifies that empty SimilarInput returns INVALID_INPUT (FR-3.3.2).
func TestSimilar_emptyInput(t *testing.T) {
	fileRepo := storetest.NewFakeFileRepository()
	embRepo := storetest.NewFakeEmbeddingRepository()
	provider := embtest.NewFakeProvider()

	eng := NewEngine(fileRepo, embRepo, provider)
	ctx := context.Background()

	_, err := eng.Similar(ctx, SimilarInput{})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
}
