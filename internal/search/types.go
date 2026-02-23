package search

// SimilarInput holds the parameters for a similarity search query.
type SimilarInput struct {
	// FilePath finds files similar to an existing file's embedding.
	FilePath string
	// Text finds files similar to the provided text by generating an embedding.
	Text string
	// Limit caps the number of results returned.
	Limit int
}
