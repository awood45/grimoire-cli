// Package testing provides hand-written fake implementations of frontmatter interfaces for use in tests.
package testing

import (
	"sync"

	"github.com/awood45/grimoire-cli/internal/frontmatter"
	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/internal/store"
)

// Compile-time interface assertion.
var _ frontmatter.Service = (*FakeFrontmatterService)(nil)

// FakeFrontmatterService is an in-memory implementation of frontmatter.Service.
// It tracks call history and supports error injection.
type FakeFrontmatterService struct {
	mu sync.Mutex

	// Data is the in-memory map of filepath to FileMetadata.
	Data map[string]store.FileMetadata

	// Error injection fields. When set, the corresponding method returns
	// this error instead of succeeding.
	ReadErr   error
	WriteErr  error
	RemoveErr error

	// Call tracking for test assertions.
	ReadCalls   []string
	WriteCalls  []string
	RemoveCalls []string
}

// NewFakeFrontmatterService creates a FakeFrontmatterService with an initialized map.
func NewFakeFrontmatterService() *FakeFrontmatterService {
	return &FakeFrontmatterService{
		Data: make(map[string]store.FileMetadata),
	}
}

// Read returns the stored metadata for the given path, or an error if not found.
func (f *FakeFrontmatterService) Read(absPath string) (store.FileMetadata, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.ReadCalls = append(f.ReadCalls, absPath)

	if f.ReadErr != nil {
		return store.FileMetadata{}, f.ReadErr
	}

	meta, exists := f.Data[absPath]
	if !exists {
		return store.FileMetadata{}, sberrors.Newf(sberrors.ErrCodeInvalidInput, "no frontmatter found in file: %s", absPath)
	}

	return meta, nil
}

// Write stores metadata for the given path.
func (f *FakeFrontmatterService) Write(absPath string, meta store.FileMetadata) error { //nolint:gocritic // hugeParam: interface requires value type.
	f.mu.Lock()
	defer f.mu.Unlock()

	f.WriteCalls = append(f.WriteCalls, absPath)

	if f.WriteErr != nil {
		return f.WriteErr
	}

	f.Data[absPath] = meta
	return nil
}

// Remove deletes stored metadata for the given path.
func (f *FakeFrontmatterService) Remove(absPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.RemoveCalls = append(f.RemoveCalls, absPath)

	if f.RemoveErr != nil {
		return f.RemoveErr
	}

	delete(f.Data, absPath)
	return nil
}
