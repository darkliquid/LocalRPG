package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// ContextLimits bounds what an assembled prompt may contain. A budget is what
// lets a larger model be used without the prompt growing until it exceeds the
// model's own window.
type ContextLimits struct {
	// TokenBudget is an estimated ceiling. Zero means unbounded.
	TokenBudget int
	// RecentTurns is how many prior turns to recall. Zero uses the default.
	RecentTurns int
	// RecentTurnChars caps one prior turn's text. Zero uses the default.
	RecentTurnChars int
	// Recall bounds. Zero uses the documented defaults.
	SceneRecallTurns  int
	SceneRecallChars  int
	RetrievalTurns    int
	RetrievalChars    int
	RetrievalHalflife int
}

const (
	defaultRecentTurns       = 6
	defaultRecentTurnChars   = 1200
	defaultSceneRecallTurns  = 4
	defaultRecallChars       = 800
	defaultRetrievalTurns    = 3
	defaultRetrievalHalflife = 12
	// minRecentTurnChars is the floor a turn's excerpt can be shortened to before
	// the recall section is dropped instead.
	minRecentTurnChars = 200
	// runesPerToken is the usual rough ratio for English prose. It is an estimate
	// on purpose: a real tokenizer would be another dependency in exchange for a
	// number that only decides how much to trim.
	runesPerToken = 4
)

// RecentTurn is one prior turn as the narrator should recall it.
type RecentTurn struct {
	Number    int
	Mode      string
	Input     string
	Narration string
}

// SpeechCueContext describes the vocal steering hints allowed in generation.
type SpeechCueContext struct {
	AudioTags        bool
	MarkdownEmphasis bool
	SampleTags       []string
	CustomGuidance   string
}

// ContextRequest is everything one assembly needs. It replaced a positional
// parameter list because recall needs the turn number, and a summary and more will
// follow, at which point the list stops being readable.
type ContextRequest struct {
	LocationID string
	PlayerID   string
	Action     string
	// PlayerName is the protagonist's display name, so the narrator can name
	// them when restating the action.
	PlayerName string
	// ActionEcho asks for a leading third-person restatement of the action.
	ActionEcho bool
	// OpeningScene is the campaign's opening prose, carried on the first turn so
	// the narrator can restate it before continuing.
	OpeningScene string
	// OpeningHooks asks the first turn to extend the scene with events that invite
	// the protagonist to act. False for the quiet scene turn that precedes the
	// player's own first action.
	OpeningHooks bool
	RulesPrompt  string
	LorePrompt   string
	// MechanicsPrompt is the engine's instruction on when to roll, generated
	// from the loaded system. Empty when the system ships no mechanics.
	MechanicsPrompt string
	Profiles        []config.VoiceProfile
	Recent          []RecentTurn
	TurnNumber      int
	Mode            string
	// Summary is the campaign's recollection of everything older than the recall
	// window. It is lossy, so it is stated as subordinate to canon.
	Summary        string
	SummaryVersion int
	// WorkingSet contains the active continuity references selected for the turn.
	WorkingSet []Ref
	// Threads are the unresolved arcs, already rendered with their idle counts. They
	// are canon, so they are never trimmed: a thread goes quiet precisely when the
	// narrator should be prompted to return to it.
	Threads []string
	// SpeechCues specifies the vocal steering hints the active TTS engine supports.
	SpeechCues SpeechCueContext
	// OmitVoiceCatalog suppresses the static catalog of all voice profiles in favor of tool-based discovery.
	OmitVoiceCatalog bool
	// Context carries the caller's trace context so assembly can be a span. Nil
	// means background, which keeps callers that never had one working.
	Context context.Context
}

// SectionStat reports one section's cost so a trace can explain the prompt.
type SectionStat struct {
	Name     string
	Tokens   int
	Included bool
}

// AssembleResult is a prompt plus a note of what was left out, so a caller can
// report trimming rather than losing context silently.
type AssembleResult struct {
	Prompt          string
	PrefixPrompt    string
	DeltaPrompt     string
	EstimatedTokens int
	Trimmed         []string
	Sections        []SectionStat
	Context         TurnContext
}

// section is one block of the prompt. Its place in the slice is its place in the
// prompt; rank is the order it is surrendered when the budget bites, lowest first.
type section struct {
	name      string
	source    string
	text      string
	droppable bool
	rank      int
	refs      []Ref
}

type ContextAssembler struct {
	store  *storage.Store
	limits ContextLimits
	logger trace.Logger
}

func NewContextAssembler(store *storage.Store) *ContextAssembler {
	return &ContextAssembler{store: store}
}

// SetLimits applies the campaign's context budget to every later assembly.
func (c *ContextAssembler) SetLimits(limits ContextLimits) {
	c.limits = limits
}

// Limits reports the limits in force, so a caller can prove configuration reached
// the assembler rather than assuming it.
func (c *ContextAssembler) Limits() ContextLimits {
	return c.limits
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (c *ContextAssembler) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

// Assemble builds the prompt and trims it to the configured budget. Trimming is
// ordered by what the narrator can best do without, defined by each section's rank.
func (c *ContextAssembler) Assemble(req ContextRequest) (AssembleResult, error) {
	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}
	_, span := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/harness").Start(ctx, "context.assemble",
		oteltrace.WithAttributes(attribute.Int("context.budget", c.limits.TokenBudget)),
	)
	defer span.End()

	sections, err := c.buildSections(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return AssembleResult{}, err
	}
	result := c.fitToBudget(req, sections)
	span.SetAttributes(attribute.Int("context.tokens", result.EstimatedTokens))
	for _, section := range result.Sections {
		span.AddEvent("section", oteltrace.WithAttributes(
			attribute.String("context.section", section.Name),
			attribute.Int("context.section.tokens", section.Tokens),
			attribute.Bool("context.section.included", section.Included),
		))
		contextMetrics().contextTokens.Record(ctx, int64(section.Tokens),
			otelmetric.WithAttributes(attribute.String("context.section", section.Name)))
	}
	return result, nil
}

