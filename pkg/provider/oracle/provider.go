package oracle

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/trace"
)

type statValue struct {
	Name  string
	Value int
}

type parts struct {
	Tier     string
	Action   string
	Stakes   string
	Location string
	Stats    []statValue
	Entities []string
}

var (
	mechanicsTierRegex   = regexp.MustCompile(`\[MECHANICS RESULT:.*?Tier=([a-zA-Z0-9_\-]+)`)
	mechanicsSimpleRegex = regexp.MustCompile(`\[MECHANICS RESULT:\s*([a-zA-Z0-9_\-]+)[,\s\]]`)
	rollResultRegex      = regexp.MustCompile(`\[ROLL RESULT:\s*([a-zA-Z0-9_\-]+)(?:,\s*([^\]]+))?\]`)
	wikilinkPattern      = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|[^\]]+)?\]\]`)
	locationRegex        = regexp.MustCompile(`(?i)(?:\*\*Current Location:\*\*|Location:)\s*([^\n\r]+)`)
	stakesRegex          = regexp.MustCompile(`(?i)(?:^|\n)\s*Stakes:\s*([^\n\r]+)`)
	playerStatsRegex     = regexp.MustCompile(`(?i)Player stats:\s*([^\n\r]+)`)
)

// parsePrompt reads the directive tags, the location, the player's action, the entities,
// and the player's stats out of an assembled prompt. Each of those is written by a
// different part of the assembler, so each is read by the function named for it.
func parsePrompt(prompt string) parts {
	var res parts

	// 1. Tier and Stakes from directive tags
	res.Tier, res.Stakes = parseDirectiveTags(prompt)
	if res.Stakes == "" {
		if match := stakesRegex.FindStringSubmatch(prompt); len(match) == 2 {
			res.Stakes = strings.TrimSpace(match[1])
		}
	}

	// 2. Location
	res.Location = parseLocation(prompt)

	// 3. Player Action
	res.Action = parsePlayerAction(strings.Split(prompt, "\n"))

	// 4. Entities from wikilinks (excluding the location)
	res.Entities = parseWikilinkEntities(prompt, res.Location)

	// 5. Player Stats
	res.Stats = parsePlayerStats(prompt)

	return res
}

// parseDirectiveTags is the tier and the stakes a prompt's directive tags carry: a
// mechanics result, a roll result, or the plain mechanics tag, whichever matches first. A
// roll result may carry the stakes with it.
func parseDirectiveTags(prompt string) (tier, stakes string) {
	if match := mechanicsTierRegex.FindStringSubmatch(prompt); len(match) == 2 {
		return match[1], ""
	}
	if match := rollResultRegex.FindStringSubmatch(prompt); len(match) >= 2 {
		tier = match[1]
		if len(match) >= 3 && match[2] != "" {
			stakes = strings.TrimSpace(match[2])
		}
		return tier, stakes
	}
	if match := mechanicsSimpleRegex.FindStringSubmatch(prompt); len(match) == 2 {
		if !strings.Contains(match[1], "=") {
			tier = match[1]
		}
	}
	return tier, ""
}

// parseLocation is the current location, with a wikilink unwrapped and any label the link
// carries dropped.
func parseLocation(prompt string) string {
	match := locationRegex.FindStringSubmatch(prompt)
	if len(match) != 2 {
		return ""
	}
	loc := strings.TrimSpace(match[1])
	if strings.HasPrefix(loc, "[[") && strings.HasSuffix(loc, "]]") {
		loc = loc[2 : len(loc)-2]
	}
	if idx := strings.Index(loc, "|"); idx != -1 {
		loc = strings.TrimSpace(loc[:idx])
	}
	return strings.TrimSpace(loc)
}

// parsePlayerAction is what the player asked for: the labelled line when the prompt has
// one, and otherwise the first prose line under the action heading. A prompt that says
// nothing falls back to a stand-in, so the deterministic provider still narrates
// something.
func parsePlayerAction(lines []string) string {
	action := "You steel your resolve and take action."
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "Player Action:") {
			action = strings.TrimSpace(strings.TrimPrefix(trimmed, "Player Action:"))
		} else if strings.HasPrefix(trimmed, "Player: Player Action:") {
			action = strings.TrimSpace(strings.TrimPrefix(trimmed, "Player: Player Action:"))
		} else if trimmed == "## PLAYER ACTION" && i+1 < len(lines) {
			for j := i + 1; j < len(lines); j++ {
				next := strings.TrimSpace(lines[j])
				if next != "" && !strings.HasPrefix(next, "#") {
					if colon := strings.Index(next, ": "); colon != -1 {
						action = strings.TrimSpace(next[colon+2:])
					} else {
						action = next
					}
					break
				}
			}
		}
	}
	return action
}

// parseWikilinkEntities is the entities a prompt links to, in the order they appear and
// without repeats. The location is left out: it has a field of its own, and listing it
// again would offer it as a participant in the scene.
func parseWikilinkEntities(prompt, location string) []string {
	var entities []string
	seen := make(map[string]bool)
	for _, m := range wikilinkPattern.FindAllStringSubmatch(prompt, -1) {
		if len(m) < 2 {
			continue
		}
		ent := strings.TrimSpace(m[1])
		if ent == "" || strings.EqualFold(ent, location) || seen[ent] {
			continue
		}
		seen[ent] = true
		entities = append(entities, ent)
	}
	return entities
}

// parsePlayerStats reads the player's stats off the line the assembler writes, one
// "Name Value" pair per comma-separated token. A token that carries no number is skipped
// rather than guessed at.
func parsePlayerStats(prompt string) []statValue {
	match := playerStatsRegex.FindStringSubmatch(prompt)
	if len(match) != 2 {
		return nil
	}

	rawStats := strings.TrimSuffix(strings.TrimSpace(match[1]), ".")
	var stats []statValue
	for _, tok := range strings.Split(rawStats, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		lastSpace := strings.LastIndex(tok, " ")
		if lastSpace == -1 {
			continue
		}
		name := strings.TrimSpace(tok[:lastSpace])
		valStr := strings.TrimSpace(tok[lastSpace+1:])
		val, err := strconv.Atoi(valStr)
		if err == nil && name != "" {
			stats = append(stats, statValue{Name: name, Value: val})
		}
	}
	return stats
}

type narrativeOracleProvider struct {
	id     string
	logger trace.Logger
}

// NewNarrativeOracleProvider builds the deterministic storyteller.
func NewNarrativeOracleProvider(id string) harness.ModelProvider {
	return &narrativeOracleProvider{id: id}
}

// SetLogger attaches a trace sink, so a deterministic provider is visible in the
// trace as the thing that answered.
func (n *narrativeOracleProvider) SetLogger(logger trace.Logger) {
	n.logger = trace.OrNil(logger)
}

// logResponse records the crafted reply's size and that no model was called.
func (n *narrativeOracleProvider) logResponse(text string, start time.Time) {
	n.logger = trace.OrNil(n.logger)
	n.logger.Event("provider.response", map[string]any{
		"role":          n.id,
		"kind":          "oracle",
		"finish_reason": "stop",
		"bytes":         len(text),
		"total_ms":      time.Since(start).Milliseconds(),
	})
}

func (n *narrativeOracleProvider) ID() string { return n.id }

func (n *narrativeOracleProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	start := time.Now()
	text := n.craftProse(req.PromptText())
	n.logResponse(text, start)
	return &harness.GenerateResponse{Text: text}, nil
}

func (n *narrativeOracleProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	start := time.Now()
	text := n.craftProse(req.PromptText())
	n.logResponse(text, start)
	out <- harness.StreamChunk{Text: text, Done: true}
	return nil
}

func (n *narrativeOracleProvider) craftProse(prompt string) string {
	return craftProse(prompt)
}

func craftProse(prompt string) string {
	p := parsePrompt(prompt)

	seed := int64(len(prompt) * 17)
	rng := rand.New(rand.NewSource(seed))

	op := opener(p, rng)
	conseq := consequence(p, rng)
	cast := castLine(p, rng)
	place := placeLine(p, rng)

	var secondParagraph []string
	if p.Action != "" && p.Action != "You steel your resolve and take action." {
		secondParagraph = append(secondParagraph, fmt.Sprintf("As you declare: %s, the consequences take shape.", strconv.Quote(p.Action)))
	} else {
		secondParagraph = append(secondParagraph, "As you steel your resolve and take action, the consequences take shape.")
	}

	if conseq != "" {
		secondParagraph = append(secondParagraph, conseq)
	}
	if cast != "" {
		secondParagraph = append(secondParagraph, cast)
	}
	if place != "" {
		secondParagraph = append(secondParagraph, place)
	}
	secondParagraph = append(secondParagraph, "What do you do next?")

	body := strings.Join(secondParagraph, " ")
	if op == "" {
		return body
	}
	return op + "\n\n" + body
}
