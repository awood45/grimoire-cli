// Package testing provides hand-written fake implementations of docgen interfaces for use in tests.
package testing

import (
	"sync"

	"github.com/awood45/grimoire-cli/internal/docgen"
)

// Compile-time interface assertion.
var _ docgen.Generator = (*FakeGenerator)(nil)

// FakeGenerator is a fake implementation of docgen.Generator.
// It records calls and returns canned output.
type FakeGenerator struct {
	mu sync.Mutex

	// GenerateOutput is the string returned by Generate.
	GenerateOutput string

	// Error injection field. When set, Generate returns this error.
	GenerateErr error

	// GenerateCalls records the data passed to Generate.
	GenerateCalls []*docgen.DocData
}

// NewFakeGenerator creates a FakeGenerator with sensible defaults.
func NewFakeGenerator() *FakeGenerator {
	return &FakeGenerator{
		GenerateOutput: "# Grimoire\n\nGenerated document.",
	}
}

// Generate returns the configured output or error.
func (f *FakeGenerator) Generate(data *docgen.DocData) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.GenerateCalls = append(f.GenerateCalls, data)

	if f.GenerateErr != nil {
		return "", f.GenerateErr
	}

	return f.GenerateOutput, nil
}