// actionEchoInstruction tells the narrator to re-anchor the scene on the player's
// action before resolving it.
const actionEchoInstruction = `## PLAYER ACTION ECHO
Open every turn with one short narration segment that restates the player's action
in the third person, using the character's name or a pronoun, before anything else
happens. Do not change what they did, invent intent they did not state, or resolve
it in that sentence; re-anchor the scene on their action, then continue.

Example:
  Player (Stretch Layabout): I jump into my ship, blasting my pursuers as the hatch closes.
  Opening narration: Stretch jumps into their ship, firing blaster shots at their pursuers until the canopy seals shut.
`

// actionEchoSection renders the echo instruction, or nothing when the turn is
// not an echoed action.
func actionEchoSection(enabled bool, action string) string {
	if !enabled || strings.TrimSpace(action) == "" {
		return ""
	}
	return actionEchoInstruction + "\n"
}

// openingRestateInstruction tells the narrator to restate the campaign's opening
// scene before continuing, so the first turn never starts mid-consequence.
const openingRestateInstruction = `## OPENING SCENE
This is the campaign's first turn. Open the narration by restating the scene below
in your own words, keeping its facts, imagery, and mood, so the player can tell
where they are and what is already happening from this turn alone. Do not skip
straight to the consequences.

Do not decide the protagonist's actions, thoughts, or feelings.
`

// openingEstablishInstruction is the same framing for a campaign with no opening
// prompt: the scene comes from the world, the rules, and the lore.
const openingEstablishInstruction = `## OPENING SCENE
This is the campaign's first turn. Establish the opening of this campaign: describe
where the protagonist is and what they can perceive.

Do not decide the protagonist's actions, thoughts, or feelings.
`

// openingHooksInstruction asks for the developments that draw the player in.
const openingHooksInstruction = `
Once the scene is established, continue with one thing that invites the protagonist
to act, and at most one present character, named as they are already known. Stop
there; do not resolve the protagonist's response.
`

// openingSceneSection renders the first turn's scene framing, or nothing when the
// turn is not an opening one. The scene is restated when the campaign has an
// opening prompt, and established from the world when it does not.
func openingSceneSection(scene string, hooks bool) string {
	trimmed := strings.TrimSpace(scene)
	if trimmed == "" && !hooks {
		return ""
	}

	var sb strings.Builder
	if trimmed != "" {
		sb.WriteString(openingRestateInstruction)
		sb.WriteString("\nOpening scene:\n")
		sb.WriteString(trimmed)
		sb.WriteString("\n")
	} else {
		sb.WriteString(openingEstablishInstruction)
	}
	if hooks {
		sb.WriteString(openingHooksInstruction)
	}
	return sb.String()
}

// turnFramingInstruction teaches the line framing the engine parses as the reply
// streams: blockquote speech and @-prefixed control records.
const turnFramingInstruction = `## TURN FORMAT
Format reminder: Every spoken line or dialogue beat MUST start with '> Speaker: "utterance"'. Plain prose without '>' is for narration only. Use '@persona {"name":"...", "reveals":"Old Name"}' when an unknown identity is revealed.

Write the turn as prose. Mark each spoken line as a blockquote whose speaker is
named before a colon, on its own line:

> Kaelen: You didn't see me here.

Every spoken line or dialogue beat MUST start with '> '. Even disembodied or unknown
speakers must use this format (e.g. '> Unknown Voice: "Who is there?"').

Emit a control record on its own line as ` + "`@type {json}`" + ` when the turn needs
one. Records are not shown to the player:
- ` + "`@persona {\"name\",\"type\",\"new\",\"gender\",\"pronouns\",\"role_tags\",\"description\",\"voice_hint\",\"reveals\"}`" + ` before a new character's first line, so they can be voiced. When an unknown or generic identity is revealed (e.g. "Unknown Voice" or "Generic Scout" turns out to be "Doctor Cain"), set ` + "`\"reveals\":\"Unknown Voice\"`" + ` so their previous identity maps to their true name and their history and voice are linked.
- ` + "`@roll {\"actor\",\"check_kind\",\"stat\",\"stakes\",\"outcomes\"}`" + ` to ask the engine to resolve an uncertain action. It ends your reply; you will be asked to continue with the result. Never invent or restate dice results; the engine renders the roll for the player.
- ` + "`@state {\"entity\",\"path\",\"op\",\"value\",\"reason\"}`" + ` after a state change.
- ` + "`@memory {\"kind\",\"entity_refs\",\"text\",\"importance\"}`" + ` to record a memory.
- ` + "`@move {\"location\"}`" + ` to move the protagonist.
`

