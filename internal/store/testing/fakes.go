// Package testing provides hand-written fake implementations of store interfaces for use in tests.
package testing

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
)

// Compile-time interface assertions.
var (
	_ store.FileRepository      = (*FakeFileRepository)(nil)
	_ store.EmbeddingRepository = (*FakeEmbeddingRepository)(nil)
)

// FakeFileRepository is an in-memory implementation of store.FileRepository.
// Error injection fields allow tests to simulate failures.
type FakeFileRepository struct {
	mu sync.Mutex

	// Data is the in-memory map of filepath to FileMetadata.
	Data map[string]store.FileMetadata

	// Error injection fields. When set, the corresponding method returns
	// this error instead of succeeding.
	InsertErr   error
	UpdateErr   error
	GetErr      error
	DeleteErr   error
	SearchErr   error
	ListTagsErr error
	AllPathsErr error
	CountErr    error

	// InsertCalls records the filepaths passed to Insert.
	InsertCalls []string
	// UpdateCalls records the filepaths passed to Update.
	UpdateCalls []string
	// DeleteCalls records the filepaths passed to Delete.
	DeleteCalls []string
}

// NewFakeFileRepository creates a FakeFileRepository with an initialized map.
func NewFakeFileRepository() *FakeFileRepository {
	return &FakeFileRepository{
		Data: make(map[string]store.FileMetadata),
	}
}

// Insert stores file metadata in memory.
func (f *FakeFileRepository) Insert(_ context.Context, meta store.FileMetadata) error { //nolint:gocritic // hugeParam: interface requires value type.
	f.mu.Lock()
	defer f.mu.Unlock()

	f.InsertCalls = append(f.InsertCalls, meta.Filepath)

	if f.InsertErr != nil {
		return f.InsertErr
	}

	if _, exists := f.Data[meta.Filepath]; exists {
		return sberrors.Newf(sberrors.ErrCodeMetadataExists, "metadata already exists: %s", meta.Filepath)
	}

	f.Data[meta.Filepath] = meta
	return nil
}

// Update replaces file metadata in memory.
func (f *FakeFileRepository) Update(_ context.Context, meta store.FileMetadata) error { //nolint:gocritic // hugeParam: interface requires value type.
	f.mu.Lock()
	defer f.mu.Unlock()

	f.UpdateCalls = append(f.UpdateCalls, meta.Filepath)

	if f.UpdateErr != nil {
		return f.UpdateErr
	}

	if _, exists := f.Data[meta.Filepath]; !exists {
		return sberrors.Newf(sberrors.ErrCodeMetadataNotFound, "metadata not found: %s", meta.Filepath)
	}

	f.Data[meta.Filepath] = meta
	return nil
}

// Get retrieves file metadata by filepath.
func (f *FakeFileRepository) Get(_ context.Context, filepath string) (store.FileMetadata, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.GetErr != nil {
		return store.FileMetadata{}, f.GetErr
	}

	meta, exists := f.Data[filepath]
	if !exists {
		return store.FileMetadata{}, sberrors.Newf(sberrors.ErrCodeMetadataNotFound, "metadata not found: %s", filepath)
	}

	return meta, nil
}

// Delete removes file metadata by filepath.
func (f *FakeFileRepository) Delete(_ context.Context, filepath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.DeleteCalls = append(f.DeleteCalls, filepath)

	if f.DeleteErr != nil {
		return f.DeleteErr
	}

	delete(f.Data, filepath)
	return nil
}

// Search returns file metadata matching the given filters.
func (f *FakeFileRepository) Search(_ context.Context, filters store.SearchFilters) ([]store.FileMetadata, error) { //nolint:gocritic // hugeParam: interface requires value type.
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.SearchErr != nil {
		return nil, f.SearchErr
	}

	var results []store.FileMetadata
	for _, meta := range f.Data {
		if matchesFilters(&meta, &filters) {
			results = append(results, meta)
		}
	}

	// Sort by updated_at descending by default.
	sort.Slice(results, func(i, j int) bool {
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})

	if filters.Limit > 0 && len(results) > filters.Limit {
		results = results[:filters.Limit]
	}

	return results, nil
}

// ListTags returns all unique tags with their counts.
func (f *FakeFileRepository) ListTags(_ context.Context, sortOrder string) ([]store.TagCount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ListTagsErr != nil {
		return nil, f.ListTagsErr
	}

	counts := make(map[string]int)
	for _, meta := range f.Data {
		for _, tag := range meta.Tags {
			counts[tag]++
		}
	}

	tags := make([]store.TagCount, 0, len(counts))
	for name, count := range counts {
		tags = append(tags, store.TagCount{Name: name, Count: count})
	}

	if sortOrder == "count" {
		sort.Slice(tags, func(i, j int) bool {
			if tags[i].Count == tags[j].Count {
				return tags[i].Name < tags[j].Name
			}
			return tags[i].Count > tags[j].Count
		})
	} else {
		sort.Slice(tags, func(i, j int) bool {
			return tags[i].Name < tags[j].Name
		})
	}

	return tags, nil
}

