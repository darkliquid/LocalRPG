package harness

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type ContextAssembler struct {
	store *storage.Store
}

func NewContextAssembler(store *storage.Store) *ContextAssembler {
	return &ContextAssembler{store: store}
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

func FormatVoiceProfilesCatalog(profiles []config.VoiceProfile) string {
	if len(profiles) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n## AVAILABLE NPC VOICE PROFILES\n")
	sb.WriteString("When introducing or speaking as new NPCs, assign an appropriate voice profile ID in their description:\n")
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
	return c.AssembleContextWithProfiles(locationID, playerID, playerAction, rulesPrompt, lorePrompt, nil, "")
}

func (c *ContextAssembler) AssembleContextWithProfiles(locationID, playerID, playerAction, rulesPrompt, lorePrompt string, profiles []config.VoiceProfile, recentEvents string) (string, error) {
	var sb strings.Builder

	if strings.TrimSpace(rulesPrompt) != "" {
		sb.WriteString("## SYSTEM RULES & RESOLUTION MECHANICS\n")
		sb.WriteString(strings.TrimSpace(rulesPrompt) + "\n\n")
	}

	if strings.TrimSpace(lorePrompt) != "" {
		sb.WriteString("## WORLD LORE & ATMOSPHERE\n")
		sb.WriteString(strings.TrimSpace(lorePrompt) + "\n\n")
	}

	sb.WriteString(speechFormattingInstruction + "\n\n")

	if len(profiles) > 0 {
		sb.WriteString(FormatVoiceProfilesCatalog(profiles) + "\n")
	}

	// Prior turns are what give the narrator a memory. Without them every call is
	// a cold start and the model reintroduces the same scene from scratch.
	if strings.TrimSpace(recentEvents) != "" {
		sb.WriteString("\n## RECENT EVENTS (oldest first, most recent last)\n")
		sb.WriteString(strings.TrimSpace(recentEvents) + "\n")
	}

	baseContext, err := c.AssembleContext(locationID, playerID, playerAction)
	if err != nil {
		return "", err
	}
	sb.WriteString(baseContext)
	return sb.String(), nil
}
