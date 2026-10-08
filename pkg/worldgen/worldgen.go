// Package worldgen turns a short brief into a draft world through a sequence of
// structured model calls. A draft is never a live world: it lives outside
// worlds/<id>/ and is committed only after review.
package worldgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/jsonrepair"
)

// Counts is how many of each thing a generation should produce.
type Counts struct {
	Locations  int `json:"locations,omitempty"`
	Factions   int `json:"factions,omitempty"`
	Characters int `json:"characters,omitempty"`
}

// DefaultCounts is the shape of a world generated from a premise alone.
func DefaultCounts() Counts {
	return Counts{Locations: 4, Factions: 3, Characters: 5}
}

// Zero reports whether no count was requested, so defaults apply.
func (c Counts) Zero() bool {
	return c.Locations == 0 && c.Factions == 0 && c.Characters == 0
}

// MaxBatch is the largest batch one entity-generation call may produce.
const MaxBatch = 10

// MaxProposals caps how many enhancements one call may propose.
const MaxProposals = 12

// Brief is the user's starting point for a whole-world generation.
type Brief struct {
	Name    string   `json:"name,omitempty"`
	Genre   string   `json:"genre,omitempty"`
	Premise string   `json:"premise"`
	Themes  []string `json:"themes,omitempty"`
	Counts  Counts   `json:"counts,omitempty"`
}

// DraftEntity is one generated entity note, before it is written.
type DraftEntity struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Tags   []string `json:"tags,omitempty"`
	Folder string   `json:"folder,omitempty"`
	Body   string   `json:"body"`
	// Source records the ingested chunk an entity came from, so provenance
	// survives into the review.
	Source string `json:"source,omitempty"`
	// Links are the wikilink targets that resolved, and Dropped the ones that did
	// not, so the review can show both.
	Links   []string `json:"links,omitempty"`
	Dropped []string `json:"dropped_links,omitempty"`
}

// DraftSection is one accept/reject unit of a draft's lore.
type DraftSection struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Draft is a generated world, before it is committed.
type Draft struct {
	ID       string             `json:"id"`
	World    core.WorldManifest `json:"world"`
	Lore     string             `json:"lore"`
	Sections []DraftSection     `json:"sections,omitempty"`
	Entities []DraftEntity      `json:"entities"`
	// Estimate and Calls record what a generation was expected to cost and what
	// it actually did, so the review can show both.
	Estimate *Estimate `json:"estimate,omitempty"`
	Calls    int       `json:"calls,omitempty"`
}

// Step is one progress report from the pipeline.
type Step struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// The pipeline's steps, in order.
const (
	StepOutline    = "outline"
	StepPlaces     = "places"
	StepCharacters = "characters"
	StepLink       = "link"
)

// Step statuses.
const (
	StatusDone  = "done"
	StatusError = "error"
)

// Generator is the model seam: a structured call returning JSON.
type Generator interface {
	GenerateJSON(ctx context.Context, prompt, schema string) ([]byte, error)
}

// ErrCallBudgetExceeded reports that a generation passed its call cap.
var ErrCallBudgetExceeded = errors.New("generation call budget exceeded")

// BudgetGenerator wraps a Generator and refuses a call once Max is reached, so
// one runaway generation cannot spend without bound. Max of zero is unbounded.
type BudgetGenerator struct {
	Inner Generator
	Max   int
	Calls int
}

// GenerateJSON forwards to the inner generator, counting calls and failing once
// the budget is spent. The error names the cap so the user can raise it.
func (b *BudgetGenerator) GenerateJSON(ctx context.Context, prompt, schema string) ([]byte, error) {
	if b.Inner == nil {
		return nil, fmt.Errorf("worldgen: no generator configured")
	}
	if b.Max > 0 && b.Calls >= b.Max {
		return nil, fmt.Errorf("%w: %d calls (raise generation.max_calls to allow more)", ErrCallBudgetExceeded, b.Max)
	}
	b.Calls++
	return b.Inner.GenerateJSON(ctx, prompt, schema)
}

// Generate runs the pipeline, reporting one Step per stage. A failure returns
// the partial draft so the earlier steps are not lost.
func Generate(ctx context.Context, gen Generator, brief Brief, onStep func(Step)) (Draft, error) {
	if gen == nil {
		return Draft{}, fmt.Errorf("worldgen: no generator configured")
	}
	brief = normalizeBrief(brief)

	draft := Draft{}
	stages := []struct {
		name string
		run  func(context.Context, Generator, Brief, *Draft) error
	}{
		{StepOutline, runOutline},
		{StepPlaces, runPlaces},
		{StepCharacters, runCharacters},
		{StepLink, runLink},
	}
	for _, stage := range stages {
		if err := ctx.Err(); err != nil {
			return draft, err
		}
		if err := stage.run(ctx, gen, brief, &draft); err != nil {
			report(onStep, Step{Name: stage.name, Status: StatusError, Detail: err.Error()})
			return draft, fmt.Errorf("worldgen: %s: %w", stage.name, err)
		}
		report(onStep, Step{Name: stage.name, Status: StatusDone})
	}
	draft.Sections = SplitLoreSections(draft.Lore)
	return draft, nil
}

