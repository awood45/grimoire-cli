package cli

import (
	stderrors "errors"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/awood45/grimoire-cli/internal/config"
	"github.com/awood45/grimoire-cli/internal/docgen"
	"github.com/awood45/grimoire-cli/internal/embedding"
	"github.com/awood45/grimoire-cli/internal/filelock"
	"github.com/awood45/grimoire-cli/internal/frontmatter"
	"github.com/awood45/grimoire-cli/internal/ledger"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
)

// AppContext holds the shared dependencies for non-init commands.
// Constructed once per command invocation, closed when the command completes.
type AppContext struct {
	Config   *config.Config
	Brain    *brain.Brain
	DB       *store.DB
	FileRepo store.FileRepository
	EmbRepo  store.EmbeddingRepository
	Ledger   ledger.Ledger
	Embedder embedding.Provider
	FM       frontmatter.Service
	Locker   filelock.Locker
	DocGen   docgen.Generator
}

// NewAppContext loads config, opens DB and ledger, and constructs
// all shared dependencies. Caller must call Close() when done.
func NewAppContext(basePath string) (*AppContext, error) {
	return newAppContext(basePath, true)
}

// NewAppContextSkipVersionCheck is like NewAppContext but skips the DB
// schema version check. Used by rebuild commands that drop and recreate
// the schema themselves.
func NewAppContextSkipVersionCheck(basePath string) (*AppContext, error) {
	return newAppContext(basePath, false)
}

func newAppContext(basePath string, checkVersion bool) (*AppContext, error) {
	b := brain.New(basePath)
	if !b.Exists() {
		return nil, sberrors.New(sberrors.ErrCodeNotInitialized,
			"Grimoire not initialized. Run 'grimoire-cli init' first.")
	}

	cfg, err := config.Load(b.ConfigPath())
	if err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	db, err := store.NewDB(b.DBPath())
	if err != nil {
		return nil, err
	}

	if checkVersion {
		if err := db.CheckVersion(store.SchemaVersion); err != nil {
			db.Close()
			return nil, err
		}
	}

	fileRepo := store.NewSQLiteFileRepository(db)
	embRepo := store.NewSQLiteEmbeddingRepository(db)

	led, err := ledger.NewFileLedger(b.LedgerPath())
	if err != nil {
		db.Close()
		return nil, err
	}

	embedder, err := createEmbedder(cfg, db, led)
	if err != nil {
		return nil, err
	}

	fm := frontmatter.NewFileService()

	locker, err := filelock.NewFlockLocker(b.LockPath())
	if err != nil {
		db.Close()
		led.Close()
		return nil, err
	}

	docGen, err := docgen.NewTemplateGenerator()
	if err != nil {
		db.Close()
		led.Close()
		locker.Close()
		return nil, err
	}

	return &AppContext{
		Config: cfg, Brain: b, DB: db,
		FileRepo: fileRepo, EmbRepo: embRepo,
		Ledger: led, Embedder: embedder,
		FM: fm, Locker: locker, DocGen: docGen,
	}, nil
}

// Close releases all held resources (ledger and database).
func (a *AppContext) Close() error {
	var errs []error
	if a.Ledger != nil {
		errs = append(errs, a.Ledger.Close())
	}
	if a.DB != nil {
		errs = append(errs, a.DB.Close())
	}
	return stderrors.Join(errs...)
}

// createEmbedder builds the appropriate embedding.Provider based on config.
func createEmbedder(cfg *config.Config, db *store.DB, led ledger.Ledger) (embedding.Provider, error) {
	switch cfg.Embedding.Provider {
	case "ollama":
		return embedding.NewOllamaProvider(
			cfg.Embedding.EffectiveOllamaURL(),
			cfg.Embedding.Model,
		), nil
	case "none", "":
		return &embedding.NoopProvider{}, nil
	default:
		// This case is caught by cfg.Validate(), but defend in depth.
		db.Close()
		led.Close()
		return nil, sberrors.Newf(sberrors.ErrCodeInvalidInput,
			"Unknown embedding provider: %q. Must be \"ollama\" or \"none\".", cfg.Embedding.Provider)
	}
}
