package sysgen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
)

// Template is a known-good schema shape with parameters to fill. Its structure
// is fixed, so only the Params vary and a built spec is valid by construction.
type Template struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	Resolution  string `json:"resolution"`  // ladder | dc | pool
	Health      string `json:"health"`      // single | wounds | none
	Advancement string `json:"advancement"` // spend | track | threshold | none
	Notation    string `json:"notation,omitempty"`
}

// Params carries the values a model chooses within a template's fixed structure.
// Every field is a value, never a structure: the template decides the shape.
type Params struct {
	Stats           []core.StatSpec       `json:"stats,omitempty"`
	Skills          []core.SkillSpec      `json:"skills,omitempty"`
	Notation        string                `json:"notation,omitempty"`
	Ladder          []core.LadderStep     `json:"ladder,omitempty"`
	DC              int                   `json:"dc,omitempty"`
	SuccessOn       string                `json:"success_on,omitempty"`
	Outcomes        []core.SuccessOutcome `json:"outcomes,omitempty"`
	HealthStat      string                `json:"health_stat,omitempty"`
	AdvancementStat string                `json:"advancement_stat,omitempty"`
	Unlocks         []core.UnlockSpec     `json:"unlocks,omitempty"`
}

// Templates returns the known-good shapes a generated system may take. They are
// derived from the reference corpus (SYS-6) and the SYS-2 resolution profiles.
func Templates() []Template {
	return []Template{
		{ID: "ladder_single", Label: "Narrative ladder with a single health track", Resolution: "ladder", Health: "single", Advancement: "none", Notation: "2d6"},
		{ID: "ladder_none", Label: "Narrative ladder, no health", Resolution: "ladder", Health: "none", Advancement: "none", Notation: "2d6"},
		{ID: "ladder_spend", Label: "Narrative ladder with an advancement economy", Resolution: "ladder", Health: "single", Advancement: "spend", Notation: "2d6"},
		{ID: "dc_single", Label: "d20 against a difficulty class with a single health track", Resolution: "dc", Health: "single", Advancement: "none", Notation: "1d20"},
		{ID: "dc_none", Label: "Rules-light d20, no health", Resolution: "dc", Health: "none", Advancement: "none", Notation: "1d20"},
		{ID: "dc_spend", Label: "d20 with an advancement economy", Resolution: "dc", Health: "single", Advancement: "spend", Notation: "1d20"},
		{ID: "pool_none", Label: "Dice pool success count", Resolution: "pool", Health: "none", Advancement: "none", Notation: "5d10"},
		{ID: "pool_single", Label: "Dice pool with a single health track", Resolution: "pool", Health: "single", Advancement: "none", Notation: "5d10"},
	}
}

// MatchTemplate finds the catalogue template closest to a chosen resolution,
// health, and advancement. An exact match wins; otherwise the health and then
// the advancement fall back, so a model's near-miss still yields a usable shape.
func MatchTemplate(resolution, health, advancement string) (Template, bool) {
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	health = strings.ToLower(strings.TrimSpace(health))
	advancement = strings.ToLower(strings.TrimSpace(advancement))

	catalogue := Templates()
	for _, t := range catalogue {
		if t.Resolution == resolution && t.Health == health && t.Advancement == advancement {
			return t, true
		}
	}
	for _, t := range catalogue {
		if t.Resolution == resolution && t.Health == health {
			return t, true
		}
	}
	for _, t := range catalogue {
		if t.Resolution == resolution {
			return t, true
		}
	}
	return Template{}, false
}

// Build assembles a MechanicsSpec from a template and its parameters. It
// validates the parameters against the template and returns an error rather
// than an invalid spec, so the caller never has to check the result.
func (t Template) Build(params Params) (*core.MechanicsSpec, error) {
	name := t.ID
	if name == "" {
		name = t.Resolution
	}

	resolution := strings.ToLower(strings.TrimSpace(t.Resolution))
	notation := strings.TrimSpace(params.Notation)
	if notation == "" {
		notation = t.Notation
	}

	profile := core.ResolutionProfile{Notation: notation}
	var outcomes []string

	switch resolution {
	case "ladder":
		if len(params.Ladder) == 0 {
			return nil, fmt.Errorf("template %s: a ladder needs at least one step", name)
		}
		for _, step := range params.Ladder {
			if strings.TrimSpace(step.Outcome) == "" {
				return nil, fmt.Errorf("template %s: a ladder step has no outcome", name)
			}
		}
		profile.Ladder = params.Ladder
		outcomes = ladderOutcomes(params.Ladder)
	case "dc":
		if params.DC == 0 {
			return nil, fmt.Errorf("template %s: a dc profile needs a non-zero dc", name)
		}
		profile.DC = params.DC
		outcomes = []string{"success", "fail"}
	case "pool":
		if strings.TrimSpace(params.SuccessOn) == "" {
			return nil, fmt.Errorf("template %s: a pool needs a success target", name)
		}
		if len(params.Outcomes) == 0 {
			return nil, fmt.Errorf("template %s: a pool needs outcomes", name)
		}
		profile.SuccessOn = params.SuccessOn
		profile.Outcomes = params.Outcomes
		outcomes = poolOutcomes(params.Outcomes)
	default:
		return nil, fmt.Errorf("template %s: unknown resolution %q", name, t.Resolution)
	}

	spec := &core.MechanicsSpec{
		Stats:  params.Stats,
		Skills: params.Skills,
		Checks: core.CheckConventions{
			Notation: notation,
			Outcome:  outcomes,
			Profiles: map[string]core.ResolutionProfile{name: profile},
		},
	}

	if problems := spec.Checks.Validate(); len(problems) > 0 {
		return nil, fmt.Errorf("template %s: %s", name, strings.Join(problems, "; "))
	}

	if health := strings.ToLower(strings.TrimSpace(t.Health)); health != "" && health != "none" {
		if strings.TrimSpace(params.HealthStat) == "" {
			return nil, fmt.Errorf("template %s: health %q needs a health stat", name, t.Health)
		}
		spec.Health = &core.HealthSpec{Stat: params.HealthStat}
	}

	if advancement := strings.ToLower(strings.TrimSpace(t.Advancement)); advancement != "" && advancement != "none" {
		if strings.TrimSpace(params.AdvancementStat) == "" {
			return nil, fmt.Errorf("template %s: advancement %q needs a currency stat", name, t.Advancement)
		}
		spec.Advancement = &core.AdvancementSpec{
			Currency: core.CurrencySpec{Stat: params.AdvancementStat},
			Mode:     advancement,
			Unlocks:  params.Unlocks,
		}
	}

	return spec, nil
}

// ladderOutcomes lists a ladder's outcome vocabulary, strongest first.
func ladderOutcomes(steps []core.LadderStep) []string {
	sorted := make([]core.LadderStep, len(steps))
	copy(sorted, steps)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Min > sorted[j].Min })
	values := make([]string, len(sorted))
	for i, step := range sorted {
		values[i] = step.Outcome
	}
	return distinctOutcomes(values)
}

// poolOutcomes lists a pool's outcome vocabulary, strongest first.
func poolOutcomes(outcomes []core.SuccessOutcome) []string {
	sorted := make([]core.SuccessOutcome, len(outcomes))
	copy(sorted, outcomes)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Min > sorted[j].Min })
	values := make([]string, len(sorted))
	for i, outcome := range sorted {
		values[i] = outcome.Outcome
	}
	return distinctOutcomes(values)
}

func distinctOutcomes(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
