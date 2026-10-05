package gui

import "context"

// EntityTypeSpec describes one offered entity type: how the picker presents it,
// the aliases the engine also recognises, and the frontmatter keys a new note of
// this type is scaffolded with.
type EntityTypeSpec struct {
	ID          string                 `json:"id"`
	Label       string                 `json:"label"`
	Description string                 `json:"description"`
	Aliases     []string               `json:"aliases,omitempty"`
	Keys        []FrontmatterKeySchema `json:"keys"`
}

// EntityTypeCatalog is the served description of the offered entity types.
// BaseKeys is what an unknown type falls back to, so a note created for a type
// the catalogue does not know still parses.
type EntityTypeCatalog struct {
	BaseKeys []FrontmatterKeySchema `json:"base_keys"`
	Types    []EntityTypeSpec       `json:"types"`
}

// entityTypeDef is one catalogue entry before its key names are resolved against
// the frontmatter schema.
type entityTypeDef struct {
	id          string
	label       string
	description string
	aliases     []string
	keys        []string
}

// entityKeyHead and entityKeyTail bracket the per-type extras, so a type's key
// order reads the same way in every note and no type can forget a base key.
var (
	entityKeyHead = []string{"id", "name", "type"}
	entityKeyTail = []string{"tags", "aliases", "location", "faction", "state"}
)

// entityKeys builds one type's ordered key list: the head, the type's own extras,
// then the tail.
func entityKeys(extras ...string) []string {
	keys := make([]string, 0, len(entityKeyHead)+len(extras)+len(entityKeyTail))
	keys = append(keys, entityKeyHead...)
	keys = append(keys, extras...)
	keys = append(keys, entityKeyTail...)
	return keys
}

// entityTypeDefs is the canonical list of offered entity types, in display order.
// npc, person and creature stay accepted by entity.IsCharacterType but are
// aliases of character rather than separate choices. appearance is offered for
// every type that can be depicted or referenced visually.
var entityTypeDefs = []entityTypeDef{
	{
		id:          "character",
		label:       "Character",
		description: "Player characters, NPCs, companions and adversaries.",
		aliases:     []string{"npc", "person", "creature"},
		keys:        entityKeys("appearance", "gender", "age", "voice"),
	},
	{
		id:          "location",
		label:       "Location",
		description: "Towns, rooms, regions and any other place a scene happens in.",
		keys:        entityKeys("appearance"),
	},
	{
		id:          "faction",
		label:       "Faction",
		description: "Guilds, orders, crews and governments the note belongs to or leads.",
		keys:        entityKeys("appearance"),
	},
	{
		id:          "item",
		label:       "Item",
		description: "Relics, weapons, tools and other things a character can carry.",
		keys:        entityKeys("appearance"),
	},
	{
		id:          "concept",
		label:       "Concept",
		description: "An idea, custom, deity or force the world is built around.",
		keys:        entityKeys(),
	},
	{
		id:          "arc",
		label:       "Narrative Arc",
		description: "A running storyline or threat, tracked as a progress clock.",
		keys:        entityKeys(),
	},
	{
		id:          "event",
		label:       "Event",
		description: "Something that happened, or is scheduled to happen.",
		keys:        entityKeys(),
	},
	{
		id:          "quest",
		label:       "Quest",
		description: "An objective the player can pursue, with a state to track it.",
		keys:        entityKeys(),
	},
	{
		id:          "lore",
		label:       "Lore",
		description: "Background history or a piece of world knowledge.",
		keys:        entityKeys(),
	},
}

// entityTypeIDs lists the catalogue's type ids in display order. It is the source
// for the frontmatter schema's type suggestions.
func entityTypeIDs() []string {
	ids := make([]string, 0, len(entityTypeDefs))
	for _, def := range entityTypeDefs {
		ids = append(ids, def.id)
	}
	return ids
}

// buildEntityTypeCatalog resolves each type's key names against the frontmatter
// schema, so a catalogue key and an editor completion describe a key identically.
func buildEntityTypeCatalog() EntityTypeCatalog {
	byName := map[string]FrontmatterKeySchema{}
	for _, key := range entityFrontmatterKeys() {
		byName[key.Name] = key
	}

	types := make([]EntityTypeSpec, 0, len(entityTypeDefs))
	for _, def := range entityTypeDefs {
		keys := make([]FrontmatterKeySchema, 0, len(def.keys))
		for _, name := range def.keys {
			keys = append(keys, byName[name])
		}
		types = append(types, EntityTypeSpec{
			ID:          def.id,
			Label:       def.label,
			Description: def.description,
			Aliases:     def.aliases,
			Keys:        keys,
		})
	}

	base := make([]FrontmatterKeySchema, 0, len(entityKeyHead)+len(entityKeyTail))
	for _, name := range entityKeys() {
		base = append(base, byName[name])
	}

	return EntityTypeCatalog{BaseKeys: base, Types: types}
}

// GetEntityTypeCatalog serves the offered entity types and the frontmatter keys
// each one scaffolds with.
func (s *Service) GetEntityTypeCatalog(_ context.Context) (EntityTypeCatalog, error) {
	return buildEntityTypeCatalog(), nil
}
