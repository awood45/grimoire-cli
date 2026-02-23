package store

import "time"

// FileMetadata represents the metadata tracked for each markdown file.
type FileMetadata struct {
	Filepath    string
	SourceAgent string
	Tags        []string
	Summary     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Embedding represents a vector embedding for a file.
type Embedding struct {
	Filepath    string
	Vector      []float32
	ModelID     string
	GeneratedAt time.Time
}

// SearchFilters defines criteria for filtering file metadata queries.
type SearchFilters struct {
	Tags            []string
	AnyTags         []string
	SourceAgent     string
	After           *time.Time
	Before          *time.Time
	SummaryContains string
	Limit           int
	Sort            string
}

// TagCount holds a tag name and the number of files using it.
type TagCount struct {
	Name  string
	Count int
}

// SimilarityResult holds a file path, similarity score, and its metadata.
type SimilarityResult struct {
	Filepath string
	Score    float64
	Metadata FileMetadata
}
