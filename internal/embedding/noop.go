package embedding

import "context"

// NoopProvider is a no-op embedding provider that returns nil embeddings.
type NoopProvider struct{}

// Compile-time interface check.
var _ Provider = (*NoopProvider)(nil)

// GenerateEmbedding returns nil, nil (no embedding generated).
func (p *NoopProvider) GenerateEmbedding(_ context.Context, _ string) ([]float32, error) {
	return nil, nil
}

// ModelID returns "none".
func (p *NoopProvider) ModelID() string {
	return "none"
}
