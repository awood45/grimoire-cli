// Package testing provides hand-written fake implementations of platform interfaces for use in tests.
package testing

import (
	"sync"

	"github.com/awood45/grimoire-cli/internal/brain"
	"github.com/awood45/grimoire-cli/internal/platform"
)

// Compile-time interface assertion.
var _ platform.Detector = (*FakeDetector)(nil)

// FakeDetector is a fake implementation of platform.Detector.
// It returns configurable platform lists and supports error injection.
type FakeDetector struct {
	mu sync.Mutex

	// Platforms is the list of platforms returned by DetectPlatforms.
	Platforms []platform.Platform

	// Error injection field. When set, InstallSkills returns this error.
	InstallErr error

	// Call tracking for test assertions.
	DetectCalls        int
	InstallCalls       int
	InstalledBrains    []*brain.Brain
	InstalledPlatforms [][]platform.Platform
}

// NewFakeDetector creates a FakeDetector that detects no platforms by default.
func NewFakeDetector() *FakeDetector {
	return &FakeDetector{}
}

// DetectPlatforms returns the configured platform list.
func (f *FakeDetector) DetectPlatforms() []platform.Platform {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.DetectCalls++
	return f.Platforms
}

// InstallSkills records the call and returns the configured error.
func (f *FakeDetector) InstallSkills(b *brain.Brain, platforms []platform.Platform) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.InstallCalls++
	f.InstalledBrains = append(f.InstalledBrains, b)
	f.InstalledPlatforms = append(f.InstalledPlatforms, platforms)

	if f.InstallErr != nil {
		return f.InstallErr
	}

	return nil
}
