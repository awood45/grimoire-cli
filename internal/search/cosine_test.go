package search

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCosineSimilarity_identical verifies that identical vectors produce a similarity of 1.0 (FR-3.3.2).
func TestCosineSimilarity_identical(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{1.0, 2.0, 3.0}

	result := CosineSimilarity(a, b)

	assert.InDelta(t, 1.0, result, 1e-6)
}

// TestCosineSimilarity_orthogonal verifies that orthogonal vectors produce a similarity of 0.0 (FR-3.3.2).
func TestCosineSimilarity_orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}

	result := CosineSimilarity(a, b)

	assert.InDelta(t, 0.0, result, 1e-6)
}

// TestCosineSimilarity_opposite verifies that opposite vectors produce a similarity of -1.0 (FR-3.3.2).
func TestCosineSimilarity_opposite(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{-1.0, -2.0, -3.0}

	result := CosineSimilarity(a, b)

	assert.InDelta(t, -1.0, result, 1e-6)
}

// TestCosineSimilarity_zeroVector verifies that a zero vector produces a similarity of 0.0 (FR-3.3.2).
func TestCosineSimilarity_zeroVector(t *testing.T) {
	a := []float32{0.0, 0.0, 0.0}
	b := []float32{1.0, 2.0, 3.0}

	result := CosineSimilarity(a, b)

	assert.InDelta(t, 0.0, result, 1e-6)

	// Also test both zero.
	result2 := CosineSimilarity(a, a)
	assert.InDelta(t, 0.0, result2, 1e-6)
}

// TestCosineSimilarity_differentLengths verifies graceful handling of different-length vectors (FR-3.3.2).
func TestCosineSimilarity_differentLengths(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{1.0, 2.0}

	// Should handle gracefully, using only the minimum overlap.
	result := CosineSimilarity(a, b)

	// With min-length approach: dot = 1*1 + 2*2 = 5, magA = sqrt(1+4) = sqrt(5), magB = sqrt(1+4) = sqrt(5).
	// cosine = 5 / 5 = 1.0.
	assert.False(t, math.IsNaN(result), "result should not be NaN")
	assert.False(t, math.IsInf(result, 0), "result should not be Inf")
}

// TestCosineSimilarity_emptyVectors verifies that empty vectors produce 0.0 (FR-3.3.2).
func TestCosineSimilarity_emptyVectors(t *testing.T) {
	result := CosineSimilarity(nil, nil)
	assert.InDelta(t, 0.0, result, 1e-6)

	result2 := CosineSimilarity([]float32{}, []float32{})
	assert.InDelta(t, 0.0, result2, 1e-6)
}