func report(onStep func(Step), s Step) {
	if onStep != nil {
		onStep(s)
	}
}

// normalizeBrief fills the defaults a caller may omit and derives the id.
func normalizeBrief(b Brief) Brief {
	b.Premise = strings.TrimSpace(b.Premise)
	b.Name = strings.TrimSpace(b.Name)
	b.Genre = strings.TrimSpace(b.Genre)
	if b.Counts.Zero() {
		b.Counts = DefaultCounts()
	}
	b.Counts = clampCounts(b.Counts)
	return b
}

// clampCounts bounds a request so one generation cannot ask for an unbounded
// world. Negative counts are treated as zero.
func clampCounts(c Counts) Counts {
	return Counts{
		Locations:  clamp(c.Locations, 0, MaxBatch),
		Factions:   clamp(c.Factions, 0, MaxBatch),
		Characters: clamp(c.Characters, 0, MaxBatch),
	}
}

func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

// SalvageObjects returns the complete objects of a reply that holds an array of
// them, even when the reply was cut off part-way. A response that ran out of room
// is not a failed response: the objects written before the cut are whole, and a
// long generation cannot afford to lose a call, or to fail outright, because one
// reply was longer than the model had room for.
func SalvageObjects(raw []byte) [][]byte {
	// The furthest repair is used whether or not it validated: it has the fence
	// stripped and the prose trimmed, and the complete objects are still whole.
	return jsonrepair.ArrayElements(jsonrepair.Repair(raw).Payload)
}

// decodeJSON repairs a model reply that is fenced, padded with prose, or
// unterminated, then unmarshals it into v.
func decodeJSON(raw []byte, v any) error {
	payload := raw
	if res := jsonrepair.Repair(raw); res.OK {
		payload = res.Payload
	}
	if err := json.Unmarshal(payload, v); err != nil {
		// The length and the tail tell a cut-off reply from a reply that was never
		// JSON, which the unmarshal error alone does not.
		return fmt.Errorf("parse model reply (%d bytes, ending %q): %w", len(payload), replyTail(payload), err)
	}
	return nil
}

// replyTail is the last few bytes of a reply, so a failure report shows whether
// the model stopped mid-word.
func replyTail(payload []byte) string {
	const window = 40
	if len(payload) <= window {
		return string(payload)
	}
	return "..." + string(payload[len(payload)-window:])
}

// linkDraft validates every wikilink in every entity body against the draft's
// own ids, dropping an unresolved one (the prose stays, the link does not).
func linkDraft(d Draft) Draft {
	known := make(map[string]struct{}, len(d.Entities))
	for _, e := range d.Entities {
		if e.ID != "" {
			known[e.ID] = struct{}{}
		}
	}
	return linkAgainst(d, known)
}

// LinkDraft is linkDraft for callers outside the package, so a producer of a
// draft (an ingestion, say) resolves its links the same way the pipeline does.
func LinkDraft(d Draft) Draft { return linkDraft(d) }

// linkAgainst validates every wikilink in a set of entities against a known-id
// set, which may include ids outside the set (existing world entities).
func linkAgainst(d Draft, known map[string]struct{}) Draft {
	for i := range d.Entities {
		body, links, dropped := resolveWikilinks(d.Entities[i].Body, known)
		d.Entities[i].Body = body
		d.Entities[i].Links = links
		d.Entities[i].Dropped = dropped
	}
	return d
}

var wikilinkPattern = `[[`

// resolveWikilinks rewrites a body's wikilinks, keeping a resolved one and
// flattening an unresolved one to its label.
func resolveWikilinks(body string, known map[string]struct{}) (string, []string, []string) {
	if !strings.Contains(body, wikilinkPattern) {
		return body, nil, nil
	}
	var links, dropped []string
	seenLink := map[string]struct{}{}
	seenDrop := map[string]struct{}{}

	out := wikilinkRe.ReplaceAllStringFunc(body, func(match string) string {
		target := entity.WikilinkTarget(match)
		label := wikilinkLabel(match)
		if _, ok := known[entity.Slugify(target)]; ok {
			if _, seen := seenLink[target]; !seen {
				seenLink[target] = struct{}{}
				links = append(links, target)
			}
			return match
		}
		if _, seen := seenDrop[target]; !seen {
			seenDrop[target] = struct{}{}
			dropped = append(dropped, target)
		}
		return label
	})
	return out, links, dropped
}

// wikilinkLabel is the human-readable text of a link: the label when one is
// given, otherwise the target.
func wikilinkLabel(match string) string {
	inner := strings.TrimSuffix(strings.TrimPrefix(match, "[["), "]]")
	if idx := strings.Index(inner, "|"); idx >= 0 {
		return strings.TrimSpace(inner[idx+1:])
	}
	return strings.TrimSpace(inner)
}
