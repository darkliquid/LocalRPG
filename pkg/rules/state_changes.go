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
func ApplyStateChanges(bridge GameHostAPI, changes []harness.StateChangeDecl, declared map[string]core.StatSpec, allowFreeform bool) error {
	if len(changes) == 0 {
		return nil
	}
	if bridge == nil {
		return fmt.Errorf("apply state changes: no host bridge")
	}

	for _, change := range changes {
		if !allowFreeform && len(declared) > 0 {
			if _, ok := declared[change.Path]; !ok {
				return fmt.Errorf("undeclared stat %q", change.Path)
			}
		}

		switch change.Op {
		case "set":
			if err := bridge.SetStat(change.Entity, change.Path, change.Value); err != nil {
				return fmt.Errorf("set %s: %w", change.Path, err)
			}
		case "add", "sub":
			current, err := bridge.GetStat(change.Entity, change.Path)
			if err != nil {
				return fmt.Errorf("read %s: %w", change.Path, err)
			}
			base, _ := toInt(current)
			delta, ok := toInt(change.Value)
			if !ok {
				return fmt.Errorf("state change %s %s: value is not a number", change.Op, change.Path)
			}
			if change.Op == "sub" {
				delta = -delta
			}
			if err := bridge.SetStat(change.Entity, change.Path, base+delta); err != nil {
				return fmt.Errorf("update %s: %w", change.Path, err)
			}
		default:
			return fmt.Errorf("unknown state change op %q", change.Op)
		}
	}
	return nil
}
