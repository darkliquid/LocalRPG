package state

import (
	"fmt"
	"strings"
	"sync"
)

// State is a mutable document addressed by dot-separated paths, as in
// "actor.stats.hp". It is safe for concurrent use.
type State struct {
	mu   sync.RWMutex
	data map[string]any
}

// NewState wraps initial as a State, adopting the map rather than copying it, so
// a later Set through the State is visible through initial. A nil initial starts
// empty.
func NewState(initial map[string]any) *State {
	if initial == nil {
		initial = make(map[string]any)
	}
	return &State{data: initial}
}

// Get reads the value at a dot-separated path. It reports false when the path
// is empty, when a component is missing, and when a component that must be a
// map holds something else.
func (s *State) Get(path string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if path == "" {
		return nil, false
	}

	parts := strings.Split(path, ".")
	var current any = s.data

	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		val, exists := m[part]
		if !exists {
			return nil, false
		}
		current = val
	}
	return current, true
}

// Set writes val at a dot-separated path, creating the intermediate maps a
// missing component needs. It fails on an empty path, and on a component that
// exists but is not a map, rather than replacing what is there.
func (s *State) Set(path string, val any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if path == "" {
		return fmt.Errorf("empty path")
	}

	parts := strings.Split(path, ".")

	current := s.data
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		next, exists := current[part]
		if !exists {
			newMap := make(map[string]any)
			current[part] = newMap
			current = newMap
			continue
		}
		nextMap, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("path component %q is not a map", part)
		}
		current = nextMap
	}

	lastPart := parts[len(parts)-1]
	current[lastPart] = val
	return nil
}

// Raw returns a shallow copy of the state: the caller owns the top-level map,
// but a nested map or slice it holds is still the state's.
func (s *State) Raw() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clone := make(map[string]any, len(s.data))
	for k, v := range s.data {
		clone[k] = v
	}
	return clone
}
