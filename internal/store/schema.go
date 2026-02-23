package store

// SchemaVersion is the current schema version for the SQLite database.
const SchemaVersion = 1

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

const createEmbeddingsTable = `CREATE TABLE IF NOT EXISTS embeddings (
    filepath TEXT PRIMARY KEY REFERENCES files(filepath) ON DELETE CASCADE,
    vector BLOB NOT NULL,
    model_id TEXT NOT NULL,
    generated_at DATETIME NOT NULL
)`
