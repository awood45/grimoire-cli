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

// Upsert inserts or replaces an embedding for the given filepath.
func (r *SQLiteEmbeddingRepository) Upsert(ctx context.Context, filepath string, vector []float32, modelID string) error {
	blob := EncodeVector(vector)
	now := time.Now().UTC()

	_, err := r.db.SQLDB().ExecContext(ctx,
		"INSERT OR REPLACE INTO embeddings (filepath, vector, model_id, generated_at) VALUES (?, ?, ?, ?)",
		filepath, blob, modelID, now,
	)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to upsert embedding")
	}
	return nil
}

// Get retrieves the embedding for the given filepath.
// Returns METADATA_NOT_FOUND if the embedding does not exist.
func (r *SQLiteEmbeddingRepository) Get(ctx context.Context, filepath string) (Embedding, error) {
	var emb Embedding
	var blob []byte

	err := r.db.SQLDB().QueryRowContext(ctx,
		"SELECT filepath, vector, model_id, generated_at FROM embeddings WHERE filepath = ?",
		filepath,
	).Scan(&emb.Filepath, &blob, &emb.ModelID, &emb.GeneratedAt)

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

// Delete removes the embedding for the given filepath.
// Does not return an error if the embedding does not exist.
func (r *SQLiteEmbeddingRepository) Delete(ctx context.Context, filepath string) error {
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
		"SELECT filepath, vector, model_id, generated_at FROM embeddings",
	)
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to query embeddings")
	}
	defer rows.Close()

	var result []Embedding
	for rows.Next() {
		var emb Embedding
		var blob []byte
		if err := rows.Scan(&emb.Filepath, &blob, &emb.ModelID, &emb.GeneratedAt); err != nil {
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
