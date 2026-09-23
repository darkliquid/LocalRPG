package entity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/state"
	"gopkg.in/yaml.v3"
)

var wikilinkRegex = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|[^\]]+)?\]\]`)

type VoiceConfig struct {
	Provider   string  `yaml:"provider,omitempty" json:"provider,omitempty"`
	VoiceID    string  `yaml:"voice_id,omitempty" json:"voice_id,omitempty"`
	Pitch      float64 `yaml:"pitch,omitempty" json:"pitch,omitempty"`
	SpeechRate float64 `yaml:"speech_rate,omitempty" json:"speech_rate,omitempty"`
	// Options carries provider-declared tunables for this voice, keyed by the
	// provider's VoiceOption.Key. Absent means the provider's own defaults.
	Options map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
}

type EntityFrontmatter struct {
	ID         string                 `yaml:"id"`
	Name       string                 `yaml:"name"`
	Type       string                 `yaml:"type"`
	Tags       []string               `yaml:"tags,omitempty"`
	Voice      *VoiceConfig           `yaml:"voice,omitempty"`
	Portrait   string                 `yaml:"portrait,omitempty"`
	Location   string                 `yaml:"location,omitempty"`
	Appearance string                 `yaml:"appearance,omitempty" json:"appearance,omitempty"`
	Aliases    []string               `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Faction    string                 `yaml:"faction,omitempty"`
	History    []int                  `yaml:"history,omitempty" json:"history,omitempty"`
	State      map[string]interface{} `yaml:"state,omitempty"`
	ExtraMeta  map[string]interface{} `yaml:",inline"`
}

type Entity struct {
	ID         string
	Name       string
	Type       string
	Tags       []string
	Voice      *VoiceConfig
	Portrait   string
	Location   string
	Faction    string
	Appearance string
	// Aliases are other names the same being is known by. They exist because a
	// model will rename a character, and the alternative to recording both names is
	// a second entity losing the first one's history.
	Aliases   []string
	History   []int
	State     *state.State
	ExtraMeta map[string]interface{}
	Body      string
	Wikilinks []string
	Hash      string
}

func (e *Entity) InitState(data map[string]interface{}) {
	e.State = state.NewState(data)
}

func ParseMarkdownEntity(data []byte) (*Entity, error) {
	hashBytes := sha256.Sum256(data)
	fileHash := hex.EncodeToString(hashBytes[:])

	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("entity markdown missing frontmatter header (---)")
	}

	endIdx := strings.Index(content[4:], "\n---\n")
	if endIdx == -1 {
		return nil, fmt.Errorf("entity markdown missing frontmatter closing delimiter")
	}

	frontmatterRaw := content[4 : 4+endIdx]
	bodyRaw := content[4+endIdx+5:]

	var fm EntityFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatterRaw), &fm); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	linksSet := make(map[string]struct{})
	var links []string

	// Scan frontmatter location/faction for wikilinks
	for _, rawRef := range []string{fm.Location, fm.Faction} {
		matches := wikilinkRegex.FindAllStringSubmatch(rawRef, -1)
		for _, m := range matches {
			if len(m) > 1 {
				target := strings.TrimSpace(m[1])
				if _, seen := linksSet[target]; !seen {
					linksSet[target] = struct{}{}
					links = append(links, target)
				}
			}
		}
	}

	// Scan body for wikilinks
	bodyMatches := wikilinkRegex.FindAllStringSubmatch(bodyRaw, -1)
	for _, m := range bodyMatches {
		if len(m) > 1 {
			target := strings.TrimSpace(m[1])
			if _, seen := linksSet[target]; !seen {
				linksSet[target] = struct{}{}
				links = append(links, target)
			}
		}
	}

	entity := &Entity{
		ID:         fm.ID,
		Name:       fm.Name,
		Type:       fm.Type,
		Tags:       fm.Tags,
		Voice:      fm.Voice,
		Portrait:   fm.Portrait,
		Location:   fm.Location,
		Faction:    fm.Faction,
		Appearance: fm.Appearance,
		Aliases:    fm.Aliases,
		History:    fm.History,
		State:      state.NewState(fm.State),
		ExtraMeta:  fm.ExtraMeta,
		Body:       bodyRaw,
		Wikilinks:  links,
		Hash:       fileHash,
	}

	return entity, nil
}

// WikilinkTarget extracts the link target from a [[target]] or [[target|label]]
// reference, returning the input unchanged when it is not a wikilink.
func WikilinkTarget(ref string) string {
	cleaned := strings.TrimSpace(ref)
	cleaned = strings.TrimPrefix(cleaned, "[[")
	cleaned = strings.TrimSuffix(cleaned, "]]")
	if idx := strings.Index(cleaned, "|"); idx >= 0 {
		cleaned = cleaned[:idx]
	}
	return strings.TrimSpace(cleaned)
}

// WikilinkTargets returns every link target found in text.
func WikilinkTargets(text string) []string {
	matches := wikilinkRegex.FindAllStringSubmatch(text, -1)
	targets := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			targets = append(targets, strings.TrimSpace(m[1]))
		}
	}
	return targets
}

// Slugify converts a display name into a stable kebab-case identifier.
func Slugify(name string) string {
	var buf strings.Builder
	precededBySeparator := true
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			buf.WriteRune(r)
			precededBySeparator = false
		case r == ' ' || r == '-' || r == '_':
			if buf.Len() > 0 && !precededBySeparator {
				buf.WriteRune('-')
				precededBySeparator = true
			}
		}
	}
	return strings.TrimSuffix(buf.String(), "-")
}

// IsCharacterType reports whether a type names a speaking being. Content authors,
// extractor models, and the graph all use different spellings for the same idea.
func IsCharacterType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "character", "npc", "person", "creature":
		return true
	}
	return false
}

func (e *Entity) SerializeMarkdown() ([]byte, error) {
	fm := EntityFrontmatter{
		ID:         e.ID,
		Name:       e.Name,
		Type:       e.Type,
		Tags:       e.Tags,
		Voice:      e.Voice,
		Portrait:   e.Portrait,
		Location:   e.Location,
		Faction:    e.Faction,
		Appearance: e.Appearance,
		Aliases:    e.Aliases,
		History:    e.History,
		ExtraMeta:  e.ExtraMeta,
	}
	if e.State != nil {
		fm.State = e.State.Raw()
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(fm); err != nil {
		return nil, fmt.Errorf("serialize frontmatter: %w", err)
	}
	buf.WriteString("---\n\n")
	buf.WriteString(e.Body)

	return buf.Bytes(), nil
}
