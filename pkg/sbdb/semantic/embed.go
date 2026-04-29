package semantic

// Embedder produces vector embeddings from text.
type Embedder interface {
	// Embed returns embeddings for one or more texts.
	Embed(texts []string) ([][]float32, error)

	// ModelID returns the identifier of the embedding model.
	ModelID() string

	// Dim returns the dimensionality of the embedding vectors.
	Dim() int
}
