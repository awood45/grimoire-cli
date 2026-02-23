package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// jsonIntVal converts a JSON number value (float64) to an int for comparison.
// JSON numbers always decode as float64 in Go when using map[string]interface{}.
func jsonIntVal(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return -1
}

// setupGrimoire creates a minimal grimoire directory structure for testing.
func setupGrimoire(t *testing.T, basePath string, cfgOverrides map[string]interface{}) {
	t.Helper()

	dirs := []string{
		filepath.Join(basePath, "files"),
		filepath.Join(basePath, "archive-files"),
		filepath.Join(basePath, "db"),
	}
	for _, d := range dirs {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}

	// Write config.yaml.
	cfg := map[string]interface{}{
		"base_path": basePath,
		"embedding": map[string]interface{}{
			"provider":   "none",
			"model":      "nomic-embed-text",
			"dimensions": 768,
		},
		"search": map[string]interface{}{
			"default_limit": 50,
		},
		"similar": map[string]interface{}{
			"default_limit": 10,
		},
	}
	for k, v := range cfgOverrides {
		cfg[k] = v
	}

	cfgBytes, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "config.yaml"), cfgBytes, 0o600))

	// Create empty ledger.
	require.NoError(t, os.WriteFile(filepath.Join(basePath, "ledger.jsonl"), []byte{}, 0o600))

	// Create SQLite DB with schema.
	db, err := store.NewDB(filepath.Join(basePath, "db", "grimoire.sqlite"))
	require.NoError(t, err)
	require.NoError(t, db.EnsureSchema())
	require.NoError(t, db.Close())
}

// TestNewAppContext_success verifies that NewAppContext constructs all dependencies.
func TestNewAppContext_success(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	setupGrimoire(t, basePath, nil)

	ctx, err := NewAppContext(basePath)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	assert.NotNil(t, ctx.Config)
	assert.NotNil(t, ctx.Brain)
	assert.NotNil(t, ctx.DB)
	assert.NotNil(t, ctx.FileRepo)
	assert.NotNil(t, ctx.EmbRepo)
	assert.NotNil(t, ctx.Ledger)
	assert.NotNil(t, ctx.Embedder)
	assert.NotNil(t, ctx.FM)
	assert.NotNil(t, ctx.Locker)
	assert.NotNil(t, ctx.DocGen)

	// Close should succeed.
	assert.NoError(t, ctx.Close())
}

// TestNewAppContext_notInitialized verifies error when grimoire doesn't exist.
func TestNewAppContext_notInitialized(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	// Don't create any structure.

	ctx, err := NewAppContext(basePath)
	assert.Nil(t, ctx)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeNotInitialized))
}

// TestNewAppContext_invalidConfig verifies error on bad config.
func TestNewAppContext_invalidConfig(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	setupGrimoire(t, basePath, map[string]interface{}{
		"embedding": map[string]interface{}{
			"provider":   "badprovider",
			"dimensions": 768,
		},
	})

	ctx, err := NewAppContext(basePath)
	assert.Nil(t, ctx)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
}

// TestNewAppContext_schemaVersionMismatch verifies error on wrong DB schema version.
func TestNewAppContext_schemaVersionMismatch(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	setupGrimoire(t, basePath, nil)

	// Tamper with the schema version.
	db, err := store.NewDB(filepath.Join(basePath, "db", "grimoire.sqlite"))
	require.NoError(t, err)
	_, execErr := db.SQLDB().Exec("PRAGMA user_version = 999")
	require.NoError(t, execErr)
	require.NoError(t, db.Close())

	ctx, err := NewAppContext(basePath)
	assert.Nil(t, ctx)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeSchemaVersion))
}

// TestNewAppContext_ollamaProvider verifies that ollama provider is constructed.
func TestNewAppContext_ollamaProvider(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	setupGrimoire(t, basePath, map[string]interface{}{
		"embedding": map[string]interface{}{
			"provider":   "ollama",
			"model":      "nomic-embed-text",
			"dimensions": 768,
		},
	})

	ctx, err := NewAppContext(basePath)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	assert.NotNil(t, ctx.Embedder)
	assert.Equal(t, "nomic-embed-text", ctx.Embedder.ModelID())
	assert.NoError(t, ctx.Close())
}

// TestAppContext_Close verifies Close handles nil fields gracefully.
func TestAppContext_Close(t *testing.T) {
	t.Parallel()

	// Close on empty context (nil fields).
	ctx := &AppContext{}
	assert.NoError(t, ctx.Close())
}

// TestNewAppContext_missingConfig verifies error when config.yaml is missing.
func TestNewAppContext_missingConfig(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	// Create dirs but no config.yaml.
	require.NoError(t, os.MkdirAll(filepath.Join(basePath, "files"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(basePath, "db"), 0o755))

	ctx, err := NewAppContext(basePath)
	assert.Nil(t, ctx)
	require.Error(t, err)
}

// TestNewAppContext_missingLedger verifies error when ledger.jsonl cannot be opened.
func TestNewAppContext_missingLedger(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	setupGrimoire(t, basePath, nil)

	// Remove ledger and make it a directory so file open fails.
	require.NoError(t, os.Remove(filepath.Join(basePath, "ledger.jsonl")))
	require.NoError(t, os.MkdirAll(filepath.Join(basePath, "ledger.jsonl"), 0o755))

	ctx, err := NewAppContext(basePath)
	assert.Nil(t, ctx)
	require.Error(t, err)
}

// TestNewAppContext_lockerFailure verifies error when the lock file cannot be created.
func TestNewAppContext_lockerFailure(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	setupGrimoire(t, basePath, nil)

	// Create .lock as a directory so FlockLocker fails to open it.
	require.NoError(t, os.MkdirAll(filepath.Join(basePath, ".lock"), 0o755))

	ctx, err := NewAppContext(basePath)
	assert.Nil(t, ctx)
	require.Error(t, err)
}

// TestNewAppContext_dbOpenFailure verifies error when DB cannot be opened.
func TestNewAppContext_dbOpenFailure(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	setupGrimoire(t, basePath, nil)

	// Remove DB file and make the db directory read-only so NewDB fails.
	dbFile := filepath.Join(basePath, "db", "grimoire.sqlite")
	require.NoError(t, os.Remove(dbFile))
	// Make db dir read-only to prevent creating a new DB.
	require.NoError(t, os.Chmod(filepath.Join(basePath, "db"), 0o444))
	t.Cleanup(func() {
		// Restore permissions for cleanup.
		os.Chmod(filepath.Join(basePath, "db"), 0o755)
	})

	ctx, err := NewAppContext(basePath)
	assert.Nil(t, ctx)
	require.Error(t, err)
}

// TestNewAppContext_emptyEmbeddingProvider verifies empty string maps to noop.
func TestNewAppContext_emptyEmbeddingProvider(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	setupGrimoire(t, basePath, map[string]interface{}{
		"embedding": map[string]interface{}{
			"provider":   "",
			"model":      "nomic-embed-text",
			"dimensions": 768,
		},
	})

	ctx, err := NewAppContext(basePath)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	assert.Equal(t, "none", ctx.Embedder.ModelID())
	assert.NoError(t, ctx.Close())
}
