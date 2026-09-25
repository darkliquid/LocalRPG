package entity

import (
	"fmt"
	"strings"
	"time"
)

// Memory kinds.
const (
	MemoryEvent        = "event"
	MemoryRelationship = "relationship"
	MemoryDiscovery    = "discovery"
	MemoryDialogue     = "dialogue"
	MemoryMechanical   = "mechanical"
)

// MemorySource records who authored a memory: the GM or the engine.
type MemorySource string

const (
	SourceGM     MemorySource = "gm"
	SourceEngine MemorySource = "engine"
)

// Memory is a searchable record of something that happened, attached to one or
// more entities. It is canonical in the index and re-derivable from the turn
// history.
type Memory struct {
	ID         int64        `json:"id"`
	Turn       int          `json:"turn"`
	Kind       string       `json:"kind"`
	EntityRefs []string     `json:"entity_refs"`
	Text       string       `json:"text"`
	Importance int          `json:"importance"`
	Tags       []string     `json:"tags,omitempty"`
	Source     MemorySource `json:"source"`
	CheckID    string       `json:"check_id,omitempty"`
	CreatedAt  time.Time    `json:"created_at,omitempty"`
}

// ValidateMemory rejects a memory that cannot be stored meaningfully.
func ValidateMemory(m *Memory) error {
	if m == nil {
		return fmt.Errorf("memory is nil")
	}
	if strings.TrimSpace(m.Text) == "" {
		return fmt.Errorf("memory has no text")
	}
	if len(m.EntityRefs) == 0 {
		return fmt.Errorf("memory has no entity refs")
	}
	if m.Importance < 1 || m.Importance > 5 {
		return fmt.Errorf("memory importance %d out of range 1-5", m.Importance)
	}
	return nil
}
