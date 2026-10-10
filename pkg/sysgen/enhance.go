package sysgen

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/systemtest"
)

// MaxProposals caps how many additions one enhancement call may propose.
const MaxProposals = 12

// Proposal kinds.
const (
	ProposalStat        = "stat"
	ProposalSkill       = "skill"
	ProposalProfile     = "profile"
	ProposalAdvancement = "advancement"
)

// Proposal is one proposed addition to a system. A proposal is additive: it
// never restates what the system already declares.
type Proposal struct {
	Kind        string                `json:"kind"`
	Title       string                `json:"title"`
	Reason      string                `json:"reason,omitempty"`
	Stat        *core.StatSpec        `json:"stat,omitempty"`
	Skill       *core.SkillSpec       `json:"skill,omitempty"`
	Profile     *ProfileAddition      `json:"profile,omitempty"`
	Advancement *core.AdvancementSpec `json:"advancement,omitempty"`
}

// ProfileAddition is a proposed resolution profile. It carries one of the three
// resolution shapes, so the assembled profile is valid by construction.
type ProfileAddition struct {
	Name      string                `json:"name"`
	Notation  string                `json:"notation,omitempty"`
	DC        int                   `json:"dc,omitempty"`
	SuccessOn string                `json:"success_on,omitempty"`
	Ladder    []core.LadderStep     `json:"ladder,omitempty"`
	Outcomes  []core.SuccessOutcome `json:"outcomes,omitempty"`
}

// build assembles the profile, refusing one that names no resolution shape.
func (p ProfileAddition) build() (core.ResolutionProfile, error) {
	profile := core.ResolutionProfile{Notation: p.Notation}
	switch {
	case p.DC != 0:
		profile.DC = p.DC
	case len(p.Ladder) > 0:
		profile.Ladder = p.Ladder
	case p.SuccessOn != "" && len(p.Outcomes) > 0:
		profile.SuccessOn = p.SuccessOn
		profile.Outcomes = p.Outcomes
	default:
		return core.ResolutionProfile{}, fmt.Errorf("profile %q needs a dc, a ladder, or a pool", p.Name)
	}
	return profile, nil
}

const enhanceSchema = `{"type":"object","properties":{"proposals":{"type":"array","items":{` +
	`"type":"object","properties":{"kind":{"type":"string","enum":["stat","skill","profile","advancement"]},` +
	`"title":{"type":"string"},"reason":{"type":"string"},` +
	`"stat":{"type":"object","properties":{"id":{"type":"string"},"label":{"type":"string"},"type":{"type":"string"}}},` +
	`"skill":{"type":"object","properties":{"id":{"type":"string"},"label":{"type":"string"},"stat":{"type":"string"}}},` +
	`"profile":{"type":"object","properties":{"name":{"type":"string"},"notation":{"type":"string"},` +
	`"dc":{"type":"integer"},"success_on":{"type":"string"},"ladder":{"type":"array"},"outcomes":{"type":"array"}}},` +
	`"advancement":{"type":"object"}}}}}}`

