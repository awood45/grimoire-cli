package config

import (
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/spf13/viper"
)

// Config holds all configuration for a grimoire instance.
type Config struct {
	BasePath  string          `yaml:"base_path" mapstructure:"base_path"`
	Embedding EmbeddingConfig `yaml:"embedding" mapstructure:"embedding"`
	Search    SearchConfig    `yaml:"search" mapstructure:"search"`
	Similar   SimilarConfig   `yaml:"similar" mapstructure:"similar"`
}

// ChunkingConfig holds chunking parameters for splitting file content into
// overlapping chunks before embedding.
type ChunkingConfig struct {
	MaxTokens     int `yaml:"max_tokens" mapstructure:"max_tokens"`
	OverlapTokens int `yaml:"overlap_tokens" mapstructure:"overlap_tokens"`
	BytesPerToken int `yaml:"bytes_per_token" mapstructure:"bytes_per_token"`
}

// EmbeddingConfig holds embedding provider configuration.
type EmbeddingConfig struct {
	Provider       string         `yaml:"provider" mapstructure:"provider"`
	Model          string         `yaml:"model" mapstructure:"model"`
	APIKeyEnv      string         `yaml:"api_key_env" mapstructure:"api_key_env"`
	Dimensions     int            `yaml:"dimensions" mapstructure:"dimensions"`
	OllamaURL      string         `yaml:"ollama_url" mapstructure:"ollama_url"`
	Chunking       ChunkingConfig `yaml:"chunking" mapstructure:"chunking"`
	DocumentPrefix string         `yaml:"document_prefix" mapstructure:"document_prefix"`
	QueryPrefix    string         `yaml:"query_prefix" mapstructure:"query_prefix"`
}

// SearchConfig holds search defaults.
type SearchConfig struct {
	DefaultLimit int `yaml:"default_limit" mapstructure:"default_limit"`
}

// SimilarConfig holds similarity search defaults.
type SimilarConfig struct {
	DefaultLimit int `yaml:"default_limit" mapstructure:"default_limit"`
}

const providerNone = "none"

// Load reads config from the given YAML file path and applies defaults.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Defaults.
	v.SetDefault("embedding.provider", providerNone)
	v.SetDefault("embedding.model", "nomic-embed-text")
	v.SetDefault("embedding.dimensions", 768)
	v.SetDefault("embedding.chunking.max_tokens", 1024)
	v.SetDefault("embedding.chunking.overlap_tokens", 128)
	v.SetDefault("embedding.chunking.bytes_per_token", 4)
	v.SetDefault("embedding.document_prefix", "search_document: ")
	v.SetDefault("embedding.query_prefix", "search_query: ")
	v.SetDefault("search.default_limit", 50)
	v.SetDefault("similar.default_limit", 10)

	if err := v.ReadInConfig(); err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to read config")
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to parse config")
	}

	return &cfg, nil
}

// Validate checks that configuration values are valid.
func (c *Config) Validate() error {
	provider := c.Embedding.Provider
	if provider == "" {
		provider = providerNone
	}

	if provider != "ollama" && provider != providerNone {
		return sberrors.Newf(sberrors.ErrCodeInvalidInput, "unknown embedding provider: %q (must be \"ollama\" or \"none\")", c.Embedding.Provider)
	}

	if c.Embedding.Dimensions <= 0 {
		return sberrors.Newf(sberrors.ErrCodeInvalidInput, "embedding dimensions must be > 0, got %d", c.Embedding.Dimensions)
	}

	if c.Search.DefaultLimit <= 0 {
		return sberrors.Newf(sberrors.ErrCodeInvalidInput, "search default_limit must be > 0, got %d", c.Search.DefaultLimit)
	}

	if c.Similar.DefaultLimit <= 0 {
		return sberrors.Newf(sberrors.ErrCodeInvalidInput, "similar default_limit must be > 0, got %d", c.Similar.DefaultLimit)
	}

	if c.Embedding.Chunking.MaxTokens <= 0 {
		return sberrors.Newf(sberrors.ErrCodeInvalidInput, "chunking max_tokens must be > 0, got %d", c.Embedding.Chunking.MaxTokens)
	}
	if c.Embedding.Chunking.BytesPerToken <= 0 {
		return sberrors.Newf(sberrors.ErrCodeInvalidInput, "chunking bytes_per_token must be > 0, got %d", c.Embedding.Chunking.BytesPerToken)
	}
	if c.Embedding.Chunking.OverlapTokens < 0 {
		return sberrors.Newf(sberrors.ErrCodeInvalidInput, "chunking overlap_tokens must be >= 0, got %d", c.Embedding.Chunking.OverlapTokens)
	}
	if c.Embedding.Chunking.OverlapTokens >= c.Embedding.Chunking.MaxTokens {
		return sberrors.Newf(sberrors.ErrCodeInvalidInput, "chunking overlap_tokens (%d) must be < max_tokens (%d)", c.Embedding.Chunking.OverlapTokens, c.Embedding.Chunking.MaxTokens)
	}

	return nil
}

// EffectiveOllamaURL returns the configured Ollama URL or the default.
func (e *EmbeddingConfig) EffectiveOllamaURL() string {
	if e.OllamaURL != "" {
		return e.OllamaURL
	}
	return "http://localhost:11434"
}
