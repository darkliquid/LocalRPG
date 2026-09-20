package harness

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/storage"
)

type ContextAssembler struct {
	store *storage.Store
}

func NewContextAssembler(store *storage.Store) *ContextAssembler {
	return &ContextAssembler{store: store}
}

func (c *ContextAssembler) AssembleContext(locationID, playerID, playerAction string) (string, error) {
	var sb strings.Builder

	// Layer 1: Immediate Scene Scope
	sb.WriteString("## IMMEDIATE SCENE\n")
	if loc, err := c.store.GetEntity(locationID); err == nil && loc != nil {
		sb.WriteString(fmt.Sprintf("**Current Location:** %s\n%s\n\n", loc.Name, loc.Body))
	}

	if player, err := c.store.GetEntity(playerID); err == nil && player != nil {
		sb.WriteString(fmt.Sprintf("**Player Character:** %s\n", player.Name))
		if player.State != nil {
			sb.WriteString(fmt.Sprintf("State: %+v\n\n", player.State.Raw()))
		}
	}

	// Layer 2: Living World Arcs & Background Agendas
	sb.WriteString("## LIVING WORLD & BACKGROUND ARCS\n")
	edges, err := c.store.GetEdgesFrom(locationID)
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "arc" {
				sb.WriteString(fmt.Sprintf("### Arc: %s\n%s\n\n", ent.Name, ent.Body))
			}
		}
	}

	// Layer 3: Present Actors
	sb.WriteString("## PRESENT CHARACTERS & NOTABLE BEINGS\n")
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "character" {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", ent.Name, ent.Body))
			}
		}
	}

	// Layer 4: Player Action
	sb.WriteString("\n## PLAYER ACTION\n")
	sb.WriteString(playerAction + "\n")

	return sb.String(), nil
}
