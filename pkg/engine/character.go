package engine

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
)

// PlayerCharacter is the protagonist's authored description, gathered during
// campaign creation. It is deliberately open-ended: Extra carries whatever
// prompts the rules system defines beyond the common ones.
type PlayerCharacter struct {
	Appearance string
	Age        string
	Gender     string
	Pronouns   string
	Background string
	Voice      *entity.VoiceConfig
	// Portrait is the character's portrait path relative to the game directory,
	// carried so recreating a campaign does not silently drop the picture.
	Portrait string
	Extra    map[string]string
}

// PlayerCharacterFromEntity recovers the authored character fields from an
// existing protagonist note, so a campaign can be recreated without retyping the
// sheet. It is the inverse of what ensurePlayerNote writes: descriptive frontmatter
// survives, while state and history are deliberately left behind because a restart
// resets them.
func PlayerCharacterFromEntity(ent *entity.Entity) PlayerCharacter {
	pc := PlayerCharacter{}
	if ent == nil {
		return pc
	}

	pc.Appearance = ent.Appearance
	pc.Age = ent.Age
	pc.Gender = ent.Gender
	pc.Voice = ent.Voice
	pc.Portrait = ent.Portrait

	for key, value := range ent.ExtraMeta {
		if key == "pronouns" {
			if s, ok := value.(string); ok {
				pc.Pronouns = s
			}
			continue
		}
		if value == nil {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(value))
		if s == "" {
			continue
		}
		if pc.Extra == nil {
			pc.Extra = map[string]string{}
		}
		pc.Extra[key] = s
	}

	return pc
}

// DefaultCharacterFields is the fallback creation spec for a system that defines
// none. The common prompts cover a usable character without any system authoring.
func DefaultCharacterFields() []core.CharacterCreationField {
	return []core.CharacterCreationField{
		{ID: "appearance", Label: "Appearance", Prompt: "How does your character look?", Kind: "long", Required: true, Generatable: true},
		{ID: "age", Label: "Age", Kind: "text", Generatable: true},
		{ID: "gender", Label: "Gender", Kind: "text", Generatable: true},
		{ID: "pronouns", Label: "Pronouns", Kind: "text"},
		{ID: "background", Label: "Background", Prompt: "Where do they come from?", Kind: "long", Generatable: true},
		{ID: "voice", Label: "Voice", Kind: "voice"},
	}
}

// CharacterFields returns a system's creation fields, or the default set when the
// system defines none.
func CharacterFields(manifest *core.SystemManifest) []core.CharacterCreationField {
	if manifest != nil && len(manifest.CharacterCreation.Fields) > 0 {
		return manifest.CharacterCreation.Fields
	}
	return DefaultCharacterFields()
}

// RequiredCharacterFields returns the ids of the fields a creation cannot omit.
func RequiredCharacterFields(fields []core.CharacterCreationField) []string {
	required := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.Required && field.Kind != "voice" {
			required = append(required, field.ID)
		}
	}
	return required
}
