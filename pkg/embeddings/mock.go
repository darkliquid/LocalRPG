package embeddings

import (
	"context"
	"sync"
)

// MockProvider provides predictable or recorded vector embeddings for unit tests.
type MockProvider struct {
	mu        sync.Mutex
	id        string
	dims      int
	EmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)
	CallCount int
	LastTexts []string
}

// NewMockProvider creates a new MockProvider with a default dimension size.
func NewMockProvider(id string, dims int) *MockProvider {
	if id == "" {
		id = "mock-embedding"
	}
	if dims <= 0 {
		dims = 384
	}
	return &MockProvider{
		id:   id,
		dims: dims,
	}
}

func (m *MockProvider) ID() string {
	return m.id
}

func (m *MockProvider) Dimensions() int {
	return m.dims
}

func (m *MockProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	m.mu.Lock()
	m.CallCount++
	m.LastTexts = append([]string(nil), texts...)
	embedFn := m.EmbedFunc
	m.mu.Unlock()

	if embedFn != nil {
		return embedFn(ctx, texts)
	}

	// Default: return zero-vectors with 1.0 at index 0
	res := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, m.dims)
		if m.dims > 0 {
			vec[0] = 1.0
		}
		res[i] = vec
	}
	return res, nil
}

var _ Provider = (*MockProvider)(nil)