// buildSections composes the prompt in order. Everything a section needs is read
// here, so trimming never re-reads the store.
func (c *ContextAssembler) buildSections(req ContextRequest) ([]section, error) {
	canonText, canonRefs, err := c.assembleCanon(req)
	if err != nil {
		return nil, err
	}

	catalogue := ""
	var catalogueRefs []Ref
	if len(req.Profiles) > 0 {
		if req.OmitVoiceCatalog {
			catalogue = "## NPC VOICES\nNew characters can be voiced dynamically. Use the search_voice_profiles and assign_voice tools to discover and assign voices, or specify voice_hint on a @persona record.\n\n"
		} else {
			catalogue = FormatVoiceProfilesCatalog(req.Profiles) + "\n"
			for _, p := range req.Profiles {
				catalogueRefs = append(catalogueRefs, Ref{Kind: RefEntity, ID: p.ID, Relation: "voice"})
			}
		}
	}

	recentText, recentRefs := c.recentSection(req)
	recallText, recallRefs := c.sceneRecall(req)
	retrievalText, retrievalRefs := c.relevantHistory(req)
	summaryText, summaryRefs := summarySection(req.Summary, req.SummaryVersion)
	workingSetText, workingSetRefs := c.workingSetSection(req)

	var actionRefs []Ref
	if req.PlayerID != "" {
		actionRefs = append(actionRefs, Ref{Kind: RefEntity, ID: req.PlayerID, Relation: "action"})
	}

	// An opening turn has no action to answer, so the section is absent rather than
	// empty: presenting it made the GM respond to the scene it was given.
	actionText := ""
	if strings.TrimSpace(req.Action) != "" {
		actionText = "\n## PLAYER ACTION\n"
		if name := strings.TrimSpace(req.PlayerName); name != "" {
			actionText += name + ": "
		}
		actionText += req.Action + "\n"
	}

	return []section{
		{name: "rules", source: "rules_prompt", text: rulesSection(req.RulesPrompt)},
		{name: "mechanics", source: "mechanics_prompt", text: mechanicsSection(req.MechanicsPrompt)},
		{name: "lore", source: "lore_prompt", text: loreSection(req.LorePrompt)},
		{name: "instructions", source: "speech_cues", text: FormatSpeechFormattingInstructions(req.SpeechCues) + "\n\n"},
		{name: "framing", source: "turn_format", text: turnFramingInstruction + "\n"},
		{name: "action_echo", source: "action_echo", text: actionEchoSection(req.ActionEcho, req.Action)},
		{name: "opening_scene", source: "opening_scene", text: openingSceneSection(req.OpeningScene, req.OpeningHooks)},
		{name: "canon", source: "canon", text: canonText, refs: canonRefs},
		{name: "working_set", source: "working_set", text: workingSetText, refs: workingSetRefs, droppable: true, rank: 5},
		{name: "summary", source: "summary", text: summaryText, refs: summaryRefs, droppable: true, rank: 6},
		{name: "recent", source: "recent", text: recentText, refs: recentRefs, droppable: true, rank: 4},
		{name: "recall", source: "scene_recall", text: recallText, refs: recallRefs, droppable: true, rank: 3},
		{name: "retrieval", source: "retrieval", text: retrievalText, refs: retrievalRefs, droppable: true, rank: 2},
		{name: "catalogue", source: "profiles", text: catalogue, refs: catalogueRefs, droppable: true, rank: 1},
		{name: "action", source: "player_action", text: actionText, refs: actionRefs},
	}, nil
}

func (c *ContextAssembler) workingSetSection(req ContextRequest) (string, []Ref) {
	if len(req.WorkingSet) == 0 {
		return "", nil
	}

	var lines []string
	var refs []Ref
	for _, r := range req.WorkingSet {
		refs = append(refs, r)
		if c.store != nil {
			if ent, err := c.store.GetEntity(r.ID); err == nil && ent != nil {
				roleDesc := r.Relation
				if roleDesc == "" {
					roleDesc = ent.Type
				}
				lines = append(lines, fmt.Sprintf("- **%s** (%s)", ent.Name, roleDesc))
				continue
			}
		}
		lines = append(lines, fmt.Sprintf("- **%s**", r.ID))
	}

	if len(lines) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("\n## ACTIVE CONTINUITY\n")
	sb.WriteString("Recently active beings, places, and ongoing arcs:\n")
	for _, line := range lines {
		sb.WriteString(line + "\n")
	}
	return sb.String(), refs
}

func rulesSection(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	return "## SYSTEM RULES & RESOLUTION MECHANICS\n" + strings.TrimSpace(prompt) + "\n\n"
}

func mechanicsSection(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	return strings.TrimSpace(prompt) + "\n\n"
}

func loreSection(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	return "## WORLD LORE & ATMOSPHERE\n" + strings.TrimSpace(prompt) + "\n\n"
}

