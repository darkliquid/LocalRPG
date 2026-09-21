package storage

import (
	"path/filepath"
	"sync"
)

// Pool hands out one shared Store per database path so callers stop opening and
// closing connections per request.
type Pool struct {
	mu     sync.Mutex
	stores map[string]*Store
}

func NewPool() *Pool {
	return &Pool{stores: make(map[string]*Store)}
}

// Store returns the shared Store for path, opening it on first use.
func (p *Pool) Store(path string) (*Store, error) {
	key := filepath.Clean(path)

	p.mu.Lock()
	defer p.mu.Unlock()

	if store, ok := p.stores[key]; ok {
		return store, nil
	}

	store, err := NewStore(key)
	if err != nil {
		return nil, err
	}
	store.shared = true
	p.stores[key] = store
	return store, nil
}

// Close releases every handle the pool opened.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error
	for key, store := range p.stores {
		if err := store.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(p.stores, key)
	}
	return firstErr
}
