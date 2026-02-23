// Package initialize implements the init command logic: directory creation,
// config generation, database setup, and platform skill installation.
package initialize

// InitOptions holds the parameters for initializing a grimoire.
type InitOptions struct {
	BasePath          string
	Force             bool
	EmbeddingProvider string
	EmbeddingModel    string
}
