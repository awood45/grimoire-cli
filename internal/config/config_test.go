package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoad_valid(t *testing.T) {
	path := writeConfig(t, `
base_path: /home/user/.grimoire
embedding:
  provider: ollama
  model: nomic-embed-text
  dimensions: 768
  ollama_url: http://localhost:11434
search:
  default_limit: 25
similar:
  default_limit: 5
`)
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "/home/user/.grimoire", cfg.BasePath)
	assert.Equal(t, "ollama", cfg.Embedding.Provider)
	assert.Equal(t, "nomic-embed-text", cfg.Embedding.Model)
	assert.Equal(t, 768, cfg.Embedding.Dimensions)
	assert.Equal(t, 25, cfg.Search.DefaultLimit)
	assert.Equal(t, 5, cfg.Similar.DefaultLimit)
}

func TestLoad_missingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	assert.Error(t, err)
}

func TestLoad_invalidYAML(t *testing.T) {
	path := writeConfig(t, `{{{invalid yaml`)
	_, err := Load(path)
	assert.Error(t, err)
}

func TestLoad_defaults(t *testing.T) {
	path := writeConfig(t, `
base_path: /home/user/.grimoire
`)
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "none", cfg.Embedding.Provider)
	assert.Equal(t, "nomic-embed-text", cfg.Embedding.Model)
	assert.Equal(t, 768, cfg.Embedding.Dimensions)
	assert.Equal(t, 50, cfg.Search.DefaultLimit)
	assert.Equal(t, 10, cfg.Similar.DefaultLimit)
}

// TestLoad_chunkingDefaults verifies that chunking config fields get correct
// default values when not explicitly set (FR-2, FR-4, FR-5, NFR-1).
func TestLoad_chunkingDefaults(t *testing.T) {
	path := writeConfig(t, `
base_path: /home/user/.grimoire
`)
	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, 1024, cfg.Embedding.Chunking.MaxTokens,
		"default max_tokens should be 1024")
	assert.Equal(t, 128, cfg.Embedding.Chunking.OverlapTokens,
		"default overlap_tokens should be 128")
	assert.Equal(t, 4, cfg.Embedding.Chunking.BytesPerToken,
		"default bytes_per_token should be 4")
	assert.Equal(t, "search_document: ", cfg.Embedding.DocumentPrefix,
		"default document_prefix should be 'search_document: '")
	assert.Equal(t, "search_query: ", cfg.Embedding.QueryPrefix,
		"default query_prefix should be 'search_query: '")
}

// TestLoad_chunkingCustomValues verifies that custom chunking config values
// override the defaults (FR-2, FR-4, FR-5).
func TestLoad_chunkingCustomValues(t *testing.T) {
	path := writeConfig(t, `
base_path: /home/user/.grimoire
embedding:
  provider: ollama
  model: nomic-embed-text
  dimensions: 768
  chunking:
    max_tokens: 2048
    overlap_tokens: 256
    bytes_per_token: 3
  document_prefix: "doc: "
  query_prefix: "query: "
`)
	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, 2048, cfg.Embedding.Chunking.MaxTokens,
		"custom max_tokens should override default")
	assert.Equal(t, 256, cfg.Embedding.Chunking.OverlapTokens,
		"custom overlap_tokens should override default")
	assert.Equal(t, 3, cfg.Embedding.Chunking.BytesPerToken,
		"custom bytes_per_token should override default")
	assert.Equal(t, "doc: ", cfg.Embedding.DocumentPrefix,
		"custom document_prefix should override default")
	assert.Equal(t, "query: ", cfg.Embedding.QueryPrefix,
		"custom query_prefix should override default")
}

// TestLoad_chunkingPartialOverride verifies that partially specified chunking
// config uses defaults for unspecified fields (FR-2).
func TestLoad_chunkingPartialOverride(t *testing.T) {
	path := writeConfig(t, `
base_path: /home/user/.grimoire
embedding:
  chunking:
    max_tokens: 512
`)
	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, 512, cfg.Embedding.Chunking.MaxTokens,
		"max_tokens should be overridden to 512")
	assert.Equal(t, 128, cfg.Embedding.Chunking.OverlapTokens,
		"overlap_tokens should use default 128")
	assert.Equal(t, 4, cfg.Embedding.Chunking.BytesPerToken,
		"bytes_per_token should use default 4")
	assert.Equal(t, "search_document: ", cfg.Embedding.DocumentPrefix,
		"document_prefix should use default")
	assert.Equal(t, "search_query: ", cfg.Embedding.QueryPrefix,
		"query_prefix should use default")
}

func TestValidate_validOllama(t *testing.T) {
	cfg := &Config{
		Embedding: EmbeddingConfig{Provider: "ollama", Dimensions: 768},
		Search:    SearchConfig{DefaultLimit: 50},
		Similar:   SimilarConfig{DefaultLimit: 10},
	}
	assert.NoError(t, cfg.Validate())
}

func TestValidate_validNone(t *testing.T) {
	cfg := &Config{
		Embedding: EmbeddingConfig{Provider: "none", Dimensions: 768},
		Search:    SearchConfig{DefaultLimit: 50},
		Similar:   SimilarConfig{DefaultLimit: 10},
	}
	assert.NoError(t, cfg.Validate())
}

func TestValidate_unknownProvider(t *testing.T) {
	cfg := &Config{
		Embedding: EmbeddingConfig{Provider: "openai", Dimensions: 768},
		Search:    SearchConfig{DefaultLimit: 50},
		Similar:   SimilarConfig{DefaultLimit: 10},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
}

func TestValidate_emptyProvider(t *testing.T) {
	cfg := &Config{
		Embedding: EmbeddingConfig{Provider: "", Dimensions: 768},
		Search:    SearchConfig{DefaultLimit: 50},
		Similar:   SimilarConfig{DefaultLimit: 10},
	}
	// Empty provider treated as "none" — should pass.
	assert.NoError(t, cfg.Validate())
}

func TestValidate_invalidDimensions(t *testing.T) {
	cfg := &Config{
		Embedding: EmbeddingConfig{Provider: "ollama", Dimensions: 0},
		Search:    SearchConfig{DefaultLimit: 50},
		Similar:   SimilarConfig{DefaultLimit: 10},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
}

func TestValidate_invalidSearchLimit(t *testing.T) {
	cfg := &Config{
		Embedding: EmbeddingConfig{Provider: "none", Dimensions: 768},
		Search:    SearchConfig{DefaultLimit: 0},
		Similar:   SimilarConfig{DefaultLimit: 10},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
}

func TestValidate_invalidSimilarLimit(t *testing.T) {
	cfg := &Config{
		Embedding: EmbeddingConfig{Provider: "none", Dimensions: 768},
		Search:    SearchConfig{DefaultLimit: 50},
		Similar:   SimilarConfig{DefaultLimit: -1},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInvalidInput))
}

func TestEffectiveOllamaURL_configured(t *testing.T) {
	cfg := EmbeddingConfig{OllamaURL: "http://gpu-server:11434"}
	assert.Equal(t, "http://gpu-server:11434", cfg.EffectiveOllamaURL())
}

func TestEffectiveOllamaURL_default(t *testing.T) {
	cfg := EmbeddingConfig{}
	assert.Equal(t, "http://localhost:11434", cfg.EffectiveOllamaURL())
}

func TestLoad_unmarshalError(t *testing.T) {
	// YAML where a struct field receives a scalar — mapstructure cannot decode this.
	path := writeConfig(t, `
embedding: "not_a_struct"
`)
	_, err := Load(path)
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeInternalError))
}
