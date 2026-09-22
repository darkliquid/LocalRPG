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

// AssembleResult is a prompt plus a note of what was left out, so a caller can
// report trimming rather than losing context silently.
type AssembleResult struct {
	Prompt          string
	EstimatedTokens int
	Trimmed         []string
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

// SetLogger attaches a trace sink. A nil logger records nothing.
func (c *ContextAssembler) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

func (c *ContextAssembler) AssembleContext(locationID, playerID, playerAction string) (string, error) {
	var sb strings.Builder

	// Layer 1: Immediate Scene Scope
	sb.WriteString("## IMMEDIATE SCENE\n")
	if loc, err := c.store.GetEntity(locationID); err == nil && loc != nil {
		sb.WriteString(fmt.Sprintf("**Current Location:** %s\n%s\n\n", loc.Name, loc.Body))
	}

	if player, err := c.store.GetEntity(playerID); err == nil && player != nil {
		sb.WriteString(fmt.Sprintf("**Player Character:** %s\n", player.Name))
		if player.State != nil {
			sb.WriteString(fmt.Sprintf("State: %+v\n\n", player.State.Raw()))
		}
	}

	// Layer 2: Living World Arcs & Background Agendas
	sb.WriteString("## LIVING WORLD & BACKGROUND ARCS\n")
	edges, err := c.store.GetEdgesFrom(locationID)
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "arc" {
				sb.WriteString(fmt.Sprintf("### Arc: %s\n%s\n\n", ent.Name, ent.Body))
			}
		}
	}

	// Layer 3: Present Actors
	sb.WriteString("## PRESENT CHARACTERS & NOTABLE BEINGS\n")
	if err == nil {
		for _, edge := range edges {
			if ent, err := c.store.GetEntity(edge.TargetID); err == nil && ent != nil && ent.Type == "character" {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", ent.Name, ent.Body))
			}
		}
	}

	// Layer 4: Player Action
	sb.WriteString("\n## PLAYER ACTION\n")
	sb.WriteString(playerAction + "\n")

	return sb.String(), nil
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

func (c *ContextAssembler) AssembleContextWithRules(locationID, playerID, playerAction, rulesPrompt, lorePrompt string) (string, error) {
	result, err := c.AssembleContextWithProfiles(locationID, playerID, playerAction, rulesPrompt, lorePrompt, nil, nil)
	if err != nil {
		return "", err
	}
	return result.Prompt, nil
}

// AssembleContextWithProfiles builds the prompt and trims it to the configured
// budget. Trimming is ordered by what the narrator can best do without: the voice
// catalogue, then the oldest recalled turns, then recall entirely. Rules, lore,
// the scene, and the player's action are never dropped, because a prompt without
// them is not a smaller prompt, it is a broken one.
func (c *ContextAssembler) AssembleContextWithProfiles(locationID, playerID, playerAction, rulesPrompt, lorePrompt string, profiles []config.VoiceProfile, recent []RecentTurn) (AssembleResult, error) {
	base, err := c.AssembleContext(locationID, playerID, playerAction)
	if err != nil {
		return AssembleResult{}, err
	}

	var head strings.Builder
	if strings.TrimSpace(rulesPrompt) != "" {
		head.WriteString("## SYSTEM RULES & RESOLUTION MECHANICS\n")
		head.WriteString(strings.TrimSpace(rulesPrompt) + "\n\n")
	}
	if strings.TrimSpace(lorePrompt) != "" {
		head.WriteString("## WORLD LORE & ATMOSPHERE\n")
		head.WriteString(strings.TrimSpace(lorePrompt) + "\n\n")
	}
	head.WriteString(speechFormattingInstruction + "\n\n")

	catalog := ""
	if len(profiles) > 0 {
		catalog = FormatVoiceProfilesCatalog(profiles) + "\n"
	}

	window := c.limits.RecentTurns
	if window <= 0 {
		window = defaultRecentTurns
	}
	charLimit := c.limits.RecentTurnChars
	if charLimit <= 0 {
		charLimit = defaultRecentTurnChars
	}

	turns := recent
	if len(turns) > window {
		turns = turns[len(turns)-window:]
	}
	recall := formatRecentTurns(turns, charLimit)

	trimmed := make([]string, 0)
	budget := c.limits.TokenBudget
	overBudget := func() bool {
		return budget > 0 && estimateTokens(head.String()+catalog+recall+base) > budget
	}

	if overBudget() && catalog != "" {
		catalog = ""
		trimmed = append(trimmed, "the voice profile catalogue")
	}

	for overBudget() && len(turns) > 0 {
		turns = turns[1:]
		recall = formatRecentTurns(turns, charLimit)
		trimmed = append(trimmed, "the oldest remembered turn")
	}

	if overBudget() && recall != "" && charLimit > minRecentTurnChars {
		charLimit = minRecentTurnChars
		recall = formatRecentTurns(turns, charLimit)
		trimmed = append(trimmed, "shorter excerpts of recent turns")
	}

	if overBudget() && recall != "" {
		recall = ""
		trimmed = append(trimmed, "all recent events")
	}

	prompt := head.String() + catalog + recall + base
	result := AssembleResult{
		Prompt:          prompt,
		EstimatedTokens: estimateTokens(prompt),
		Trimmed:         trimmed,
	}

	// The prompt is recorded here and nowhere else. Everything downstream refers
	// to it by hash, so the trace holds one copy rather than one per call site.
	c.logger = trace.OrNil(c.logger)
	c.logger.Event("context.assembled", map[string]interface{}{
		"estimated_tokens": result.EstimatedTokens,
		"budget":           c.limits.TokenBudget,
		"trimmed":          trimmed,
		"recall_turns":     len(turns),
		"prompt":           prompt,
	})

	return result, nil
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
