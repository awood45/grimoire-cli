package store

// SchemaVersion is the current schema version for the SQLite database.
const SchemaVersion = 2

const createFilesTable = `CREATE TABLE IF NOT EXISTS files (
    filepath TEXT PRIMARY KEY,
    source_agent TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
)`

const createFileTagsTable = `CREATE TABLE IF NOT EXISTS file_tags (
    filepath TEXT NOT NULL REFERENCES files(filepath) ON DELETE CASCADE,
    tag TEXT NOT NULL,
    PRIMARY KEY (filepath, tag)
)`

const createFileTagsIndex = `CREATE INDEX IF NOT EXISTS idx_file_tags_tag ON file_tags(tag)`

// v2 embeddings table with composite primary key and chunk metadata.
const createEmbeddingsTable = `CREATE TABLE IF NOT EXISTS embeddings (
    filepath TEXT NOT NULL REFERENCES files(filepath) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL DEFAULT 0,
    vector BLOB NOT NULL,
    model_id TEXT NOT NULL,
    generated_at DATETIME NOT NULL,
    chunk_start INTEGER NOT NULL DEFAULT 0,
    chunk_end INTEGER NOT NULL DEFAULT 0,
    is_summary BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (filepath, chunk_index)
)`

// Migration DDL for v1 -> v2.
const migrateEmbeddingsV1ToV2 = `CREATE TABLE embeddings_v2 (
    filepath TEXT NOT NULL REFERENCES files(filepath) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL DEFAULT 0,
    vector BLOB NOT NULL,
    model_id TEXT NOT NULL,
    generated_at DATETIME NOT NULL,
    chunk_start INTEGER NOT NULL DEFAULT 0,
    chunk_end INTEGER NOT NULL DEFAULT 0,
    is_summary BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (filepath, chunk_index)
)`

const migrateEmbeddingsCopyV1 = `INSERT INTO embeddings_v2 (filepath, chunk_index, vector, model_id, generated_at)
    SELECT filepath, 0, vector, model_id, generated_at FROM embeddings`

const migrateEmbeddingsDropV1 = `DROP TABLE embeddings`

const migrateEmbeddingsRenameV2 = `ALTER TABLE embeddings_v2 RENAME TO embeddings`
