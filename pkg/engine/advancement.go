package engine

import (
	"context"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// Engine-owned stats that track advancement bookkeeping. They ride the entity's
// state so the existing sheet shows them without a new frontmatter section.
const (
	AdvancementLevelStat   = "advancement_level"
	AdvancementPendingStat = "advancement_pending"
)

// applyAdvancement awards the currency a turn earned, then advances the track or
// threshold the system declares. It is a no-op without an advancement spec, so a
// schema-light system behaves exactly as before.
func (o *TurnOrchestrator) applyAdvancement(ctx context.Context, turn *Turn) {
	if o.mechanics == nil || o.mechanics.Advancement == nil || o.playerID == "" {
		return
	}
	bridge := o.advancementBridge()
	if bridge == nil {
		return
	}

	spec := o.mechanics.Advancement
	o.logger = trace.OrNil(o.logger)
	if award := rules.EarnFromTurn(spec, o.mechanics.Checks.Outcome, turn.Checks); award != 0 {
		if err := rules.ApplyEarn(bridge, spec, o.playerID, award); err != nil {
			o.logger.Event("advancement.award_error", map[string]interface{}{"error": err.Error()})
		} else {
			o.logger.Event("advancement.award", map[string]interface{}{"amount": award})
			label := spec.Currency.Label
			if label == "" {
				label = spec.Currency.Stat
			}
			if err := o.timeline.RecordAdvancement(turn, o.playerID, fmt.Sprintf("Earned %d %s.", award, label)); err != nil {
				o.logger.Event("advancement.memory_error", map[string]interface{}{"error": err.Error()})
			}
		}
	}

	value := o.currencyValue(bridge, spec)
	switch spec.Mode {
	case "threshold":
		o.applyThreshold(turn, bridge, spec, value)
	case "track":
		o.tickTrack(turn, bridge, spec, value)
	}
}

// advancementBridge returns the host API the rules engine writes through, or nil
// when no engine is configured.
func (o *TurnOrchestrator) advancementBridge() rules.GameHostAPI {
	if o.rulesEngine == nil {
		return nil
	}
	return o.rulesEngine.HostAPI()
}

// currencyValue reads the advancement currency as an int, treating an unset stat
// as zero.
func (o *TurnOrchestrator) currencyValue(bridge rules.GameHostAPI, spec *core.AdvancementSpec) int {
	raw, err := bridge.GetStat(o.playerID, spec.Currency.Stat)
	if err != nil {
		return 0
	}
	return intValue(raw)
}

// applyThreshold applies every level the value has crossed that has not been
// applied yet, remembering the highest applied threshold on the entity.
func (o *TurnOrchestrator) applyThreshold(turn *Turn, bridge rules.GameHostAPI, spec *core.AdvancementSpec, value int) {
	applied := o.stateInt(bridge, AdvancementLevelStat)
	for _, level := range spec.Levels {
		if level.At <= applied || value < level.At {
			continue
		}
		if err := rules.ApplyEffects(bridge, o.playerID, level.Effects, level.Label); err != nil {
			o.logger.Event("advancement.threshold_error", map[string]interface{}{
				"level": level.At,
				"error": err.Error(),
			})
			continue
		}
		if err := bridge.SetStat(o.playerID, AdvancementLevelStat, level.At); err != nil {
			o.logger.Event("advancement.threshold_error", map[string]interface{}{"error": err.Error()})
			continue
		}
		applied = level.At
		o.logger.Event("advancement.threshold", map[string]interface{}{"level": level.At})
		text := fmt.Sprintf("Reached %s.", level.Label)
		if level.Label == "" {
			text = fmt.Sprintf("Crossed the %d threshold.", level.At)
		}
		if err := o.timeline.RecordAdvancement(turn, o.playerID, text); err != nil {
			o.logger.Event("advancement.memory_error", map[string]interface{}{"error": err.Error()})
		}
	}
}

// tickTrack clears a full track and flags an advancement as available, once per
// whole track_size the currency has reached.
func (o *TurnOrchestrator) tickTrack(turn *Turn, bridge rules.GameHostAPI, spec *core.AdvancementSpec, value int) {
	if spec.TrackSize <= 0 {
		return
	}
	for value >= spec.TrackSize {
		if err := rules.ApplyEarn(bridge, spec, o.playerID, -spec.TrackSize); err != nil {
			o.logger.Event("advancement.track_error", map[string]interface{}{"error": err.Error()})
			return
		}
		value -= spec.TrackSize
		if err := bridge.SetStat(o.playerID, AdvancementPendingStat, 1); err != nil {
			o.logger.Event("advancement.track_error", map[string]interface{}{"error": err.Error()})
			return
		}
		o.logger.Event("advancement.track_filled", map[string]interface{}{"size": spec.TrackSize})
		if err := o.timeline.RecordAdvancement(turn, o.playerID, "Filled an advancement track."); err != nil {
			o.logger.Event("advancement.memory_error", map[string]interface{}{"error": err.Error()})
		}
	}
}

// stateInt reads a stat through the bridge as an int, treating an unset value as
// zero.
func (o *TurnOrchestrator) stateInt(bridge rules.GameHostAPI, path string) int {
	raw, err := bridge.GetStat(o.playerID, path)
	if err != nil {
		return 0
	}
	return intValue(raw)
}

// intValue converts a stored state value to an int, tolerating the numeric
// shapes YAML and JSON decoding produce.
func intValue(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	}
	return 0
}
