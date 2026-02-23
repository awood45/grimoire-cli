package initialize_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/awood45/grimoire-cli/internal/config"
	"github.com/awood45/grimoire-cli/internal/docgen"
	docgentesting "github.com/awood45/grimoire-cli/internal/docgen/testing"
	"github.com/awood45/grimoire-cli/internal/initialize"
	"github.com/awood45/grimoire-cli/internal/platform"
	platformtesting "github.com/awood45/grimoire-cli/internal/platform/testing"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInit_createsStructure verifies FR-3.1.1: a fresh init creates the
// full directory structure with files/, archive-files/, db/, config.yaml,
// grimoire.md, and ledger.jsonl.
func TestInit_createsStructure(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.NoError(t, err)

	// Verify directories exist.
	assertDirExists(t, filepath.Join(basePath, "files"))
	assertDirExists(t, filepath.Join(basePath, "archive-files"))
	assertDirExists(t, filepath.Join(basePath, "db"))

	// Verify files exist.
	assertFileExists(t, filepath.Join(basePath, "config.yaml"))
	assertFileExists(t, filepath.Join(basePath, "grimoire.md"))
	assertFileExists(t, filepath.Join(basePath, "ledger.jsonl"))

	// Verify grimoire.md content comes from generator.
	docContent, err := os.ReadFile(filepath.Join(basePath, "grimoire.md"))
	require.NoError(t, err)
	assert.Equal(t, fakeGen.GenerateOutput, string(docContent))

	// Verify generator was called with empty DocData.
	require.Len(t, fakeGen.GenerateCalls, 1)
	assert.Equal(t, &docgen.DocData{}, fakeGen.GenerateCalls[0])

	// Verify the DB is valid by opening it and checking schema.
	db, err := store.NewDB(filepath.Join(basePath, "db", "grimoire.sqlite"))
	require.NoError(t, err)
	defer db.Close()
	err = db.CheckVersion(store.SchemaVersion)
	require.NoError(t, err)

	// Verify ledger is an empty file.
	ledgerContent, err := os.ReadFile(filepath.Join(basePath, "ledger.jsonl"))
	require.NoError(t, err)
	assert.Empty(t, ledgerContent)
}

// TestInit_alreadyExists verifies FR-3.1.1: returns ALREADY_INITIALIZED
// when the brain exists and --force is not set.
func TestInit_alreadyExists(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	// First init should succeed.
	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.NoError(t, err)

	// Second init without --force should fail.
	err = initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeAlreadyInitialized),
		"expected ALREADY_INITIALIZED error, got: %v", err)
}

// TestInit_force_preservesData verifies FR-3.1.1: with --force, init preserves
// existing files/, archive-files/, ledger.jsonl, and DB while regenerating
// config.yaml and grimoire.md.
func TestInit_force_preservesData(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	// Initial setup.
	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.NoError(t, err)

	// Write a test file into files/ and ledger content to verify preservation.
	testFilePath := filepath.Join(basePath, "files", "test.md")
	err = os.WriteFile(testFilePath, []byte("# Test"), 0o600)
	require.NoError(t, err)

	ledgerPath := filepath.Join(basePath, "ledger.jsonl")
	ledgerData := []byte("sentinel-ledger-content\n")
	err = os.WriteFile(ledgerPath, ledgerData, 0o600)
	require.NoError(t, err)

	// Reinitialize with force and different generator output.
	fakeGen.GenerateOutput = "# Updated Grimoire"
	err = initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
		Force:    true,
	})
	require.NoError(t, err)

	// Verify files/ content is preserved.
	content, err := os.ReadFile(testFilePath)
	require.NoError(t, err)
	assert.Equal(t, "# Test", string(content))

	// Verify ledger is preserved.
	ledgerContent, err := os.ReadFile(ledgerPath)
	require.NoError(t, err)
	assert.Equal(t, "sentinel-ledger-content\n", string(ledgerContent))

	// Verify grimoire.md is regenerated.
	docContent, err := os.ReadFile(filepath.Join(basePath, "grimoire.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Updated Grimoire", string(docContent))

	// Verify config.yaml is regenerated (exists and is valid).
	assertFileExists(t, filepath.Join(basePath, "config.yaml"))
}

