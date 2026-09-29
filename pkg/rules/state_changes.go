package rules

import (
	"fmt"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// ApplyStateChanges writes the GM's proposed state changes through the host
// bridge, so they persist to the entity's Markdown frontmatter. When a system
// declares stats and freeform state is not allowed, an undeclared path is
// rejected.
//
// A change the engine cannot apply - an entity reference that resolves to nothing,
// a value that is not a number, an undeclared stat - is skipped and reported in
// the returned notes instead of failing. The GM's prose has already narrated the
// consequence, and a mechanics mistake must not cost the player the turn they just
// played. The error is reserved for a bridge that was never wired.
func ApplyStateChanges(bridge GameHostAPI, changes []harness.StateChangeDecl, declared map[string]core.StatSpec, allowFreeform bool) ([]string, error) {
	if len(changes) == 0 {
		return nil, nil
	}
	if bridge == nil {
		return nil, fmt.Errorf("apply state changes: no host bridge")
	}

	notes := make([]string, 0, len(changes))
	for _, change := range changes {
		if !allowFreeform && len(declared) > 0 {
			if _, ok := declared[change.Path]; !ok {
				notes = append(notes, fmt.Sprintf("state change to %s on %q was rejected: undeclared stat", change.Path, change.Entity))
				continue
			}
		}

		switch change.Op {
		case "set":
			if err := bridge.SetStat(change.Entity, change.Path, change.Value); err != nil {
				notes = append(notes, fmt.Sprintf("state change to %s on %q was skipped: %v", change.Path, change.Entity, err))
			}
		case "add", "sub":
			current, err := bridge.GetStat(change.Entity, change.Path)
			if err != nil {
				notes = append(notes, fmt.Sprintf("state change to %s on %q was skipped: %v", change.Path, change.Entity, err))
				continue
			}
			base, _ := toInt(current)
			delta, ok := toInt(change.Value)
			if !ok {
				notes = append(notes, fmt.Sprintf("state change %s %s on %q was skipped: value is not a number", change.Op, change.Path, change.Entity))
				continue
			}
			if change.Op == "sub" {
				delta = -delta
			}
			if err := bridge.SetStat(change.Entity, change.Path, base+delta); err != nil {
				notes = append(notes, fmt.Sprintf("state change to %s on %q was skipped: %v", change.Path, change.Entity, err))
			}
		default:
			notes = append(notes, fmt.Sprintf("state change to %s on %q was skipped: unknown op %q", change.Path, change.Entity, change.Op))
		}
	}
	return notes, nil
}
