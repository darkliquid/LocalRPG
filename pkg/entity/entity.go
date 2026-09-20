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
	Provider string  `yaml:"provider,omitempty"`
	VoiceID  string  `yaml:"voice_id,omitempty"`
	Pitch    float64 `yaml:"pitch,omitempty"`
}

type EntityFrontmatter struct {
	ID        string                 `yaml:"id"`
	Name      string                 `yaml:"name"`
	Type      string                 `yaml:"type"`
	Tags      []string               `yaml:"tags,omitempty"`
	Voice     *VoiceConfig           `yaml:"voice,omitempty"`
	Portrait  string                 `yaml:"portrait,omitempty"`
	Location  string                 `yaml:"location,omitempty"`
	Faction   string                 `yaml:"faction,omitempty"`
	State     map[string]interface{} `yaml:"state,omitempty"`
	ExtraMeta map[string]interface{} `yaml:",inline"`
}

type Entity struct {
	ID        string
	Name      string
	Type      string
	Tags      []string
	Voice     *VoiceConfig
	Portrait  string
	Location  string
	Faction   string
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
		ID:        fm.ID,
		Name:      fm.Name,
		Type:      fm.Type,
		Tags:      fm.Tags,
		Voice:     fm.Voice,
		Portrait:  fm.Portrait,
		Location:  fm.Location,
		Faction:   fm.Faction,
		State:     state.NewState(fm.State),
		ExtraMeta: fm.ExtraMeta,
		Body:      bodyRaw,
		Wikilinks: links,
		Hash:      fileHash,
	}

	return entity, nil
}

func (e *Entity) SerializeMarkdown() ([]byte, error) {
	fm := EntityFrontmatter{
		ID:        e.ID,
		Name:      e.Name,
		Type:      e.Type,
		Tags:      e.Tags,
		Voice:     e.Voice,
		Portrait:  e.Portrait,
		Location:  e.Location,
		Faction:   e.Faction,
		ExtraMeta: e.ExtraMeta,
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
