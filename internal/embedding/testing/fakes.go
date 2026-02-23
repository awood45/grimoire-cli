// Package testing provides hand-written fake implementations of embedding interfaces for use in tests.
package testing

import (
	"context"
	"sync"

	"github.com/awood45/grimoire-cli/internal/embedding"
)

// Compile-time interface assertion.
var _ embedding.Provider = (*FakeProvider)(nil)

// FakeProvider is a configurable fake implementation of embedding.Provider.
// It returns fixed vectors and model IDs for deterministic testing.
type FakeProvider struct {
	mu sync.Mutex

	// FixedVector is the vector returned by GenerateEmbedding.
	FixedVector []float32

	// FixedModelID is the model ID returned by ModelID.
	FixedModelID string

	// Error injection field. When set, GenerateEmbedding returns this error.
	GenerateErr error

	// GenerateCalls records the text inputs passed to GenerateEmbedding.
	GenerateCalls []string
}

// NewFakeProvider creates a FakeProvider with sensible defaults.
func NewFakeProvider() *FakeProvider {
	return &FakeProvider{
		FixedVector:  []float32{0.1, 0.2, 0.3},
		FixedModelID: "fake-model",
	}
}

// GenerateEmbedding returns the configured fixed vector or error.
func (f *FakeProvider) GenerateEmbedding(_ context.Context, text string) ([]float32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.GenerateCalls = append(f.GenerateCalls, text)

	if f.GenerateErr != nil {
		return nil, f.GenerateErr
	}

	// Return a copy to prevent mutation.
	result := make([]float32, len(f.FixedVector))
	copy(result, f.FixedVector)
	return result, nil
}

// ModelID returns the configured fixed model ID.
func (f *FakeProvider) ModelID() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.FixedModelID
}
