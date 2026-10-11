package engine

import "strings"

// TriggerConfig tunes the significance heuristic, so a campaign can change its
// thresholds without a schema change.
type TriggerConfig struct {
	// DecisiveOutcomes are the check outcomes that make a turn worth illustrating
	// under the broad significant heuristic.
	DecisiveOutcomes []string
	// NarrationThreshold is the rune count above which a beat has substance.
	NarrationThreshold int
	// ExtremeOutcomes are the check outcomes that mark a scene change: a critical
	// result either way, not an ordinary hit or miss.
	ExtremeOutcomes []string
	// MajorCharacterWindow is how many recent turns a character must be absent from
	// to count as newly introduced.
	MajorCharacterWindow int
}

// DefaultTriggerConfig is the heuristic's default thresholds.
func DefaultTriggerConfig() TriggerConfig {
	return TriggerConfig{
		DecisiveOutcomes:     []string{"strong", "success", "pass", "critical", "miss", "fail", "failure"},
		NarrationThreshold:   400,
		ExtremeOutcomes:      []string{"critical", "critical_success", "fumble", "critical_failure"},
		MajorCharacterWindow: 5,
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

func MajorSceneChange(turn Turn, pastTurns []Turn, cfg TriggerConfig) (bool, string) {
	if turn.SceneBreak {
		return true, "scene break"
	}
	var prev Turn
	if len(pastTurns) > 0 {
		prev = pastTurns[len(pastTurns)-1]
	}
	if turn.Location != "" && prev.Location != "" && turn.Location != prev.Location {
		return true, "location changed"
	}
	for _, check := range turn.Checks {
		if isDecisiveOutcome(check.Outcome, cfg.ExtremeOutcomes) {
			return true, "extreme check: " + check.Outcome
		}
	}
	if speaker := majorNewCharacter(turn, pastTurns, cfg.MajorCharacterWindow); speaker != "" {
		return true, "major new character: " + speaker
	}
	return false, ""
}

// majorNewCharacter returns the first speaker this turn who has not spoken in the
// previous window turns, or "". A window rather than a single turn is what makes
// it a *major* introduction: a character who speaks every few beats is not new.
func majorNewCharacter(turn Turn, pastTurns []Turn, window int) string {
	if window <= 0 {
		window = 5
	}
	recent := make(map[string]bool)
	start := len(pastTurns) - window
	if start < 0 {
		start = 0
	}
	for _, past := range pastTurns[start:] {
		for _, segment := range past.Segments {
			if segment.SpeakerID != "" {
				recent[segment.SpeakerID] = true
			}
		}
	}
	for _, segment := range turn.Segments {
		if segment.SpeakerID != "" && !recent[segment.SpeakerID] {
			return segment.SpeakerID
		}
	}
	return ""
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
		// Fall through to the broad heuristic below.
	case "major":
		return o.majorSceneChange(turn, pastTurns)
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

// majorSceneChange applies the major-change policy: a scene break, a location
// change, an extreme outcome, or a newly introduced character.
func (o *TurnOrchestrator) majorSceneChange(turn Turn, pastTurns []Turn) bool {
	cfg := o.triggerConfig
	if len(cfg.ExtremeOutcomes) == 0 {
		cfg = DefaultTriggerConfig()
	}
	ok, reason := MajorSceneChange(turn, pastTurns, cfg)
	if ok && o.logger != nil {
		o.logger.Event("scene.illustrate", map[string]interface{}{"reason": reason})
	}
	return ok
}
