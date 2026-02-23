package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/awood45/grimoire-cli/internal/sberrors"
)

// Compile-time interface check.
var _ FileRepository = (*SQLiteFileRepository)(nil)

// SQLiteFileRepository implements FileRepository using SQLite.
type SQLiteFileRepository struct {
	db *DB
}

// NewSQLiteFileRepository creates a new SQLiteFileRepository backed by the given DB.
func NewSQLiteFileRepository(db *DB) *SQLiteFileRepository {
	return &SQLiteFileRepository{db: db}
}

// Insert inserts a new file and its tags within a single transaction (FR-3.2.1).
func (r *SQLiteFileRepository) Insert(ctx context.Context, meta FileMetadata) error { //nolint:gocritic // hugeParam: interface requires value type.
	tx, err := r.db.SQLDB().BeginTx(ctx, nil)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to begin transaction")
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op.

	_, err = tx.ExecContext(ctx,
		"INSERT INTO files (filepath, source_agent, summary, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		meta.Filepath, meta.SourceAgent, meta.Summary, meta.CreatedAt, meta.UpdatedAt,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return sberrors.New(sberrors.ErrCodeMetadataExists,
				"metadata already exists for filepath: "+meta.Filepath)
		}
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to insert file")
	}

	if err := insertTags(ctx, tx, meta.Filepath, meta.Tags); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to commit insert transaction")
	}

	return nil
}

// Update updates a file's metadata and replaces its tags within a single transaction (FR-3.2.2).
func (r *SQLiteFileRepository) Update(ctx context.Context, meta FileMetadata) error { //nolint:gocritic // hugeParam: interface requires value type.
	tx, err := r.db.SQLDB().BeginTx(ctx, nil)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to begin transaction")
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op.

	result, err := tx.ExecContext(ctx,
		"UPDATE files SET source_agent = ?, summary = ?, updated_at = ? WHERE filepath = ?",
		meta.SourceAgent, meta.Summary, meta.UpdatedAt, meta.Filepath,
	)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to update file")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to check rows affected")
	}
	if rows == 0 {
		return sberrors.New(sberrors.ErrCodeMetadataNotFound,
			"no metadata found for filepath: "+meta.Filepath)
	}

	// Delete existing tags and insert new ones.
	_, err = tx.ExecContext(ctx, "DELETE FROM file_tags WHERE filepath = ?", meta.Filepath)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to delete old tags")
	}

	if err := insertTags(ctx, tx, meta.Filepath, meta.Tags); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to commit update transaction")
	}

	return nil
}

// Get retrieves file metadata and its tags by filepath (FR-3.2.3).
func (r *SQLiteFileRepository) Get(ctx context.Context, filepath string) (FileMetadata, error) {
	var meta FileMetadata
	err := r.db.SQLDB().QueryRowContext(ctx,
		"SELECT filepath, source_agent, summary, created_at, updated_at FROM files WHERE filepath = ?",
		filepath,
	).Scan(&meta.Filepath, &meta.SourceAgent, &meta.Summary, &meta.CreatedAt, &meta.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FileMetadata{}, sberrors.New(sberrors.ErrCodeMetadataNotFound,
				"no metadata found for filepath: "+filepath)
		}
		return FileMetadata{}, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to get file")
	}

	tags, err := r.getTags(ctx, filepath)
	if err != nil {
		return FileMetadata{}, err
	}
	meta.Tags = tags

	return meta, nil
}

// Delete removes a file by filepath; foreign key cascades handle tags (FR-3.2.4).
func (r *SQLiteFileRepository) Delete(ctx context.Context, filepath string) error {
	result, err := r.db.SQLDB().ExecContext(ctx,
		"DELETE FROM files WHERE filepath = ?", filepath,
	)
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to delete file")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to check rows affected")
	}
	if rows == 0 {
		return sberrors.New(sberrors.ErrCodeMetadataNotFound,
			"no metadata found for filepath: "+filepath)
	}

	return nil
}