// assembleCanon renders what is true: the scene, the player, the world's arcs, and
// who is present. It is never trimmed, because every line of it is a constraint the
// model would otherwise have to guess.
func (c *ContextAssembler) assembleCanon(req ContextRequest) (string, []Ref, error) {
	var sb strings.Builder
	var refs []Ref

	sb.WriteString("## IMMEDIATE SCENE\n")
	if loc, err := c.store.GetEntity(req.LocationID); err == nil && loc != nil {
		refs = append(refs, Ref{Kind: RefEntity, ID: loc.ID, Relation: "location"})
		sb.WriteString(fmt.Sprintf("**Current Location:** %s\n%s\n", loc.Name, strings.TrimSpace(loc.Body)))
		if loc.State != nil {
			if rendered := RenderState(loc.State.Raw()); rendered != "" {
				sb.WriteString("State: " + rendered + "\n")
			}
		}
		sb.WriteString("\n")
	}

	if player, err := c.store.GetEntity(req.PlayerID); err == nil && player != nil {
		refs = append(refs, Ref{Kind: RefEntity, ID: player.ID, Relation: "player"})
		sb.WriteString(fmt.Sprintf("**Player Character:** %s (the protagonist, played by the user)\n", player.Name))
		sb.WriteString(playerProfile(player))
		if player.State != nil {
			sb.WriteString(fmt.Sprintf("State: %+v\n", player.State.Raw()))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## LIVING WORLD & BACKGROUND ARCS\n")
	edges, err := c.store.GetEdgesFrom(req.LocationID)
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "arc" {
				refs = append(refs, Ref{Kind: RefEntity, ID: ent.ID, Relation: "arc"})
				sb.WriteString(fmt.Sprintf("### Arc: %s\n%s\n", ent.Name, strings.TrimSpace(ent.Body)))
				if ent.State != nil {
					if rendered := RenderState(ent.State.Raw()); rendered != "" {
						sb.WriteString("State: " + rendered + "\n")
					}
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("## PRESENT CHARACTERS & NOTABLE BEINGS\n")
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "character" {
				refs = append(refs, Ref{Kind: RefEntity, ID: ent.ID, Relation: "present"})
				sb.WriteString(canonEntity(ent) + "\n")
			}
		}
	}

	if names := c.establishedNames(req); names != "" {
		sb.WriteString("\n" + names)
	}

	if len(req.Threads) > 0 {
		sb.WriteString("\n## OPEN THREADS\n")
		for _, thread := range req.Threads {
			refs = append(refs, Ref{Kind: RefThread, ID: thread, Relation: "thread"})
			sb.WriteString("- " + thread + "\n")
		}
	}

	return sb.String(), dedupeRefs(refs), nil
}

// playerProfile renders the protagonist's authored traits. The narrator is told
// the name already; without the rest it tends to reinvent age, gender, and
// pronouns turn to turn.
func playerProfile(ent *entity.Entity) string {
	var sb strings.Builder
	if v := strings.TrimSpace(ent.Age); v != "" {
		sb.WriteString("Age: " + v + "\n")
	}
	if v := strings.TrimSpace(ent.Gender); v != "" {
		sb.WriteString("Gender: " + v + "\n")
	}
	if ent.ExtraMeta != nil {
		if v, ok := ent.ExtraMeta["pronouns"].(string); ok && strings.TrimSpace(v) != "" {
			sb.WriteString("Pronouns: " + strings.TrimSpace(v) + "\n")
		}
	}
	if v := strings.TrimSpace(ent.Appearance); v != "" {
		sb.WriteString("Appearance: " + v + "\n")
	}
	return sb.String()
}

// windowTurns is the part of the timeline actually replayed. The caller passes the
// whole timeline and the limit is applied here, so every section that reasons about
// "already in the window" agrees on what the window is.
func (c *ContextAssembler) windowTurns(req ContextRequest) []RecentTurn {
	window := c.limits.RecentTurns
	if window <= 0 {
		window = defaultRecentTurns
	}
	turns := req.Recent
	if len(turns) > window {
		turns = turns[len(turns)-window:]
	}
	return turns
}

// summarySection renders the campaign's long memory. It is explicitly a
// recollection: it is model-written and lossy, and canon wins wherever they differ.
func summarySection(summary string, summaryVersion int) (string, []Ref) {
	if strings.TrimSpace(summary) == "" {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("\n## STORY SO FAR (a recollection, not authoritative)\n")
	sb.WriteString("Where this differs from the state and notes above, they are correct and this is not.\n")
	sb.WriteString(strings.TrimSpace(summary) + "\n")
	refs := []Ref{{Kind: RefSummary, ID: strconv.Itoa(summaryVersion), Relation: "summary"}}
	return sb.String(), refs
}

// recentSection renders the tail of the timeline, bounded by the configured window
// and excerpt length.
func (c *ContextAssembler) recentSection(req ContextRequest) (string, []Ref) {
	charLimit := c.limits.RecentTurnChars
	if charLimit <= 0 {
		charLimit = defaultRecentTurnChars
	}
	window := c.windowTurns(req)
	var refs []Ref
	for _, turn := range window {
		refs = append(refs, Ref{Kind: RefTurn, ID: strconv.Itoa(turn.Number), Relation: "recent"})
	}
	return formatRecentTurns(window, charLimit), dedupeRefs(refs)
}

// recalledTurns returns the turns scene recall will render: the newest at this
// location, oldest first, excluding anything the window already replays. Retrieval
// shares it so it excludes exactly what was shown, rather than everything that
// happens to be at this location.
func (c *ContextAssembler) recalledTurns(req ContextRequest) []storage.TurnRecord {
	if c.store == nil || req.LocationID == "" {
		return nil
	}

	limit := c.limits.SceneRecallTurns
	if limit <= 0 {
		limit = defaultSceneRecallTurns
	}

	window := c.windowTurns(req)
	inWindow := make(map[int]bool, len(window))
	for _, turn := range window {
		inWindow[turn.Number] = true
	}

	before := req.TurnNumber
	if before <= 0 {
		before = 1 << 30
	}

	// Ask for extra, because some are filtered out as already in the window.
	turns, err := c.store.TurnsAtLocation(req.LocationID, before, limit+len(inWindow))
	if err != nil {
		return nil
	}

	selected := make([]storage.TurnRecord, 0, limit)
	for _, turn := range turns {
		if inWindow[turn.Number] {
			continue
		}
		if len(selected) == limit {
			break
		}
		selected = append(selected, turn)
	}
	return selected
}

// sceneRecall renders what happened where the party is standing. A place feels
// continuous only if returning to it is not the same as arriving.
func (c *ContextAssembler) sceneRecall(req ContextRequest) (string, []Ref) {
	if c.store == nil || req.LocationID == "" {
		return "", nil
	}

	limit := c.limits.SceneRecallTurns
	if limit <= 0 {
		limit = defaultSceneRecallTurns
	}
	charLimit := c.limits.SceneRecallChars
	if charLimit <= 0 {
		charLimit = defaultRecallChars
	}

	lines := make([]string, 0, limit)
	var refs []Ref
	for _, turn := range c.recalledTurns(req) {
		narrated := TruncateRunes(strings.TrimSpace(turn.Narration), charLimit)
		if narrated == "" {
			continue
		}
		refs = append(refs, Ref{Kind: RefTurn, ID: strconv.Itoa(turn.Number), Relation: "recall"})
		lines = append(lines, fmt.Sprintf("Turn %d: %s", turn.Number, narrated))
	}
	if len(lines) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("\n## WHAT HAPPENED HERE\n")
	for _, line := range lines {
		sb.WriteString(line + "\n")
	}
	return sb.String(), dedupeRefs(refs)
}

// TurnRecordView is the part of a past turn recall needs. It keeps the assembler
// from depending on the storage projection for a ranking that only reads two
// fields.
type TurnRecordView struct {
	Number    int
	Narration string
}

// rankedTurn is a recalled turn and the weight it is ranked by.
type rankedTurn struct {
	record TurnRecordView
	score  float64
}

// relevantHistory retrieves turns that share entities with the ones in play. It
// does only what scene recall cannot: it finds the turn where a promise was made or
// a secret was told, wherever it happened and however long ago.
//
// The query set excludes the player and the location. The player is mentioned by
// every turn ever recorded, so including it would rank noise first, and the
// location is what scene recall already covers.
func (c *ContextAssembler) relevantHistory(req ContextRequest) (string, []Ref) {
	window := c.windowTurns(req)
	if c.store == nil || len(window) == 0 {
		return "", nil
	}

	limit, charLimit, halfLife := c.retrievalLimits()

	query, err := c.retrievalQuery(req, turnNumbers(window))
	if err != nil || len(query) == 0 {
		return "", nil
	}

	before := req.TurnNumber
	if before <= 0 {
		before = 1 << 30
	}

	// Over-fetch, then drop what the window or scene recall already carries.
	candidates, err := c.store.TurnsMentioningEntities(query, before, limit*4)
	if err != nil {
		return "", nil
	}

	excluded := c.excludedTurns(req, window)
	ranked := rankAndTrim(c.rankTurns(candidates, excluded, query, before, halfLife), limit)
	return retrievalSection(ranked, charLimit)
}

// retrievalLimits is how much retrieval may return, how much of each turn it may quote,
// and how fast a recalled turn's weight decays with age. Each is defaulted when the
// configuration asks for nothing.
func (c *ContextAssembler) retrievalLimits() (limit, charLimit, halfLife int) {
	limit = c.limits.RetrievalTurns
	if limit <= 0 {
		limit = defaultRetrievalTurns
	}
	charLimit = c.limits.RetrievalChars
	if charLimit <= 0 {
		charLimit = defaultRecallChars
	}
	halfLife = c.limits.RetrievalHalflife
	if halfLife <= 0 {
		halfLife = defaultRetrievalHalflife
	}
	return limit, charLimit, halfLife
}

// turnNumbers is the numbers of a window of turns, which is what the store is queried
// by.
func turnNumbers(window []RecentTurn) []int {
	numbers := make([]int, 0, len(window))
	for _, turn := range window {
		numbers = append(numbers, turn.Number)
	}
	return numbers
}

// retrievalQuery is the entities whose turns are worth recalling: the characters the
// window mentions, minus the player and the location.
//
// Only characters are queried, because the location is scene recall's job and arcs are
// always in the prompt already.
func (c *ContextAssembler) retrievalQuery(req ContextRequest, numbers []int) ([]string, error) {
	mentioned, err := c.store.EntitiesInTurns(numbers)
	if err != nil {
		return nil, err
	}

	query := make([]string, 0, len(mentioned))
	for _, id := range mentioned {
		if id == req.PlayerID || id == req.LocationID {
			continue
		}
		ent, err := c.store.GetEntity(id)
		if err != nil || ent == nil || ent.Type != "character" {
			continue
		}
		query = append(query, id)
	}
	return query, nil
}

// excludedTurns is what retrieval must not return: the window it is working from, and
// everything scene recall has already put in the prompt.
func (c *ContextAssembler) excludedTurns(req ContextRequest, window []RecentTurn) map[int]bool {
	excluded := make(map[int]bool, len(window))
	for _, turn := range window {
		excluded[turn.Number] = true
	}
	for _, turn := range c.recalledTurns(req) {
		excluded[turn.Number] = true
	}
	return excluded
}

// rankTurns scores each candidate turn by how many of the query entities it names,
// decayed by how long ago it happened, and keeps only those that name any of them.
//
// Overlap is per candidate: how many of the query entities this turn names, counted once
// each. One entity can hold several mention rows, because the table's key includes the
// kind, and counting rows would weight an entity who is both wikilinked and extracted
// twice.
func (c *ContextAssembler) rankTurns(candidates []storage.TurnRecord, excluded map[int]bool, query []string, before, halfLife int) []rankedTurn {
	ranked := make([]rankedTurn, 0, len(candidates))
	for _, candidate := range candidates {
		if excluded[candidate.Number] {
			continue
		}

		mentions, err := c.store.ListEntitiesForTurn(candidate.Number)
		if err != nil {
			continue
		}
		named := make(map[string]bool, len(mentions))
		for _, mention := range mentions {
			named[mention.EntityID] = true
		}
		overlap := 0
		for _, id := range query {
			if named[id] {
				overlap++
			}
		}
		if overlap == 0 {
			continue
		}

		age := before - candidate.Number
		if age < 0 {
			age = 0
		}
		weight := math.Pow(0.5, float64(age)/float64(halfLife))
		ranked = append(ranked, rankedTurn{
			record: TurnRecordView{Number: candidate.Number, Narration: candidate.Narration},
			score:  float64(overlap) * weight,
		})
	}
	return ranked
}

// rankAndTrim puts the best-scoring recalls first, the most recent first among equals,
// and keeps no more than the limit.
func rankAndTrim(ranked []rankedTurn, limit int) []rankedTurn {
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].record.Number > ranked[j].record.Number
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked
}

// retrievalSection renders the recalled turns as the prompt's own section, with the refs
// a trace attributes them by. A turn whose narration truncates to nothing is dropped,
// because a heading with no prose under it is noise.
func retrievalSection(ranked []rankedTurn, charLimit int) (string, []Ref) {
	lines := make([]string, 0, len(ranked))
	var refs []Ref
	for _, entry := range ranked {
		narration := TruncateRunes(strings.TrimSpace(entry.record.Narration), charLimit)
		if narration == "" {
			continue
		}
		refs = append(refs, Ref{Kind: RefTurn, ID: strconv.Itoa(entry.record.Number), Relation: "retrieval"})
		lines = append(lines, fmt.Sprintf("Turn %d: %s", entry.record.Number, narration))
	}
	if len(lines) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("\n## RELEVANT HISTORY\n")
	for _, line := range lines {
		sb.WriteString(line + "\n")
	}
	return sb.String(), dedupeRefs(refs)
}

// renderedSections is what a set of sections amounts to: the whole prompt, the stable
// prefix of it with the part that may be cached behind it, and each section's cost.
type renderedSections struct {
	prompt  string
	prefix  string
	delta   string
	stats   []SectionStat
	reports []SectionReport
}

// fitToBudget drops optional sections in rank order until the prompt fits, then
// records every section's cost so a trace can explain the result.
func (c *ContextAssembler) fitToBudget(req ContextRequest, sections []section) AssembleResult {
	budget := c.limits.TokenBudget
	trimmed := c.trimToBudget(req, sections, budget)
	rendered := renderSections(sections)

	prefixHash := PrefixHash(rendered.prefix)
	result := AssembleResult{
		Prompt:          rendered.prompt,
		PrefixPrompt:    rendered.prefix,
		DeltaPrompt:     rendered.delta,
		EstimatedTokens: estimateTokens(rendered.prompt),
		Trimmed:         trimmed,
		Sections:        rendered.stats,
		Context: TurnContext{
			TurnNumber:      req.TurnNumber,
			Mode:            req.Mode,
			Budget:          c.limits.TokenBudget,
			EstimatedTokens: estimateTokens(rendered.prompt),
			Sections:        rendered.reports,
			Refs:            dedupeRefs(collectRefs(sections)),
			WorkingSet:      req.WorkingSet,
			Threads:         req.Threads,
			SummaryVersion:  req.SummaryVersion,
			PromptHash:      hashPrompt(rendered.prompt),
			PrefixHash:      prefixHash,
			Strategy:        StrategyFullPrompt,
		},
	}

	// The prompt is recorded here and nowhere else. Everything downstream refers
	// to it by hash, so the trace holds one copy rather than one per call site.
	c.logger = trace.OrNil(c.logger)
	c.logger.Event("context.assembled", map[string]any{
		"estimated_tokens": result.EstimatedTokens,
		"budget":           budget,
		"trimmed":          trimmed,
		"recall_turns":     len(c.windowTurns(req)),
		"sections":         rendered.stats,
		"prompt":           result.Prompt,
	})

	return result
}

// sectionsTokens is what a set of sections costs, which is what the budget is compared
// against.
func sectionsTokens(sections []section) int {
	sum := 0
	for _, candidate := range sections {
		sum += estimateTokens(candidate.text)
	}
	return sum
}

// trimToBudget drops droppable sections in rank order until the prompt fits the budget,
// shortening the recall window when there is nothing left that may be dropped whole. It
// reports what was dropped, in the order it went.
//
// Rank runs from least to most valuable, so rank one goes first and the search starts
// from the top after each drop: dropping a later rank can only help once the earlier one
// is gone.
func (c *ContextAssembler) trimToBudget(req ContextRequest, sections []section, budget int) []string {
	trimmed := make([]string, 0)

	for budget > 0 && sectionsTokens(sections) > budget {
		dropped := false
		for _, rank := range []int{1, 2, 3, 4, 5, 6} {
			for index := range sections {
				if sections[index].droppable && sections[index].rank == rank && sections[index].text != "" {
					sections[index].text = ""
					trimmed = append(trimmed, sectionDescription(sections[index].name))
					dropped = true
					break
				}
			}
			if dropped {
				break
			}
		}
		if dropped {
			continue
		}
		// Nothing left that may be dropped as a whole, so the recall window is
		// shortened rather than removed.
		if !c.shortenRecent(req, sections) {
			break
		}
		trimmed = append(trimmed, "shorter excerpts of recent turns")
	}

	return trimmed
}

// renderSections walks the sections in order, writing the ones that survived into the
// prompt and recording what each cost. A section that never changes between turns is
// part of the prefix, which is what the rest of the prompt is cached behind.
func renderSections(sections []section) renderedSections {
	var prompt, prefixPrompt, deltaPrompt strings.Builder
	rendered := renderedSections{
		stats:   make([]SectionStat, 0, len(sections)),
		reports: make([]SectionReport, 0, len(sections)),
	}

	for _, candidate := range sections {
		included := candidate.text != ""
		tokens := estimateTokens(candidate.text)
		if included {
			prompt.WriteString(candidate.text)
			if isPrefixSection(candidate.name) {
				prefixPrompt.WriteString(candidate.text)
			} else {
				deltaPrompt.WriteString(candidate.text)
			}
		}
		rendered.stats = append(rendered.stats, SectionStat{
			Name:     candidate.name,
			Tokens:   tokens,
			Included: included,
		})
		rendered.reports = append(rendered.reports, SectionReport{
			Name:     candidate.name,
			Tokens:   tokens,
			Included: included,
			Source:   candidate.source,
			Refs:     candidate.refs,
		})
	}

	rendered.prompt = prompt.String()
	rendered.prefix = prefixPrompt.String()
	rendered.delta = deltaPrompt.String()
	return rendered
}

func collectRefs(sections []section) []Ref {
	var all []Ref
	for _, s := range sections {
		if s.text != "" {
			all = append(all, s.refs...)
		}
	}
	return all
}

func dedupeRefs(refs []Ref) []Ref {
	seen := make(map[string]bool)
	var out []Ref
	for _, r := range refs {
		key := string(r.Kind) + ":" + r.ID + ":" + r.Relation
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

func isPrefixSection(name string) bool {
	return name == "rules" || name == "lore" || name == "instructions" || name == "catalogue"
}

func hashPrompt(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

// shortenRecent halves the excerpt cap, which is the last thing surrendered before
// recall disappears entirely.
func (c *ContextAssembler) shortenRecent(req ContextRequest, sections []section) bool {
	for index := range sections {
		if sections[index].name != "recent" || sections[index].text == "" {
			continue
		}

		limit := c.limits.RecentTurnChars
		if limit <= 0 {
			limit = defaultRecentTurnChars
		}
		if limit <= minRecentTurnChars {
			sections[index].text = ""
			return true
		}

		limit /= 2
		if limit < minRecentTurnChars {
			limit = minRecentTurnChars
		}
		c.limits.RecentTurnChars = limit
		sections[index].text, sections[index].refs = c.recentSection(req)
		return true
	}
	return false
}

func sectionDescription(name string) string {
	switch name {
	case "catalogue":
		return "the voice profile catalogue"
	case "retrieval":
		return "relevant history"
	case "recall":
		return "what happened here"
	case "summary":
		return "the story so far"
	case "recent":
		return "all recent events"
	case "working_set":
		return "the active continuity working set"
	default:
		return name
	}
}

// FormatVoiceProfilesCatalog describes the voices available for NPCs. It asks for
// traits rather than profile IDs: voice assignment is the extractor's job, and an
// ID written into prose is a leak the player would see.
func FormatVoiceProfilesCatalog(profiles []config.VoiceProfile) string {
	if len(profiles) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n## AVAILABLE NPC VOICE PROFILES\n")
	sb.WriteString("New characters are voiced automatically. Describe an NPC's manner of speech\n")
	sb.WriteString("(age, temperament, accent, timbre) when you introduce them; never write voice\n")
	sb.WriteString("IDs, tags, or profile names into the narration.\n")
	for _, p := range profiles {
		sb.WriteString(fmt.Sprintf("- `%s`: %s\n", p.ID, p.Description))
	}
	return sb.String()
}

// FormatSpeechFormattingInstructions builds the speech and prose formatting prompt
// tailored to the active engine's speech steering capabilities.
func FormatSpeechFormattingInstructions(cues SpeechCueContext) string {
	var sb strings.Builder
	sb.WriteString("## SPEECH FORMATTING\n")
	sb.WriteString("Write each spoken line on its own line as a blockquote, formatted as:\n")
	sb.WriteString("> Name: \"the words spoken\"\n")
	sb.WriteString("Every spoken line, dialogue beat, disembodied voice, or unknown speaker MUST start with '> '.\n")
	sb.WriteString("Use a character's established name, or [[their note name]] to link them.\n")
	sb.WriteString("Keep narration on its own lines with no leading '> ' or name. If a speaker is unknown\n")
	sb.WriteString("or cannot be named, format with a blockquote (e.g. '> Unknown Voice: \"Who goes there?\"')\n")
	sb.WriteString("or leave the words in the narration instead of inventing a character.\n\n")

	sb.WriteString("## PROSE FORMATTING\n")
	sb.WriteString("Separate narration beats with blank lines, one beat per paragraph.\n")
	sb.WriteString("Use plain prose. Do not emit headings, tables, or code fences in narration.\n")
	if cues.MarkdownEmphasis {
		sb.WriteString("You may use *single asterisks* for vocal emphasis and --- for a scene break.\n\n")
	} else {
		sb.WriteString("You may use *single asterisks* for emphasis and --- for a scene break.\n\n")
	}

	if cues.AudioTags {
		sb.WriteString("## VOICE ACTING & SPEECH STEERING\n")
		sb.WriteString("You may steer the vocal delivery of spoken lines and narration beats using bracketed\n")
		sb.WriteString("performance tags immediately before dialogue or delivery. Common supported cues:\n")
		if len(cues.SampleTags) > 0 {
			sb.WriteString("- Supported cues: " + strings.Join(cues.SampleTags, ", ") + "\n")
		} else {
			sb.WriteString("- Delivery/Volume: `[whispers]`, `[softly]`, `[shouts]`, `[loudly]`\n")
			sb.WriteString("- Reactions: `[sighs]`, `[laughs]`, `[chuckles]`, `[gasp]`, `[clears throat]`\n")
			sb.WriteString("- Moods: `[excited]`, `[angry]`, `[sad]`, `[nervous]`, `[playful]`, `[tired]`\n")
		}
		sb.WriteString("Example: > Garrick: \"[whispers] Keep your head down.\"\n")
		sb.WriteString("Example: [sighs] It has been a long winter in the northern reaches.\n")
		sb.WriteString("Use cues purposefully to enhance drama; do not clutter every sentence.\n\n")
	}

	sb.WriteString("## CONTINUITY\n")
	sb.WriteString("Never rename a character who has already appeared. Once someone is introduced,\n")
	sb.WriteString("reuse exactly the same name, and link them with [[that name]] every time.\n")
	sb.WriteString("Continue the conversation the player is having; do not restart the scene.\n")
	if cues.AudioTags {
		sb.WriteString("Do not write voice IDs or profile names into the narration.")
	} else {
		sb.WriteString("Do not write stage directions, voice tags, brackets, or profile names into the narration or dialogue (e.g. do not write [whispers]), as the voice synthesizer will mispronounce them.")
	}
	return sb.String()
}

// formatRecentTurns renders the tail of the timeline for the narrator. It is a
// transcript rather than a summary, because summarising is what loses the details
// a player expects the GM to remember.
func formatRecentTurns(turns []RecentTurn, charLimit int) string {
	if len(turns) == 0 || charLimit <= 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## RECENT EVENTS (oldest first, most recent last)\n")
	for _, turn := range turns {
		if input := strings.TrimSpace(turn.Input); input != "" {
			fmt.Fprintf(&sb, "Turn %d - Player [%s]: %s\n", turn.Number, turn.Mode, TruncateRunes(input, charLimit))
		} else {
			fmt.Fprintf(&sb, "Turn %d - [%s]\n", turn.Number, turn.Mode)
		}
		if narration := strings.TrimSpace(turn.Narration); narration != "" {
			fmt.Fprintf(&sb, "Narrator: %s\n\n", TruncateRunes(narration, charLimit))
		}
	}
	return sb.String()
}

// TruncateRunes shortens text to a rune count, marking that it was cut.
func TruncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}

// estimateTokens converts text to a rough token count.
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return len([]rune(text))/runesPerToken + 1
}

// RenderState prints an entity's state in a stable order, so the same note always
// produces the same prompt. It is exported because the continuity checks compare
// narration against the same rendering.
func RenderState(raw map[string]any) string {
	if len(raw) == 0 {
		return ""
	}

	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, raw[key]))
	}
	return strings.Join(parts, ", ")
}

