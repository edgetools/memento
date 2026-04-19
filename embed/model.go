package embed

import (
	"runtime/debug"

	sentex "github.com/edgetools/go-sentex"
)

const modelID = "all-MiniLM-L6-v2"

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

// ID returns the canonical model identifier ("all-MiniLM-L6-v2").
// Used as part of the cache key to detect model changes.
func (m *Model) ID() string {
	return modelID
}

// SentexVersion returns the version string of the github.com/edgetools/go-sentex
// module as recorded in the binary's build info. Returns "unknown" if build info
// is unavailable (e.g. when running via `go run` without module mode).
func (m *Model) SentexVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/edgetools/go-sentex" {
				return dep.Version
			}
		}
	}
	return "unknown"
}

// EmbedBatch returns one vector per element of texts, in the same order.
// Used at startup to embed all chunks for new or changed pages.
func (m *Model) EmbedBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	return m.inner.EmbedBatch(texts)
}
