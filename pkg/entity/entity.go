package entity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
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
	Voice           *VoiceConfig           `yaml:"voice,omitempty"`
	Portrait        string                 `yaml:"portrait,omitempty"`
	PortraitVersion int                    `yaml:"portrait_version,omitempty" json:"portrait_version,omitempty"`
	PortraitHistory []string               `yaml:"portrait_history,omitempty" json:"portrait_history,omitempty"`
	Location        string                 `yaml:"location,omitempty"`
	Appearance      string                 `yaml:"appearance,omitempty" json:"appearance,omitempty"`
	Gender          string                 `yaml:"gender,omitempty" json:"gender,omitempty"`
	Age             string                 `yaml:"age,omitempty" json:"age,omitempty"`
	Aliases         []string               `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Faction         string                 `yaml:"faction,omitempty"`
	History         []int                  `yaml:"history,omitempty" json:"history,omitempty"`
	State           map[string]interface{} `yaml:"state,omitempty"`
	ExtraMeta       map[string]interface{} `yaml:",inline"`
}

type Entity struct {
	ID              string
	Name            string
	Type            string
	Tags            []string
	Voice           *VoiceConfig
	Portrait        string
	PortraitVersion int
	PortraitHistory []string
	Location        string
	Faction         string
	Appearance      string
	Gender          string
	Age             string
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
	// Folder is where the note sits under entities/, relative and slash-separated,
	// with "" for the root. It is a location, not frontmatter: the same note in two
	// folders is the same note, so SerializeMarkdown must never emit it.
	Folder string
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
		return nil, frontmatterError(frontmatterRaw, err)
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

	entID := fm.ID
	if entID == "" {
		entID = Slugify(fm.Name)
	}

	entity := &Entity{
		ID:         entID,
		Name:       fm.Name,
		Type:       fm.Type,
		Tags:       fm.Tags,
		Voice:      fm.Voice,
		Portrait:        fm.Portrait,
		PortraitVersion: fm.PortraitVersion,
		PortraitHistory: fm.PortraitHistory,
		Location:        fm.Location,
		Faction:    fm.Faction,
		Appearance: fm.Appearance,
		Gender:     fm.Gender,
		Age:        fm.Age,
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

// WikilinkBasename returns the final path segment of a link target, so a
// hand-written [[guilds/silver-hand]] can still resolve to the note whose id is
// silver-hand. The app never generates the path-qualified form.
func WikilinkBasename(target string) string {
	cleaned := strings.TrimSpace(target)
	if idx := strings.LastIndex(cleaned, "/"); idx >= 0 {
		cleaned = cleaned[idx+1:]
	}
	return strings.TrimSpace(cleaned)
}

// DeclaredID returns the id a document's frontmatter declares, or "" when it has
// none. It exists so a writer can detect a collision without parsing the whole
// note and without paying for the body.
func DeclaredID(data []byte) string {
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return ""
	}
	endIdx := strings.Index(content[4:], "\n---\n")
	if endIdx == -1 {
		return ""
	}

	var fm struct {
		ID string `yaml:"id"`
	}
	if err := yaml.Unmarshal([]byte(content[4:4+endIdx]), &fm); err != nil {
		return ""
	}
	return strings.TrimSpace(fm.ID)
}

// FrontmatterError is a frontmatter parse failure carrying the byte offset of the
// offending position, so a caller can point at the line instead of quoting a yaml
// error at the author.
type FrontmatterError struct {
	Offset int
	Msg    string
}

func (e *FrontmatterError) Error() string {
	return "parse frontmatter: " + e.Msg
}

// Line reports the 1-based line of the failure within a document, so a caller can
// place a diagnostic without knowing how the frontmatter is delimited.
func (e *FrontmatterError) Line(document string) int {
	if e.Offset <= 0 {
		return 1
	}
	if e.Offset > len(document) {
		return strings.Count(document, "\n") + 1
	}
	return strings.Count(document[:e.Offset], "\n") + 1
}

// Column reports the 1-based column of the failure within its line.
func (e *FrontmatterError) Column(document string) int {
	if e.Offset <= 0 || e.Offset > len(document) {
		return 1
	}
	lastNewline := strings.LastIndexByte(document[:e.Offset], '\n')
	return e.Offset - lastNewline
}

// frontmatterError converts a yaml failure into one carrying a byte offset in the
// document. gopkg.in/yaml.v3 reports a line number but not a position, so the line
// is located inside the raw block and the block's own header offset is added.
func frontmatterError(frontmatterRaw string, err error) *FrontmatterError {
	line := yamlErrorLine(err.Error())
	offset := len("---\n") + lineOffset(frontmatterRaw, line)
	return &FrontmatterError{Offset: offset, Msg: err.Error()}
}

// lineOffset returns the byte offset of a 1-based line within text.
func lineOffset(text string, line int) int {
	if line <= 1 {
		return 0
	}
	offset := 0
	for i := 1; i < line; i++ {
		next := strings.IndexByte(text[offset:], '\n')
		if next < 0 {
			return len(text)
		}
		offset += next + 1
	}
	return offset
}

// yamlErrorLine extracts the 1-based line from a gopkg.in/yaml.v3 error, which
// reads "yaml: line 3: ...".
func yamlErrorLine(message string) int {
	const marker = "line "
	idx := strings.Index(message, marker)
	if idx < 0 {
		return 1
	}
	rest := message[idx+len(marker):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 1
	}
	line, err := strconv.Atoi(rest[:end])
	if err != nil || line < 1 {
		return 1
	}
	return line
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
		Voice:           e.Voice,
		Portrait:        e.Portrait,
		PortraitVersion: e.PortraitVersion,
		PortraitHistory: e.PortraitHistory,
		Location:        e.Location,
		Faction:         e.Faction,
		Appearance:      e.Appearance,
		Gender:          e.Gender,
		Age:             e.Age,
		Aliases:         e.Aliases,
		History:         e.History,
	}
	if len(e.ExtraMeta) > 0 {
		extra := make(map[string]interface{}, len(e.ExtraMeta))
		for k, v := range e.ExtraMeta {
			switch strings.ToLower(k) {
			case "id", "name", "type", "tags", "voice", "portrait", "portrait_version", "portrait_history", "location", "faction", "appearance", "gender", "age", "aliases", "history", "state":
				continue
			default:
				extra[k] = v
			}
		}
		if len(extra) > 0 {
			fm.ExtraMeta = extra
		}
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
