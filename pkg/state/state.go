package state

import (
	"fmt"
	"strings"
	"sync"
)

type State struct {
	mu   sync.RWMutex
	data map[string]any
}

func NewState(initial map[string]any) *State {
	if initial == nil {
		initial = make(map[string]any)
	}
	return &State{data: initial}
}

func (s *State) Get(path string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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

func (s *State) Set(path string, val any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return fmt.Errorf("empty path")
	}

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

func (s *State) Raw() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clone := make(map[string]any, len(s.data))
	for k, v := range s.data {
		clone[k] = v
	}
	return clone
}
