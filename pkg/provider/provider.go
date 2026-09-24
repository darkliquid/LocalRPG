// Package provider is the leaf registry of every provider the binary ships.
// Providers register themselves on import; pkg/provider imports no other
// internal package so that no provider can create an import cycle.
package provider

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Registration is one provider as the registry sees it. Build unmarshals the
// family's config from raw JSON, so pkg/provider stays free of config and
// harness/media types; the caller type-asserts the result to the family
// interface.
type Registration struct {
	Descriptor Descriptor
	Build      func(ctx context.Context, raw []byte) (interface{}, error)
}

var (
	mu   sync.RWMutex
	byID = map[string]Registration{}
)

// Register adds a provider. It panics on an empty or duplicate ID: both are
// build-time bugs, and a panic at init is the cheapest way to catch them.
func Register(reg Registration) {
	if reg.Descriptor.ID == "" {
		panic("provider: registration with an empty ID")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, exists := byID[reg.Descriptor.ID]; exists {
		panic(fmt.Sprintf("provider: duplicate registration %q", reg.Descriptor.ID))
	}
	byID[reg.Descriptor.ID] = reg
}

// Lookup returns a registration by descriptor ID.
func Lookup(id string) (Registration, bool) {
	mu.RLock()
	defer mu.RUnlock()
	reg, ok := byID[id]
	return reg, ok
}

// List returns descriptors, optionally filtered by family, sorted by ID.
func List(families ...Family) []Descriptor {
	mu.RLock()
	defer mu.RUnlock()
	descs := make([]Descriptor, 0, len(byID))
	for _, reg := range byID {
		if len(families) > 0 && !containsFamily(families, reg.Descriptor.Family) {
			continue
		}
		descs = append(descs, reg.Descriptor)
	}
	sort.Slice(descs, func(i, j int) bool { return descs[i].ID < descs[j].ID })
	return descs
}

// IDs returns every registered ID, sorted. Used by startup validation.
func IDs() []string {
	mu.RLock()
	defer mu.RUnlock()
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Validate reports registrations that are malformed.
func Validate() error {
	mu.RLock()
	defer mu.RUnlock()
	for id, reg := range byID {
		if id == "" || reg.Descriptor.Family == "" || reg.Build == nil {
			return fmt.Errorf("provider: malformed registration %q", id)
		}
	}
	return nil
}

// Reset clears the registry. It exists for tests only.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	byID = map[string]Registration{}
}

func containsFamily(families []Family, candidate Family) bool {
	for _, family := range families {
		if family == candidate {
			return true
		}
	}
	return false
}
