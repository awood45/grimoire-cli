package initialize

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/awood45/grimoire-cli/internal/docgen"
	"github.com/awood45/grimoire-cli/internal/platform"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
)

// Initializer creates and configures a grimoire directory structure.
type Initializer struct {
	docGen   docgen.Generator
	detector platform.Detector
}

// NewInitializer creates an Initializer with the given document generator and
// platform detector.
func NewInitializer(docGen docgen.Generator, detector platform.Detector) *Initializer {
	return &Initializer{
		docGen:   docGen,
		detector: detector,
	}
}

// Init creates or reinitializes a grimoire at the path specified in opts.
// It creates the directory structure, writes config.yaml, generates
// grimoire.md, sets up the SQLite database, creates the ledger file,
// and installs platform skills.
func (i *Initializer) Init(_ context.Context, opts InitOptions) error {
	b := brain.New(opts.BasePath)

	if b.Exists() && !opts.Force {
		return sberrors.Newf(sberrors.ErrCodeAlreadyInitialized,
			"grimoire already exists at %s. Use --force to reinitialize", opts.BasePath)
	}

	if b.Exists() && opts.Force {
		if err := i.reinitialize(b, &opts); err != nil {
			return err
		}
	} else {
		if err := i.freshInit(b, &opts); err != nil {
			return err
		}
	}

	// Detect and install platform skills.
	platforms := i.detector.DetectPlatforms()
	if len(platforms) > 0 {
		if err := i.detector.InstallSkills(b, platforms); err != nil {
			return err
		}
	}

	return nil
}

// freshInit creates a brand new grimoire directory structure.
func (i *Initializer) freshInit(b *brain.Brain, opts *InitOptions) error {
	// Create directories.
	dirs := []string{
		b.FilesDir(),
		b.ArchiveDir(),
		filepath.Dir(b.DBPath()), // db/ directory.
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return sberrors.Wrap(err, sberrors.ErrCodeInternalError,
				fmt.Sprintf("failed to create directory: %s", dir))
		}
	}

	// Write config.yaml.
	if err := i.writeConfig(b, opts); err != nil {
		return err
	}

	// Generate and write grimoire.md.
	if err := i.writeDoc(b); err != nil {
		return err
	}

	// Create SQLite DB with schema.
	if err := i.createDB(b.DBPath()); err != nil {
		return err
	}

	// Create empty ledger.jsonl.
	if err := os.WriteFile(b.LedgerPath(), []byte{}, 0o600); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to create ledger file")
	}

	return nil
}

// reinitialize preserves existing data (files/, archive-files/, ledger.jsonl, db/)
// while regenerating config.yaml and grimoire.md.
func (i *Initializer) reinitialize(b *brain.Brain, opts *InitOptions) error {
	// Overwrite config.yaml.
	if err := i.writeConfig(b, opts); err != nil {
		return err
	}

	// Regenerate grimoire.md.
	if err := i.writeDoc(b); err != nil {
		return err
	}

	return nil
}

// writeConfig generates and writes the config.yaml file.
func (i *Initializer) writeConfig(b *brain.Brain, opts *InitOptions) error {
	provider := opts.EmbeddingProvider
	if provider == "" {
		provider = "none"
	}

	model := opts.EmbeddingModel
	if model == "" {
		model = "nomic-embed-text"
	}

	content := fmt.Sprintf(`base_path: %s
embedding:
  provider: %s
  model: %s
  api_key_env: ""
  dimensions: 768
  ollama_url: "http://localhost:11434"
search:
  default_limit: 50
similar:
  default_limit: 10
`, opts.BasePath, provider, model)

	if err := os.WriteFile(b.ConfigPath(), []byte(content), 0o600); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to write config.yaml")
	}

	return nil
}

// createDB opens a new SQLite database, applies the schema, and closes it.
func (i *Initializer) createDB(dbPath string) error {
	db, err := store.NewDB(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	return db.EnsureSchema()
}

// writeDoc generates and writes the grimoire.md document.
func (i *Initializer) writeDoc(b *brain.Brain) error {
	content, err := i.docGen.Generate(&docgen.DocData{})
	if err != nil {
		return err
	}

	if err := os.WriteFile(b.DocPath(), []byte(content), 0o600); err != nil {
		return sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to write grimoire.md")
	}

	return nil
}
