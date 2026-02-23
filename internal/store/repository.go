package store

import "context"

// FileRepository defines persistence operations for file metadata.
type FileRepository interface {
	Insert(ctx context.Context, meta FileMetadata) error
	Update(ctx context.Context, meta FileMetadata) error
	Get(ctx context.Context, filepath string) (FileMetadata, error)
	Delete(ctx context.Context, filepath string) error
	Search(ctx context.Context, filters SearchFilters) ([]FileMetadata, error)
	ListTags(ctx context.Context, sort string) ([]TagCount, error)
	AllFilepaths(ctx context.Context) ([]string, error)
	Count(ctx context.Context) (int, error)
}

// EmbeddingRepository defines persistence operations for vector embeddings.
type EmbeddingRepository interface {
	Upsert(ctx context.Context, filepath string, vector []float32, modelID string) error
	Get(ctx context.Context, filepath string) (Embedding, error)
	Delete(ctx context.Context, filepath string) error
	GetAll(ctx context.Context) ([]Embedding, error)
}