// TestInit_embeddingConfig verifies FR-3.1.1: embedding flags populate config.yaml.
func TestInit_embeddingConfig(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath:          basePath,
		EmbeddingProvider: "ollama",
		EmbeddingModel:    "custom-model",
	})
	require.NoError(t, err)

	// Read and verify the config file contains the embedding settings.
	configContent, err := os.ReadFile(filepath.Join(basePath, "config.yaml"))
	require.NoError(t, err)

	configStr := string(configContent)
	assert.Contains(t, configStr, "ollama")
	assert.Contains(t, configStr, "custom-model")
}

// TestInit_customPath verifies FR-3.1.1: --path flag overrides the default base path.
func TestInit_customPath(t *testing.T) {
	t.Parallel()
	basePath := filepath.Join(t.TempDir(), "custom", "path")

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.NoError(t, err)

	// Verify the structure was created at the custom path.
	assertDirExists(t, filepath.Join(basePath, "files"))
	assertDirExists(t, filepath.Join(basePath, "archive-files"))
	assertDirExists(t, filepath.Join(basePath, "db"))
	assertFileExists(t, filepath.Join(basePath, "config.yaml"))
}

// TestInit_detectsClaudeCode verifies FR-3.1.2: the initializer detects
// platforms and installs skills.
func TestInit_detectsClaudeCode(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	fakeDet.Platforms = []platform.Platform{platform.PlatformClaudeCode}
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.NoError(t, err)

	// Verify detection was called.
	assert.Equal(t, 1, fakeDet.DetectCalls)

	// Verify skill installation was called with the correct platform list.
	assert.Equal(t, 1, fakeDet.InstallCalls)
	require.Len(t, fakeDet.InstalledPlatforms, 1)
	assert.Equal(t, []platform.Platform{platform.PlatformClaudeCode}, fakeDet.InstalledPlatforms[0])
}

// TestInit_noPlatform verifies FR-3.1.2: init works when no platform is detected.
func TestInit_noPlatform(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	// No platforms configured (default).
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.NoError(t, err)

	// Verify detection was called.
	assert.Equal(t, 1, fakeDet.DetectCalls)

	// Verify no skill installation was attempted (no platforms to install for).
	assert.Equal(t, 0, fakeDet.InstallCalls)
}