// Propose reads a system and proposes additions of the requested kinds. Nothing
// is written; the caller applies the accepted set.
func Propose(ctx context.Context, gen Generator, sys System, instruction string, kinds []string) ([]Proposal, error) {
	if gen == nil {
		return nil, fmt.Errorf("sysgen: no generator configured")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out struct {
		Proposals []Proposal `json:"proposals"`
	}
	if err := generateJSON(ctx, gen, proposePrompt(sys, instruction, kinds), enhanceSchema, &out); err != nil {
		return nil, fmt.Errorf("enhance: %w", err)
	}

	proposals := make([]Proposal, 0, len(out.Proposals))
	for _, proposal := range out.Proposals {
		if len(proposals) >= MaxProposals {
			break
		}
		proposal.Kind = normalizeProposalKind(proposal.Kind)
		proposal.Title = strings.TrimSpace(proposal.Title)
		proposal.Reason = strings.TrimSpace(proposal.Reason)
		if proposal.Kind == "" || !proposalHasContent(proposal) {
			continue
		}
		if proposal.Title == "" {
			proposal.Title = proposalDefaultTitle(proposal)
		}
		proposals = append(proposals, proposal)
	}
	return proposals, nil
}

// ApplyAdditions returns a copy of the spec with each accepted addition appended
// to its list. It refuses a duplicate id in any list, so an existing declaration
// is never altered.
func ApplyAdditions(spec *core.MechanicsSpec, accepted []Proposal) (*core.MechanicsSpec, error) {
	out := cloneMechanics(spec)
	if out.Checks.Profiles == nil {
		out.Checks.Profiles = map[string]core.ResolutionProfile{}
	}

	statIDs := make(map[string]bool, len(out.Stats))
	for _, stat := range out.Stats {
		statIDs[stat.ID] = true
	}
	skillIDs := make(map[string]bool, len(out.Skills))
	for _, skill := range out.Skills {
		skillIDs[skill.ID] = true
	}

	for _, proposal := range accepted {
		switch proposal.Kind {
		case ProposalStat:
			if proposal.Stat == nil {
				return nil, fmt.Errorf("a stat proposal has no stat")
			}
			if statIDs[proposal.Stat.ID] {
				return nil, fmt.Errorf("stat %q already exists", proposal.Stat.ID)
			}
			statIDs[proposal.Stat.ID] = true
			out.Stats = append(out.Stats, *proposal.Stat)
		case ProposalSkill:
			if proposal.Skill == nil {
				return nil, fmt.Errorf("a skill proposal has no skill")
			}
			if skillIDs[proposal.Skill.ID] {
				return nil, fmt.Errorf("skill %q already exists", proposal.Skill.ID)
			}
			skillIDs[proposal.Skill.ID] = true
			out.Skills = append(out.Skills, *proposal.Skill)
		case ProposalProfile:
			if proposal.Profile == nil {
				return nil, fmt.Errorf("a profile proposal has no profile")
			}
			if proposal.Profile.Name == "" {
				return nil, fmt.Errorf("a profile proposal has no name")
			}
			if _, exists := out.Checks.Profiles[proposal.Profile.Name]; exists {
				return nil, fmt.Errorf("profile %q already exists", proposal.Profile.Name)
			}
			profile, err := proposal.Profile.build()
			if err != nil {
				return nil, err
			}
			out.Checks.Profiles[proposal.Profile.Name] = profile
		case ProposalAdvancement:
			if proposal.Advancement == nil {
				return nil, fmt.Errorf("an advancement proposal has no advancement")
			}
			if out.Advancement != nil {
				return nil, fmt.Errorf("the system already declares advancement")
			}
			advancement := *proposal.Advancement
			out.Advancement = &advancement
		default:
			return nil, fmt.Errorf("unknown proposal kind %q", proposal.Kind)
		}
	}
	return out, nil
}

// ValidateEnhancement applies the accepted additions and runs the smoke gate on
// the result, so an addition that breaks the system is caught before it is
// offered. It makes no model call.
func ValidateEnhancement(sys System, accepted []Proposal) GateResult {
	script := strings.TrimSpace(sys.Script) != ""
	spec, err := ApplyAdditions(sys.Mechanics, accepted)
	if err != nil {
		return gateFailure(err.Error(), script)
	}
	if problems := spec.Validate(); len(problems) > 0 {
		return gateFailure(strings.Join(problems, "; "), script)
	}
	enhanced := sys
	enhanced.Mechanics = spec
	return Gate(enhanced)
}

func gateFailure(detail string, script bool) GateResult {
	return GateResult{
		OK:       false,
		Failures: []systemtest.Failure{{Scenario: "smoke", Detail: detail}},
		Script:   script,
	}
}

func normalizeProposalKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case ProposalStat:
		return ProposalStat
	case ProposalSkill:
		return ProposalSkill
	case ProposalProfile:
		return ProposalProfile
	case ProposalAdvancement:
		return ProposalAdvancement
	default:
		return ""
	}
}

func proposalHasContent(proposal Proposal) bool {
	switch proposal.Kind {
	case ProposalStat:
		return proposal.Stat != nil && strings.TrimSpace(proposal.Stat.ID) != ""
	case ProposalSkill:
		return proposal.Skill != nil && strings.TrimSpace(proposal.Skill.ID) != ""
	case ProposalProfile:
		return proposal.Profile != nil && strings.TrimSpace(proposal.Profile.Name) != ""
	case ProposalAdvancement:
		return proposal.Advancement != nil
	default:
		return false
	}
}

