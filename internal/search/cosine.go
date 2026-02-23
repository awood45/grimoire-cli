package search

import "math"

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// Returns 0.0 if either vector has zero magnitude or if vectors are empty.
// If vectors have different lengths, only the overlapping prefix is used.
func CosineSimilarity(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}

	if n == 0 {
		return 0.0
	}

	var dot, magA, magB float64
	for i := 0; i < n; i++ {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		magA += ai * ai
		magB += bi * bi
	}

	mag := math.Sqrt(magA) * math.Sqrt(magB)
	if mag == 0.0 {
		return 0.0
	}

	return dot / mag
}