// validSortColumns maps user-facing sort names to SQL column expressions for Search.
var validSortColumns = map[string]string{
	"updated_at":   "f.updated_at",
	"created_at":   "f.created_at",
	"filepath":     "f.filepath",
	"source_agent": "f.source_agent",
}

// Search finds files matching the given filters (FR-3.3.1).
func (r *SQLiteFileRepository) Search(ctx context.Context, filters SearchFilters) ([]FileMetadata, error) { //nolint:gocritic // hugeParam: interface requires value type.
	query, args := buildSearchQuery(&filters)

	rows, err := r.db.SQLDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to search files")
	}
	defer rows.Close()

	// Collect all metadata first, then close the rows cursor before
	// issuing additional queries for tags. This avoids a deadlock
	// when MaxOpenConns is 1 (the SQLite default).
	var results []FileMetadata
	for rows.Next() {
		var meta FileMetadata
		if err := rows.Scan(&meta.Filepath, &meta.SourceAgent, &meta.Summary, &meta.CreatedAt, &meta.UpdatedAt); err != nil {
			return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to scan search result")
		}
		results = append(results, meta)
	}
	if err := rows.Err(); err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "error iterating search results")
	}
	rows.Close()

	// Fetch tags for each result after the search rows cursor is closed.
	for i := range results {
		tags, err := r.getTags(ctx, results[i].Filepath)
		if err != nil {
			return nil, err
		}
		results[i].Tags = tags
	}

	return results, nil
}

// buildSearchQuery constructs the SQL query and arguments for a Search call.
func buildSearchQuery(filters *SearchFilters) (queryStr string, queryArgs []any) {
	var (
		query  strings.Builder
		args   []any
		wheres []string
	)

	hasANDTags := len(filters.Tags) > 0
	hasORTags := len(filters.AnyTags) > 0

	// Base SELECT and JOIN clause.
	buildSearchSelectClause(&query, filters, hasANDTags, hasORTags, &wheres, &args)

	// Additional WHERE filters.
	buildSearchWhereFilters(filters, &wheres, &args)

	if len(wheres) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(wheres, " AND "))
	}

	// AND tags: GROUP BY + HAVING.
	if hasANDTags {
		query.WriteString(" GROUP BY f.filepath, f.source_agent, f.summary, f.created_at, f.updated_at")
		query.WriteString(fmt.Sprintf(" HAVING COUNT(DISTINCT ft.tag) = %d", len(filters.Tags)))
	}

	// Sort.
	sortCol := "f.updated_at DESC"
	if filters.Sort != "" {
		if col, ok := validSortColumns[filters.Sort]; ok {
			sortCol = col
		}
	}
	query.WriteString(" ORDER BY ")
	query.WriteString(sortCol)

	// Limit.
	if filters.Limit > 0 {
		query.WriteString(" LIMIT ?")
		args = append(args, filters.Limit)
	}

	return query.String(), args
}

// buildSearchSelectClause writes the SELECT/JOIN part of the search query.
func buildSearchSelectClause(query *strings.Builder, filters *SearchFilters, hasANDTags, hasORTags bool, wheres *[]string, args *[]any) {
	const selectCols = "SELECT f.filepath, f.source_agent, f.summary, f.created_at, f.updated_at FROM files f"

	switch {
	case hasANDTags:
		query.WriteString(selectCols)
		query.WriteString(" JOIN file_tags ft ON f.filepath = ft.filepath")
		placeholders := makePlaceholders(len(filters.Tags))
		*wheres = append(*wheres, "ft.tag IN ("+placeholders+")")
		for _, tag := range filters.Tags {
			*args = append(*args, tag)
		}
	case hasORTags:
		query.WriteString("SELECT DISTINCT f.filepath, f.source_agent, f.summary, f.created_at, f.updated_at FROM files f")
		query.WriteString(" JOIN file_tags ft ON f.filepath = ft.filepath")
		placeholders := makePlaceholders(len(filters.AnyTags))
		*wheres = append(*wheres, "ft.tag IN ("+placeholders+")")
		for _, tag := range filters.AnyTags {
			*args = append(*args, tag)
		}
	default:
		query.WriteString(selectCols)
	}
}

