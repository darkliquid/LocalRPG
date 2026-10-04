package gui

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

//go:embed schema/entity-frontmatter.json
var entityFrontmatterSchemaJSON []byte

// schemaEntry is one key and its type, as the configuration reference lists them.
type schemaEntry struct {
	key string
	typ string
}

func walkSchema(prefix string, t reflect.Type) []schemaEntry {
	return walkSchemaInto(prefix, t, make(map[reflect.Type]bool))
}

func walkSchemaInto(prefix string, t reflect.Type, seen map[reflect.Type]bool) []schemaEntry {
	t = derefType(t)
	switch t.Kind() {
	case reflect.Struct:
		if seen[t] {
			return nil
		}
		seen[t] = true
		var out []schemaEntry
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := yamlFieldName(field)
			if name == "" || name == "-" {
				continue
			}
			out = append(out, walkSchemaInto(prefix+"."+name, field.Type, seen)...)
		}
		return out
	case reflect.Slice, reflect.Array:
		elem := derefType(t.Elem())
		if elem.Kind() == reflect.Struct {
			return walkSchemaInto(prefix+"[]", elem, seen)
		}
		return []schemaEntry{{key: prefix, typ: "[]" + typeLabel(elem)}}
	case reflect.Map:
		value := derefType(t.Elem())
		if value.Kind() == reflect.Struct {
			return walkSchemaInto(prefix+".<key>", value, seen)
		}
		if t.Key().Kind() == reflect.String {
			return []schemaEntry{{key: prefix, typ: fmt.Sprintf("map<string, %s>", typeLabel(value))}}
		}
		return []schemaEntry{{key: prefix, typ: "map"}}
	case reflect.Interface:
		return []schemaEntry{{key: prefix, typ: "any"}}
	default:
		return []schemaEntry{{key: prefix, typ: typeLabel(t)}}
	}
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func typeLabel(t reflect.Type) string {
	t = derefType(t)
	switch t.Kind() {
	case reflect.Interface:
		return "any"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Struct:
		return t.Name()
	default:
		return t.Kind().String()
	}
}

func yamlFieldName(field reflect.StructField) string {
	name := field.Tag.Get("yaml")
	if idx := strings.Index(name, ","); idx >= 0 {
		name = name[:idx]
	}
	return strings.TrimSpace(name)
}

// FrontmatterKeySchema describes one accepted frontmatter key.
type FrontmatterKeySchema struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Values      []string `json:"values,omitempty"`
	Description string   `json:"description"`
}

// FrontmatterSchema is the generated description of an entity note's frontmatter.
// AllowUnknown is true because EntityFrontmatter has an inline catch-all: the
// engine is schema-agnostic, so the schema describes the accepted keys rather than
// gating them.
type FrontmatterSchema struct {
	AllowUnknown bool                   `json:"allowUnknown"`
	Keys         []FrontmatterKeySchema `json:"keys"`
}

// EntityFrontmatterSchema returns the generated schema the editor completes from.
func EntityFrontmatterSchema() FrontmatterSchema {
	var schema FrontmatterSchema
	if err := json.Unmarshal(entityFrontmatterSchemaJSON, &schema); err != nil {
		// A schema that will not parse is not worth failing an editor over: the
		// document still edits, only without completion.
		return FrontmatterSchema{AllowUnknown: true}
	}
	return schema
}

// frontmatterKeyDescriptions documents each accepted key. A test asserts every
// field of EntityFrontmatter has an entry, so a new field cannot ship
// undocumented.
var frontmatterKeyDescriptions = map[string]string{
	"id":               "The note's stable identity. It is unique per collection and does not change when the note moves between folders.",
	"name":             "The display name shown in the codex and used when the engine matches a mention.",
	"type":             "What kind of thing the note is. Types that read as a being (character, npc, person, creature) are the only ones the engine treats specially.",
	"tags":             "Free-form labels for grouping and search.",
	"voice":            "The text-to-speech voice this note speaks with, including pitch and speech rate.",
	"portrait":         "The path of the note's portrait image, when it has one.",
	"portrait_version": "A counter bumped each time the portrait is regenerated, so the client can bust its image cache.",
	"portrait_history": "Previous portrait paths, kept so an earlier portrait can be restored.",
	"location":         "Where the note is. May contain a [[wikilink]] to a location note.",
	"appearance":       "A physical description used when generating art or describing the note.",
	"gender":           "The note's gender, as free text.",
	"age":              "The note's age, as free text, because a campaign may count in years, seasons or reigns.",
	"aliases":          "Other names the same thing is known by. They are matched when resolving mentions and when completing wikilinks.",
	"faction":          "The group the note belongs to. May contain a [[wikilink]] to a faction note.",
	"history":          "The turn numbers this note took part in. Written by the engine, not by hand.",
	"state":            "Arbitrary per-note state the mechanics hooks read and write. The engine does not interpret the keys.",
}

// frontmatterKeyValues lists the values worth suggesting for the keys whose
// domain is small. They are suggestions, not a closed set: only the being-like
// types change engine behaviour, and any other type string is accepted.
var frontmatterKeyValues = map[string][]string{
	"type": {"character", "location", "faction", "item", "event", "quest", "lore"},
}

// renderEntityFrontmatterSchema reflects over the entity frontmatter so the keys
// the editor offers cannot drift from the keys the loader accepts.
func renderEntityFrontmatterSchema() string {
	t := reflect.TypeOf(entity.EntityFrontmatter{})
	keys := make([]FrontmatterKeySchema, 0, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name := yamlFieldName(field)
		if name == "" || name == "-" {
			continue
		}
		keys = append(keys, FrontmatterKeySchema{
			Name:        name,
			Type:        frontmatterTypeLabel(field.Type),
			Required:    !strings.Contains(field.Tag.Get("yaml"), "omitempty"),
			Values:      frontmatterKeyValues[name],
			Description: frontmatterKeyDescriptions[name],
		})
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].Name < keys[j].Name })

	encoded, err := json.MarshalIndent(FrontmatterSchema{AllowUnknown: true, Keys: keys}, "", "  ")
	if err != nil {
		return ""
	}
	return string(encoded) + "\n"
}

// frontmatterTypeLabel names a field's type for a reader: "[]string" for a list,
// "map<string, any>" for a free-form map, and the struct name for a nested one.
func frontmatterTypeLabel(t reflect.Type) string {
	base := derefType(t)
	switch base.Kind() {
	case reflect.Slice:
		return "[]" + typeLabel(base.Elem())
	case reflect.Map:
		return fmt.Sprintf("map<%s, %s>", typeLabel(base.Key()), typeLabel(base.Elem()))
	default:
		return typeLabel(base)
	}
}
