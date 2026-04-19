package embed

import sentex "github.com/edgetools/go-sentex"

// Model wraps a sentex.Model to provide memento's embedding API.
// Callers depend on embed.Model rather than sentex.Model directly,
// preserving the option to swap the backend without touching call sites.
type Model struct {
	inner *sentex.Model
}

// LoadModel loads the all-MiniLM-L6-v2 ONNX model via go-sentex.
// On first run, go-sentex downloads ~87 MB from the HuggingFace Hub cache
// (respecting HF_HOME); subsequent runs are offline.
func LoadModel() (*Model, error) {
	m, err := sentex.LoadModel()
	if err != nil {
		return nil, err
	}
	return &Model{inner: m}, nil
}

// Dimensions returns the number of dimensions in each embedding vector
// (384 for all-MiniLM-L6-v2).
func (m *Model) Dimensions() int {
	return m.inner.Dimensions()
}

// Embed returns a 384-dimensional L2-normalised vector for text.
// Inputs longer than ~256 tokens are silently truncated by go-sentex.
func (m *Model) Embed(text string) ([]float32, error) {
	return m.inner.Embed(text)
}

// EmbedBatch returns one vector per element of texts, in the same order.
// Used at startup to embed all chunks for new or changed pages.
func (m *Model) EmbedBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	return m.inner.EmbedBatch(texts)
}
