package state

import (
	"fmt"
	"strings"
	"sync"
)

type State struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

func NewState(initial map[string]interface{}) *State {
	if initial == nil {
		initial = make(map[string]interface{})
	}
	return &State{data: initial}
}

func (s *State) Get(path string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	parts := strings.Split(path, ".")
	var current interface{} = s.data

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
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

func (s *State) Set(path string, val interface{}) error {
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
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
			continue
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("path component %q is not a map", part)
		}
		current = nextMap
	}

	lastPart := parts[len(parts)-1]
	current[lastPart] = val
	return nil
}

func (s *State) Raw() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clone := make(map[string]interface{}, len(s.data))
	for k, v := range s.data {
		clone[k] = v
	}
	return clone
}
