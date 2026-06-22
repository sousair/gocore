// Package embedding defines the Embedder port for turning text into dense
// vectors. Concrete implementations live in sub-packages (e.g. embedding/tei).
package embedding

import "context"

// Embedder turns text into a dense vector. The vector length is defined by the
// implementation's model; callers normalise it if their index requires it.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
