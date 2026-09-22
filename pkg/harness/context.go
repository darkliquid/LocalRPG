package harness

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/storage"
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
}

const (
	defaultRecentTurns     = 6
	defaultRecentTurnChars = 1200
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

// ContextRequest is everything one assembly needs. It replaced a positional
// parameter list because recall needs the turn number, and a summary and more will
// follow, at which point the list stops being readable.
type ContextRequest struct {
	LocationID  string
	PlayerID    string
	Action      string
	RulesPrompt string
	LorePrompt  string
	Profiles    []config.VoiceProfile
	Recent      []RecentTurn
	TurnNumber  int
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
	EstimatedTokens int
	Trimmed         []string
	Sections        []SectionStat
}

// section is one block of the prompt. Its place in the slice is its place in the
// prompt; rank is the order it is surrendered when the budget bites, lowest first.
type section struct {
	name      string
	text      string
	droppable bool
	rank      int
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
	sections, err := c.buildSections(req)
	if err != nil {
		return AssembleResult{}, err
	}
	return c.fitToBudget(req, sections), nil
}

// buildSections composes the prompt in order. Everything a section needs is read
// here, so trimming never re-reads the store.
func (c *ContextAssembler) buildSections(req ContextRequest) ([]section, error) {
	canon, err := c.assembleCanon(req)
	if err != nil {
		return nil, err
	}

	catalogue := ""
	if len(req.Profiles) > 0 {
		catalogue = FormatVoiceProfilesCatalog(req.Profiles) + "\n"
	}

	return []section{
		{name: "rules", text: rulesSection(req.RulesPrompt)},
		{name: "lore", text: loreSection(req.LorePrompt)},
		{name: "instructions", text: speechFormattingInstruction + "\n\n"},
		{name: "canon", text: canon},
		{name: "recent", text: c.recentSection(req), droppable: true, rank: 4},
		{name: "catalogue", text: catalogue, droppable: true, rank: 1},
		{name: "action", text: "\n## PLAYER ACTION\n" + req.Action + "\n"},
	}, nil
}

func rulesSection(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	return "## SYSTEM RULES & RESOLUTION MECHANICS\n" + strings.TrimSpace(prompt) + "\n\n"
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
func (c *ContextAssembler) assembleCanon(req ContextRequest) (string, error) {
	var sb strings.Builder

	sb.WriteString("## IMMEDIATE SCENE\n")
	if loc, err := c.store.GetEntity(req.LocationID); err == nil && loc != nil {
		sb.WriteString(fmt.Sprintf("**Current Location:** %s\n%s\n\n", loc.Name, loc.Body))
	}

	if player, err := c.store.GetEntity(req.PlayerID); err == nil && player != nil {
		sb.WriteString(fmt.Sprintf("**Player Character:** %s\n", player.Name))
		if player.State != nil {
			sb.WriteString(fmt.Sprintf("State: %+v\n\n", player.State.Raw()))
		}
	}

	sb.WriteString("## LIVING WORLD & BACKGROUND ARCS\n")
	edges, err := c.store.GetEdgesFrom(req.LocationID)
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "arc" {
				sb.WriteString(fmt.Sprintf("### Arc: %s\n%s\n\n", ent.Name, ent.Body))
			}
		}
	}

	sb.WriteString("## PRESENT CHARACTERS & NOTABLE BEINGS\n")
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "character" {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", ent.Name, ent.Body))
			}
		}
	}

	return sb.String(), nil
}

// recentSection renders the tail of the timeline, bounded by the configured window
// and excerpt length.
func (c *ContextAssembler) recentSection(req ContextRequest) string {
	window := c.limits.RecentTurns
	if window <= 0 {
		window = defaultRecentTurns
	}
	charLimit := c.limits.RecentTurnChars
	if charLimit <= 0 {
		charLimit = defaultRecentTurnChars
	}

	turns := req.Recent
	if len(turns) > window {
		turns = turns[len(turns)-window:]
	}
	return formatRecentTurns(turns, charLimit)
}

// fitToBudget drops optional sections in rank order until the prompt fits, then
// records every section's cost so a trace can explain the result.
func (c *ContextAssembler) fitToBudget(req ContextRequest, sections []section) AssembleResult {
	trimmed := make([]string, 0)
	budget := c.limits.TokenBudget

	total := func() int {
		sum := 0
		for _, candidate := range sections {
			sum += estimateTokens(candidate.text)
		}
		return sum
	}

	for budget > 0 && total() > budget {
		dropped := false
		for _, rank := range []int{1, 2, 3, 4} {
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

	var prompt strings.Builder
	stats := make([]SectionStat, 0, len(sections))
	for _, candidate := range sections {
		if candidate.text != "" {
			prompt.WriteString(candidate.text)
		}
		stats = append(stats, SectionStat{
			Name:     candidate.name,
			Tokens:   estimateTokens(candidate.text),
			Included: candidate.text != "",
		})
	}

	result := AssembleResult{
		Prompt:          prompt.String(),
		EstimatedTokens: estimateTokens(prompt.String()),
		Trimmed:         trimmed,
		Sections:        stats,
	}

	// The prompt is recorded here and nowhere else. Everything downstream refers
	// to it by hash, so the trace holds one copy rather than one per call site.
	c.logger = trace.OrNil(c.logger)
	c.logger.Event("context.assembled", map[string]interface{}{
		"estimated_tokens": result.EstimatedTokens,
		"budget":           budget,
		"trimmed":          trimmed,
		"recall_turns":     len(req.Recent),
		"sections":         stats,
		"prompt":           result.Prompt,
	})

	return result
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
		sections[index].text = c.recentSection(req)
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
	case "recent":
		return "all recent events"
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

// speechFormattingInstruction is injected for every game. The parser needs one
// shape it can resolve deterministically, and the last line tells the model what to
// do when it cannot name a speaker, which is where attribution usually fails.
const speechFormattingInstruction = `## SPEECH FORMATTING
Write each spoken line on its own line, formatted as  Name: "the words spoken"
Use a character's established name, or [[their note name]] to link them.
Keep narration on its own lines with no leading name. If you cannot name the
speaker, leave the words in the narration instead of inventing a name.

## PROSE FORMATTING
Separate narration beats with blank lines, one beat per paragraph.
Use plain prose. Do not emit headings, tables, or code fences in narration.
You may use *single asterisks* for emphasis and --- for a scene break.

## CONTINUITY
Never rename a character who has already appeared. Once someone is introduced,
reuse exactly the same name, and link them with [[that name]] every time.
Continue the conversation the player is having; do not restart the scene.
Do not write voice IDs, voice tags, or profile names into the narration.`

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
