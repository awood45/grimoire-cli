package embedding

import "context"

// Provider generates vector embeddings from text.
type Provider interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
	ModelID() string
}