func proposalDefaultTitle(proposal Proposal) string {
	switch proposal.Kind {
	case ProposalStat:
		return proposal.Stat.ID
	case ProposalSkill:
		return proposal.Skill.ID
	case ProposalProfile:
		return proposal.Profile.Name
	case ProposalAdvancement:
		if proposal.Advancement.Mode != "" {
			return proposal.Advancement.Mode + " advancement"
		}
		return "advancement"
	default:
		return proposal.Kind
	}
}

// cloneMechanics copies a spec deeply enough that applying additions never
// mutates the original, so applying no proposals leaves the system unchanged.
func cloneMechanics(spec *core.MechanicsSpec) *core.MechanicsSpec {
	if spec == nil {
		return &core.MechanicsSpec{}
	}
	out := *spec
	out.Stats = append([]core.StatSpec(nil), spec.Stats...)
	out.Skills = append([]core.SkillSpec(nil), spec.Skills...)
	out.Checks.Outcome = append([]string(nil), spec.Checks.Outcome...)
	out.Checks.Difficulty = append([]core.DifficultySpec(nil), spec.Checks.Difficulty...)
	if spec.Checks.Profiles != nil {
		profiles := make(map[string]core.ResolutionProfile, len(spec.Checks.Profiles))
		maps.Copy(profiles, spec.Checks.Profiles)
		out.Checks.Profiles = profiles
	}
	if spec.Health != nil {
		health := *spec.Health
		out.Health = &health
	}
	if spec.Advancement != nil {
		advancement := *spec.Advancement
		out.Advancement = &advancement
	}
	return &out
}

func proposePrompt(sys System, instruction string, kinds []string) string {
	var b strings.Builder
	b.WriteString("Propose additions to an existing tabletop RPG system. Only propose new material; never restate or rewrite what the system already declares.\n")
	if sys.Name != "" {
		fmt.Fprintf(&b, "\nSystem Name: %s\n", sys.Name)
	}
	if sys.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", sys.Description)
	}
	if summary := mechanicsSummary(sys.Mechanics); summary != "" {
		b.WriteString("\nExisting mechanics (do not duplicate these):\n" + summary)
	}
	if strings.TrimSpace(sys.RulesPrompt) != "" {
		b.WriteString("\nRules prose (for context):\n" + truncateText(sys.RulesPrompt, 1200) + "\n")
	}
	if len(kinds) > 0 {
		fmt.Fprintf(&b, "\nKinds to propose: %s\n", strings.Join(kinds, ", "))
	}
	if instruction != "" {
		fmt.Fprintf(&b, "\nInstruction: %s\n", instruction)
	}
	fmt.Fprintf(&b, "\nPropose at most %d additions, each with a one-line reason. A skill must name a stat that exists or that you also propose.\n", MaxProposals)
	b.WriteString(`Return {"proposals":[{"kind":...,"title":...,"reason":...}]}.`)
	return b.String()
}

func mechanicsSummary(spec *core.MechanicsSpec) string {
	if spec == nil {
		return ""
	}
	var b strings.Builder
	if len(spec.Stats) > 0 {
		ids := make([]string, 0, len(spec.Stats))
		for _, stat := range spec.Stats {
			ids = append(ids, stat.ID)
		}
		fmt.Fprintf(&b, "Stats: %s\n", strings.Join(ids, ", "))
	}
	if len(spec.Skills) > 0 {
		ids := make([]string, 0, len(spec.Skills))
		for _, skill := range spec.Skills {
			ids = append(ids, skill.ID)
		}
		fmt.Fprintf(&b, "Skills: %s\n", strings.Join(ids, ", "))
	}
	if spec.Checks.Notation != "" {
		fmt.Fprintf(&b, "Check notation: %s\n", spec.Checks.Notation)
	}
	if len(spec.Checks.Profiles) > 0 {
		names := make([]string, 0, len(spec.Checks.Profiles))
		for name := range spec.Checks.Profiles {
			names = append(names, name)
		}
		fmt.Fprintf(&b, "Profiles: %s\n", strings.Join(names, ", "))
	}
	if spec.Health != nil && spec.Health.Stat != "" {
		fmt.Fprintf(&b, "Health stat: %s\n", spec.Health.Stat)
	}
	if spec.Advancement != nil {
		fmt.Fprintf(&b, "Advancement: %s\n", spec.Advancement.Mode)
	}
	return b.String()
}

func truncateText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
