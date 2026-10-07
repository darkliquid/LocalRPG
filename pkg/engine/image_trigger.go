package engine

import "strings"

// TriggerConfig tunes the significance heuristic, so a campaign can change its
// thresholds without a schema change.
type TriggerConfig struct {
	// DecisiveOutcomes are the check outcomes that make a turn worth illustrating.
	DecisiveOutcomes []string
	// NarrationThreshold is the rune count above which a beat has substance.
	NarrationThreshold int
}

// DefaultTriggerConfig is the heuristic's default thresholds.
func DefaultTriggerConfig() TriggerConfig {
	return TriggerConfig{
		DecisiveOutcomes:   []string{"strong", "success", "pass", "critical", "miss", "fail", "failure"},
		NarrationThreshold: 400,
	}
}

// ShouldIllustrate reports whether a turn is significant enough to illustrate,
// and why. It is pure, so the decision is testable and its reason is traceable.
func ShouldIllustrate(turn, prev Turn, cfg TriggerConfig) (bool, string) {
	if turn.SceneBreak {
		return true, "scene break"
	}
	for _, check := range turn.Checks {
		if isDecisiveOutcome(check.Outcome, cfg.DecisiveOutcomes) {
			return true, "decisive check: " + check.Outcome
		}
	}
	if turn.Location != "" && prev.Location != "" && turn.Location != prev.Location {
		return true, "location changed"
	}
	if speaker := newSpeaker(turn, prev); speaker != "" {
		return true, "new speaker: " + speaker
	}
	if cfg.NarrationThreshold > 0 && len([]rune(turn.Narration)) > cfg.NarrationThreshold {
		return true, "substantial narration"
	}
	return false, ""
}

func isDecisiveOutcome(outcome string, decisive []string) bool {
	value := strings.ToLower(strings.TrimSpace(outcome))
	if value == "" {
		return false
	}
	for _, candidate := range decisive {
		if value == strings.ToLower(candidate) {
			return true
		}
	}
	return false
}

// newSpeaker returns the first speaker in the turn who did not speak in the
// previous turn, or "".
func newSpeaker(turn, prev Turn) string {
	if len(turn.Segments) == 0 {
		return ""
	}
	before := make(map[string]bool, len(prev.Segments))
	for _, segment := range prev.Segments {
		if segment.SpeakerID != "" {
			before[segment.SpeakerID] = true
		}
	}
	for _, segment := range turn.Segments {
		if segment.SpeakerID != "" && !before[segment.SpeakerID] {
			return segment.SpeakerID
		}
	}
	return ""
}

// shouldIllustrate applies the configured image policy to a turn. An unset policy
// keeps today's scene-break behaviour.
func (o *TurnOrchestrator) shouldIllustrate(turn Turn, pastTurns []Turn) bool {
	switch o.imageTrigger {
	case "off", "manual":
		return false
	case "every_turn":
		return true
	case "significant":
		// Fall through to the heuristic below.
	default:
		return turn.SceneBreak
	}

	cfg := o.triggerConfig
	if cfg.NarrationThreshold == 0 {
		cfg = DefaultTriggerConfig()
	}
	var prev Turn
	if len(pastTurns) > 0 {
		prev = pastTurns[len(pastTurns)-1]
	}
	ok, reason := ShouldIllustrate(turn, prev, cfg)
	if ok && o.logger != nil {
		o.logger.Event("scene.illustrate", map[string]interface{}{"reason": reason})
	}
	return ok
}
