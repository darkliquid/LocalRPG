package rules

import (
	"fmt"
	"sort"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// ResolveOpposed rolls the opponent's side of an opposed check with the actor's
// notation and compares the two totals. The higher total wins; a tie follows
// ties, where "opponent" hands it to the opponent and anything else keeps it with
// the actor. It returns the opponent's roll, its total, and whether the actor
// won, so the caller can map the contest through a profile or the conventions.
func ResolveOpposed(notation string, actorTotal, opponentBonus int, ties string) (*harness.RollSummary, int, bool, error) {
	roll, err := EvaluateRoll(notation)
	if err != nil {
		return nil, 0, false, fmt.Errorf("resolve opposed check: %w", err)
	}
	total := roll.Total + opponentBonus
	actorWon := actorTotal > total || (actorTotal == total && ties != core.TieOpponent)
	return roll.Summary(total), total, actorWon, nil
}

// OpposedStat returns the stat the opponent rolls: the request's own Opposed, or
// the profile's default when the request names none. Empty means the check is
// not opposed.
func OpposedStat(req harness.CheckRequest, p core.ResolutionProfile, hasProfile bool) string {
	if req.Opposed != "" {
		return req.Opposed
	}
	if hasProfile {
		return p.Opposed
	}
	return ""
}

// ProfileOpposedOutcome maps an opposed result through a profile: a win takes the
// profile's best outcome, a loss its worst. It returns ("", false) when the
// profile cannot decide, so the caller falls back to the conventions. It is
// shared by the schema resolver and the engine's default resolver so the two
// cannot drift.
func ProfileOpposedOutcome(p core.ResolutionProfile, actorWon bool) (string, bool) {
	if len(p.Ladder) > 0 {
		steps := append([]core.LadderStep(nil), p.Ladder...)
		sort.SliceStable(steps, func(i, j int) bool { return steps[i].Min > steps[j].Min })
		if actorWon {
			return steps[0].Outcome, true
		}
		return steps[len(steps)-1].Outcome, true
	}
	if p.DC != 0 {
		if actorWon {
			return "success", true
		}
		return "fail", true
	}
	if len(p.Outcomes) > 0 {
		outcomes := append([]core.SuccessOutcome(nil), p.Outcomes...)
		sort.SliceStable(outcomes, func(i, j int) bool { return outcomes[i].Min > outcomes[j].Min })
		if actorWon {
			return outcomes[0].Outcome, true
		}
		return outcomes[len(outcomes)-1].Outcome, true
	}
	return "", false
}