// AllFilepaths returns all filepaths in the repository.
func (f *FakeFileRepository) AllFilepaths(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.AllPathsErr != nil {
		return nil, f.AllPathsErr
	}

	paths := make([]string, 0, len(f.Data))
	for path := range f.Data {
		paths = append(paths, path)
	}

	sort.Strings(paths)
	return paths, nil
}

// Count returns the number of entries in the repository.
func (f *FakeFileRepository) Count(_ context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.CountErr != nil {
		return 0, f.CountErr
	}

	return len(f.Data), nil
}

// matchesFilters checks if metadata matches the given search filters.
func matchesFilters(meta *store.FileMetadata, filters *store.SearchFilters) bool {
	if !matchesScalarFilters(meta, filters) {
		return false
	}

	if !matchesTimeFilters(meta, filters) {
		return false
	}

	return matchesTagFilters(meta, filters)
}

// matchesScalarFilters checks source agent and summary filters.
func matchesScalarFilters(meta *store.FileMetadata, filters *store.SearchFilters) bool {
	if filters.SourceAgent != "" && meta.SourceAgent != filters.SourceAgent {
		return false
	}

	if filters.SummaryContains != "" && !strings.Contains(
		strings.ToLower(meta.Summary),
		strings.ToLower(filters.SummaryContains),
	) {
		return false
	}

	return true
}

// matchesTimeFilters checks after and before time filters.
func matchesTimeFilters(meta *store.FileMetadata, filters *store.SearchFilters) bool {
	if filters.After != nil && !meta.UpdatedAt.After(*filters.After) {
		return false
	}

	if filters.Before != nil && !meta.UpdatedAt.Before(*filters.Before) {
		return false
	}

	return true
}

// matchesTagFilters checks both all-tags (AND) and any-tags (OR) filters.
func matchesTagFilters(meta *store.FileMetadata, filters *store.SearchFilters) bool {
	if len(filters.Tags) == 0 && len(filters.AnyTags) == 0 {
		return true
	}

	tagSet := make(map[string]bool, len(meta.Tags))
	for _, t := range meta.Tags {
		tagSet[t] = true
	}

	// All tags (AND): every required tag must be present.
	for _, required := range filters.Tags {
		if !tagSet[required] {
			return false
		}
	}

	// Any tags (OR): at least one tag must be present.
	if len(filters.AnyTags) > 0 {
		for _, anyTag := range filters.AnyTags {
			if tagSet[anyTag] {
				return true
			}
		}
		return false
	}

	return true
}

// FakeEmbeddingRepository is an in-memory implementation of store.EmbeddingRepository.
// Error injection fields allow tests to simulate failures.
type FakeEmbeddingRepository struct {
	mu sync.Mutex

	// Data is the in-memory map of filepath to Embedding.
	Data map[string]store.Embedding

	// Error injection fields.
	UpsertErr error
	GetErr    error
	DeleteErr error
	GetAllErr error

	// UpsertCalls records the filepaths passed to Upsert.
	UpsertCalls []string
	// DeleteCalls records the filepaths passed to Delete.
	DeleteCalls []string
}

// NewFakeEmbeddingRepository creates a FakeEmbeddingRepository with an initialized map.
func NewFakeEmbeddingRepository() *FakeEmbeddingRepository {
	return &FakeEmbeddingRepository{
		Data: make(map[string]store.Embedding),
	}
}

// Upsert stores or replaces an embedding in memory.
func (f *FakeEmbeddingRepository) Upsert(_ context.Context, filepath string, vector []float32, modelID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.UpsertCalls = append(f.UpsertCalls, filepath)

	if f.UpsertErr != nil {
		return f.UpsertErr
	}

	f.Data[filepath] = store.Embedding{
		Filepath: filepath,
		Vector:   vector,
		ModelID:  modelID,
	}
	return nil
}

// Get retrieves an embedding by filepath.
func (f *FakeEmbeddingRepository) Get(_ context.Context, filepath string) (store.Embedding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.GetErr != nil {
		return store.Embedding{}, f.GetErr
	}

	emb, exists := f.Data[filepath]
	if !exists {
		return store.Embedding{}, sberrors.Newf(sberrors.ErrCodeMetadataNotFound, "embedding not found: %s", filepath)
	}

	return emb, nil
}

// Delete removes an embedding by filepath.
func (f *FakeEmbeddingRepository) Delete(_ context.Context, filepath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.DeleteCalls = append(f.DeleteCalls, filepath)

	if f.DeleteErr != nil {
		return f.DeleteErr
	}

	delete(f.Data, filepath)
	return nil
}

// GetAll returns all embeddings.
func (f *FakeEmbeddingRepository) GetAll(_ context.Context) ([]store.Embedding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.GetAllErr != nil {
		return nil, f.GetAllErr
	}

	embeddings := make([]store.Embedding, 0, len(f.Data))
	for _, emb := range f.Data {
		embeddings = append(embeddings, emb)
	}

	sort.Slice(embeddings, func(i, j int) bool {
		return embeddings[i].Filepath < embeddings[j].Filepath
	})

	return embeddings, nil
}