// canonEntity renders one entity as a fact line: its name, its type, its prose, and
// the state the engine is tracking. Without the state, a guttering brazier or a
// dwindling reserve is invisible to the narrator.
func canonEntity(ent *entity.Entity) string {
	line := fmt.Sprintf("- **%s** (%s): %s", ent.Name, ent.Type, strings.TrimSpace(ent.Body))
	if ent.State != nil {
		if rendered := RenderState(ent.State.Raw()); rendered != "" {
			line += "\n  State: " + rendered
		}
	}
	return line
}

// establishedNames lists the names in play, so the model reuses them instead of
// inventing new ones for beings it has already met. It covers what is present and
// what the recall window mentions, and nothing else: the wider cast belongs to a
// summary, because a roster of every note would be thousands of characters that
// could never be trimmed.
func (c *ContextAssembler) establishedNames(req ContextRequest) string {
	seen := make(map[string]bool)
	names := make([]string, 0)

	add := func(id string) {
		if id == "" || id == req.PlayerID || seen[id] {
			return
		}
		ent, err := c.store.GetEntity(id)
		if err != nil || ent == nil || ent.Name == "" {
			return
		}
		switch ent.Type {
		case "character", "location", "arc":
		default:
			return
		}
		seen[id] = true
		if len(ent.Aliases) > 0 {
			names = append(names, fmt.Sprintf("%s (also known as %s)", ent.Name, strings.Join(ent.Aliases, ", ")))
			return
		}
		names = append(names, ent.Name)
	}

	if edges, err := c.store.GetEdgesFrom(req.LocationID); err == nil {
		for _, edge := range edges {
			add(edge.TargetID)
		}
	}
	if len(req.Recent) > 0 {
		numbers := make([]int, 0, len(req.Recent))
		for _, turn := range req.Recent {
			numbers = append(numbers, turn.Number)
		}
		if ids, err := c.store.EntitiesInTurns(numbers); err == nil {
			for _, id := range ids {
				add(id)
			}
		}
	}

	if len(names) == 0 {
		return ""
	}
	sort.SliceStable(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	var sb strings.Builder
	sb.WriteString("## ESTABLISHED NAMES\n")
	sb.WriteString("Reuse these names exactly; never rename a being who has already appeared.\n")
	for _, name := range names {
		sb.WriteString("- " + name + "\n")
	}
	return sb.String()
}