// TestInit_generatorError verifies that a generator error is propagated.
func TestInit_generatorError(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeGen.GenerateErr = sberrors.New(sberrors.ErrCodeInternalError, "generation failed")
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestInit_installSkillsError verifies that an InstallSkills error is propagated.
func TestInit_installSkillsError(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	fakeDet.Platforms = []platform.Platform{platform.PlatformClaudeCode}
	fakeDet.InstallErr = sberrors.New(sberrors.ErrCodeInternalError, "install failed")
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestInit_defaultEmbeddingProvider verifies that the default embedding provider is "none".
func TestInit_defaultEmbeddingProvider(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.NoError(t, err)

	configContent, err := os.ReadFile(filepath.Join(basePath, "config.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(configContent), "provider: none")
}

// TestInit_mkdirError verifies that a MkdirAll error is wrapped and returned.
func TestInit_mkdirError(t *testing.T) {
	t.Parallel()

	// Use a file (not a directory) as base path so MkdirAll fails when trying
	// to create subdirectories.
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "not-a-dir")
	err := os.WriteFile(basePath, []byte("block"), 0o600)
	require.NoError(t, err)

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err = initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestInit_writeConfigError verifies that a config write error on reinit is propagated.
func TestInit_writeConfigError(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	// First, do a successful init.
	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.NoError(t, err)

	// Make the config.yaml a read-only directory to force a write error.
	configPath := filepath.Join(basePath, "config.yaml")
	err = os.Remove(configPath)
	require.NoError(t, err)
	err = os.MkdirAll(configPath, 0o755)
	require.NoError(t, err)

	err = initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
		Force:    true,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestInit_writeDocError verifies that a doc write error on reinit is propagated.
func TestInit_writeDocError(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	// First, do a successful init.
	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.NoError(t, err)

	// Make the grimoire.md a directory to force a write error.
	docPath := filepath.Join(basePath, "grimoire.md")
	err = os.Remove(docPath)
	require.NoError(t, err)
	err = os.MkdirAll(docPath, 0o755)
	require.NoError(t, err)

	err = initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
		Force:    true,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestInit_ledgerWriteError verifies that a ledger file creation error is propagated.
func TestInit_ledgerWriteError(t *testing.T) {
	t.Parallel()

	// Create a base path where ledger.jsonl is a directory to force a write error.
	basePath := t.TempDir()

	// Pre-create ledger.jsonl as a directory so the init write fails.
	ledgerPath := filepath.Join(basePath, "ledger.jsonl")
	err := os.MkdirAll(ledgerPath, 0o755)
	require.NoError(t, err)

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err = initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestInit_freshInit_writeConfigError verifies that a config write error during
// fresh init is propagated.
func TestInit_freshInit_writeConfigError(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	// Pre-create config.yaml as a directory so write fails.
	// Brain won't exist (no files/ or db/), so fresh init path is taken.
	configPath := filepath.Join(basePath, "config.yaml")
	err := os.MkdirAll(configPath, 0o755)
	require.NoError(t, err)

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err = initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestInit_freshInit_writeDocError verifies that a doc write error during
// fresh init is propagated.
func TestInit_freshInit_writeDocError(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	// Pre-create grimoire.md as a directory so write fails.
	// Brain won't exist (no files/ or db/), so fresh init path is taken.
	docPath := filepath.Join(basePath, "grimoire.md")
	err := os.MkdirAll(docPath, 0o755)
	require.NoError(t, err)

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err = initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}

// TestInit_freshInit_dbError verifies that a database creation error during
// fresh init is propagated.
func TestInit_freshInit_dbError(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	// Pre-create the DB file as a directory so NewDB fails.
	dbPath := filepath.Join(basePath, "db", "grimoire.sqlite")
	err := os.MkdirAll(dbPath, 0o755)
	require.NoError(t, err)

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err = initializer.Init(context.Background(), initialize.InitOptions{
		BasePath: basePath,
	})
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeDatabaseError))
}

// TestInit_configLoadableByConfigPackage verifies FR-3.1.1: the generated
// config.yaml is valid and can be loaded by the config package.
func TestInit_configLoadableByConfigPackage(t *testing.T) {
	t.Parallel()
	basePath := t.TempDir()

	fakeGen := docgentesting.NewFakeGenerator()
	fakeDet := platformtesting.NewFakeDetector()
	initializer := initialize.NewInitializer(fakeGen, fakeDet)

	err := initializer.Init(context.Background(), initialize.InitOptions{
		BasePath:          basePath,
		EmbeddingProvider: "ollama",
		EmbeddingModel:    "custom-model",
	})
	require.NoError(t, err)

	// Load the config and verify it's valid.
	cfg, err := config.Load(filepath.Join(basePath, "config.yaml"))
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())

	assert.Equal(t, basePath, cfg.BasePath)
	assert.Equal(t, "ollama", cfg.Embedding.Provider)
	assert.Equal(t, "custom-model", cfg.Embedding.Model)
	assert.Equal(t, 768, cfg.Embedding.Dimensions)
	assert.Equal(t, 50, cfg.Search.DefaultLimit)
	assert.Equal(t, 10, cfg.Similar.DefaultLimit)
}

// assertDirExists checks that the given path exists and is a directory.
func assertDirExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err, "directory should exist: %s", path)
	assert.True(t, info.IsDir(), "should be a directory: %s", path)
}

// assertFileExists checks that the given path exists and is a regular file.
func assertFileExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err, "file should exist: %s", path)
	assert.False(t, info.IsDir(), "should be a regular file: %s", path)
}
