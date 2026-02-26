// Package metadata orchestrates metadata mutations across frontmatter,
// ledger, and SQLite stores with locking and optional embedding generation.
package metadata

import "github.com/awood45/grimoire-cli/internal/store"

// CreateOptions holds the parameters for creating file metadata.
type CreateOptions struct {
	Filepath             string
	SourceAgent          string
	Tags                 []string
	Summary              string
	SummaryEmbeddingText string // Optional: embed this text as summary vector
}

// UpdateOptions holds the parameters for updating file metadata.
// Nil/empty fields indicate no change. Tags nil = no change, empty = clear all.
// Summary nil = no change, pointer to empty = clear.
type UpdateOptions struct {
	Filepath             string
	Tags                 []string
	SourceAgent          string
	Summary              *string
	SummaryEmbeddingText *string // Optional: embed this text as summary vector
}

// ArchiveResult holds the result of archiving a file.
type ArchiveResult struct {
	OriginalPath string
	ArchivePath  string
	Metadata     store.FileMetadata
}