// buildSearchWhereFilters appends additional WHERE conditions for non-tag filters.
func buildSearchWhereFilters(filters *SearchFilters, wheres *[]string, args *[]any) {
	if filters.SourceAgent != "" {
		*wheres = append(*wheres, "f.source_agent = ?")
		*args = append(*args, filters.SourceAgent)
	}
	if filters.After != nil {
		*wheres = append(*wheres, "f.updated_at > ?")
		*args = append(*args, *filters.After)
	}
	if filters.Before != nil {
		*wheres = append(*wheres, "f.updated_at < ?")
		*args = append(*args, *filters.Before)
	}
	if filters.SummaryContains != "" {
		*wheres = append(*wheres, "f.summary LIKE '%' || ? || '%'")
		*args = append(*args, filters.SummaryContains)
	}
}

// validTagSortColumns maps user-facing sort names to SQL ORDER BY clauses for ListTags.
var validTagSortColumns = map[string]string{
	"name":  "tag ASC",
	"count": "count DESC, tag ASC",
}

// ListTags returns all tags with their file counts, sorted as specified (FR-3.3.3).
func (r *SQLiteFileRepository) ListTags(ctx context.Context, sortBy string) ([]TagCount, error) {
	orderClause := "tag ASC"
	if clause, ok := validTagSortColumns[sortBy]; ok {
		orderClause = clause
	}

	query := "SELECT tag, COUNT(*) as count FROM file_tags GROUP BY tag ORDER BY " + orderClause

	rows, err := r.db.SQLDB().QueryContext(ctx, query)
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to list tags")
	}
	defer rows.Close()

	var tags []TagCount
	for rows.Next() {
		var tc TagCount
		if err := rows.Scan(&tc.Name, &tc.Count); err != nil {
			return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to scan tag count")
		}
		tags = append(tags, tc)
	}
	if err := rows.Err(); err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "error iterating tags")
	}

	return tags, nil
}

// AllFilepaths returns the filepaths of all tracked files (FR-3.4.3).
func (r *SQLiteFileRepository) AllFilepaths(ctx context.Context) ([]string, error) {
	rows, err := r.db.SQLDB().QueryContext(ctx, "SELECT filepath FROM files")
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to query filepaths")
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var fp string
		if err := rows.Scan(&fp); err != nil {
			return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to scan filepath")
		}
		paths = append(paths, fp)
	}
	if err := rows.Err(); err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "error iterating filepaths")
	}

	return paths, nil
}

// Count returns the total number of tracked files (FR-3.4.1).
func (r *SQLiteFileRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.SQLDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM files").Scan(&count)
	if err != nil {
		return 0, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to count files")
	}
	return count, nil
}

// getTags fetches all tags for a given filepath.
func (r *SQLiteFileRepository) getTags(ctx context.Context, filepath string) ([]string, error) {
	rows, err := r.db.SQLDB().QueryContext(ctx,
		"SELECT tag FROM file_tags WHERE filepath = ? ORDER BY tag",
		filepath,
	)
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to get tags")
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to scan tag")
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "error iterating tags")
	}

	return tags, nil
}

// insertTags inserts tags for a filepath within an existing transaction.
func insertTags(ctx context.Context, tx *sql.Tx, filepath string, tags []string) error {
	for _, tag := range tags {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO file_tags (filepath, tag) VALUES (?, ?)",
			filepath, tag,
		)
		if err != nil {
			return sberrors.Wrap(err, sberrors.ErrCodeDatabaseError, "failed to insert tag: "+tag)
		}
	}
	return nil
}

// makePlaceholders returns a comma-separated string of n question marks.
func makePlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

// isUniqueConstraintError checks if an error is a SQLite unique constraint violation.
func isUniqueConstraintError(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
