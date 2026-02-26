package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/awood45/grimoire-cli/internal/sberrors"
)

// Compile-time interface check.
var _ EmbeddingRepository = (*SQLiteEmbeddingRepository)(nil)

// SQLiteEmbeddingRepository implements EmbeddingRepository using a SQLite database.
type SQLiteEmbeddingRepository struct {
	db *DB
}

// NewSQLiteEmbeddingRepository creates a new SQLiteEmbeddingRepository backed by the given DB.
func NewSQLiteEmbeddingRepository(db *DB) *SQLiteEmbeddingRepository {
	return &SQLiteEmbeddingRepository{db: db}
}

// Upsert inserts or replaces an embedding for the given (filepath, chunk_index) pair.
func (r *SQLiteEmbeddingRepository) Upsert(ctx context.Context, emb Embedding) error { //nolint:gocritic // hugeParam: interface requires value type.
	blob := EncodeVector(emb.Vector)
	now := time.Now().UTC()

	_, err := r.db.SQLDB().ExecContext(ctx,
		`INSERT OR REPLACE INTO embeddings
		 (filepath, chunk_index, vector, model_id, generated_at, chunk_start, chunk_end, is_summary)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		emb.Filepath, emb.ChunkIndex, blob, emb.ModelID, now,
		emb.ChunkStart, emb.ChunkEnd, emb.IsSummary,
	)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to upsert embedding")
	}
	return nil
}

// Get retrieves a single representative embedding for the given filepath.
// Returns the embedding with the lowest chunk_index (summary at -1 if it exists, else chunk 0).
// Returns METADATA_NOT_FOUND if no embedding exists for the filepath.
func (r *SQLiteEmbeddingRepository) Get(ctx context.Context, filepath string) (Embedding, error) {
	var emb Embedding
	var blob []byte

	err := r.db.SQLDB().QueryRowContext(ctx,
		`SELECT filepath, chunk_index, vector, model_id, generated_at, chunk_start, chunk_end, is_summary
		 FROM embeddings WHERE filepath = ? ORDER BY chunk_index ASC LIMIT 1`,
		filepath,
	).Scan(&emb.Filepath, &emb.ChunkIndex, &blob, &emb.ModelID, &emb.GeneratedAt,
		&emb.ChunkStart, &emb.ChunkEnd, &emb.IsSummary)

	if errors.Is(err, sql.ErrNoRows) {
		return Embedding{}, sberrors.Newf(sberrors.ErrCodeMetadataNotFound,
			"embedding not found for %q", filepath)
	}
	if err != nil {
		return Embedding{}, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to get embedding")
	}

	emb.Vector = DecodeVector(blob)
	return emb, nil
}

// GetForFile returns all embeddings for a file, ordered by chunk_index ascending.
// Returns METADATA_NOT_FOUND if no embeddings exist for the filepath.
func (r *SQLiteEmbeddingRepository) GetForFile(ctx context.Context, filepath string) ([]Embedding, error) {
	rows, err := r.db.SQLDB().QueryContext(ctx,
		`SELECT filepath, chunk_index, vector, model_id, generated_at, chunk_start, chunk_end, is_summary
		 FROM embeddings WHERE filepath = ? ORDER BY chunk_index ASC`,
		filepath,
	)
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to query embeddings for file")
	}
	defer rows.Close()

	var result []Embedding
	for rows.Next() {
		var emb Embedding
		var blob []byte
		if err := rows.Scan(&emb.Filepath, &emb.ChunkIndex, &blob, &emb.ModelID, &emb.GeneratedAt,
			&emb.ChunkStart, &emb.ChunkEnd, &emb.IsSummary); err != nil {
			return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to scan embedding row")
		}
		emb.Vector = DecodeVector(blob)
		result = append(result, emb)
	}
	if err := rows.Err(); err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "error iterating embedding rows")
	}

	if len(result) == 0 {
		return nil, sberrors.Newf(sberrors.ErrCodeMetadataNotFound,
			"embeddings not found for %q", filepath)
	}

	return result, nil
}

// DeleteForFile removes all embeddings for the given filepath.
// Does not return an error if no embeddings exist for the filepath.
func (r *SQLiteEmbeddingRepository) DeleteForFile(ctx context.Context, filepath string) error {
	_, err := r.db.SQLDB().ExecContext(ctx,
		"DELETE FROM embeddings WHERE filepath = ?",
		filepath,
	)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to delete embedding")
	}
	return nil
}

// GetAll retrieves all stored embeddings with decoded vectors.
func (r *SQLiteEmbeddingRepository) GetAll(ctx context.Context) ([]Embedding, error) {
	rows, err := r.db.SQLDB().QueryContext(ctx,
		`SELECT filepath, chunk_index, vector, model_id, generated_at, chunk_start, chunk_end, is_summary
		 FROM embeddings`,
	)
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to query embeddings")
	}
	defer rows.Close()

	var result []Embedding
	for rows.Next() {
		var emb Embedding
		var blob []byte
		if err := rows.Scan(&emb.Filepath, &emb.ChunkIndex, &blob, &emb.ModelID, &emb.GeneratedAt,
			&emb.ChunkStart, &emb.ChunkEnd, &emb.IsSummary); err != nil {
			return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to scan embedding row")
		}
		emb.Vector = DecodeVector(blob)
		result = append(result, emb)
	}
	if err := rows.Err(); err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "error iterating embedding rows")
	}

	return result, nil
}
