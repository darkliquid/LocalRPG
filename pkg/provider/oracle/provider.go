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

func parsePrompt(prompt string) parts {
	var res parts

	// 1. Tier and Stakes from directive tags
	if match := mechanicsTierRegex.FindStringSubmatch(prompt); len(match) == 2 {
		res.Tier = match[1]
	} else if match := rollResultRegex.FindStringSubmatch(prompt); len(match) >= 2 {
		res.Tier = match[1]
		if len(match) >= 3 && match[2] != "" && res.Stakes == "" {
			res.Stakes = strings.TrimSpace(match[2])
		}
	} else if match := mechanicsSimpleRegex.FindStringSubmatch(prompt); len(match) == 2 {
		if !strings.Contains(match[1], "=") {
			res.Tier = match[1]
		}
	}

	if match := stakesRegex.FindStringSubmatch(prompt); len(match) == 2 && res.Stakes == "" {
		res.Stakes = strings.TrimSpace(match[1])
	}

	// 2. Location
	if match := locationRegex.FindStringSubmatch(prompt); len(match) == 2 {
		loc := strings.TrimSpace(match[1])
		if strings.HasPrefix(loc, "[[") && strings.HasSuffix(loc, "]]") {
			loc = loc[2 : len(loc)-2]
		}
		if idx := strings.Index(loc, "|"); idx != -1 {
			loc = strings.TrimSpace(loc[:idx])
		}
		res.Location = strings.TrimSpace(loc)
	}

	// 3. Player Action
	lines := strings.Split(prompt, "\n")
	res.Action = "You steel your resolve and take action."
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "Player Action:") {
			res.Action = strings.TrimSpace(strings.TrimPrefix(trimmed, "Player Action:"))
		} else if strings.HasPrefix(trimmed, "Player: Player Action:") {
			res.Action = strings.TrimSpace(strings.TrimPrefix(trimmed, "Player: Player Action:"))
		} else if trimmed == "## PLAYER ACTION" && i+1 < len(lines) {
			for j := i + 1; j < len(lines); j++ {
				next := strings.TrimSpace(lines[j])
				if next != "" && !strings.HasPrefix(next, "#") {
					if colon := strings.Index(next, ": "); colon != -1 {
						res.Action = strings.TrimSpace(next[colon+2:])
					} else {
						res.Action = next
					}
					break
				}
			}
		}
	}

	// 4. Entities from wikilinks (excluding the location)
	matches := wikilinkPattern.FindAllStringSubmatch(prompt, -1)
	seen := make(map[string]bool)
	for _, m := range matches {
		if len(m) >= 2 {
			ent := strings.TrimSpace(m[1])
			if ent != "" && !strings.EqualFold(ent, res.Location) && !seen[ent] {
				seen[ent] = true
				res.Entities = append(res.Entities, ent)
			}
		}
	}

	// 5. Player Stats
	if statMatch := playerStatsRegex.FindStringSubmatch(prompt); len(statMatch) == 2 {
		rawStats := strings.TrimSpace(statMatch[1])
		rawStats = strings.TrimSuffix(rawStats, ".")
		tokens := strings.Split(rawStats, ",")
		for _, tok := range tokens {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			lastSpace := strings.LastIndex(tok, " ")
			if lastSpace != -1 {
				name := strings.TrimSpace(tok[:lastSpace])
				valStr := strings.TrimSpace(tok[lastSpace+1:])
				val, err := strconv.Atoi(valStr)
				if err == nil && name != "" {
					res.Stats = append(res.Stats, statValue{Name: name, Value: val})
				}
			}
		}
	}

	return res
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
	n.logger.Event("provider.response", map[string]interface{}{
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
	p := parsePrompt(prompt)
	tier := p.Tier
	if tier == "" {
		tier = "Success"
	}
	playerAction := p.Action
	entities := p.Entities

	seed := int64(len(prompt) * 17)
	rng := rand.New(rand.NewSource(seed))

	successOpeners := []string{
		"With practiced grace and sharp focus, your intent takes hold.",
		"The tides of fate answer your call; shadows part before your advance.",
		"Your action lands with resounding clarity across the chamber.",
	}

	mixedOpeners := []string{
		"You gain ground, though not without feeling the cold sting of consequence.",
		"The maneuver succeeds, but the environment twists unexpectedly beneath your boots.",
		"A hard-won advantage, though eyes in the darkness take note of your position.",
	}

	failureOpeners := []string{
		"The darkness lashes out; your footing betrays you at the pivotal instant.",
		"A sudden jarring blow forces you back as the enemy anticipates your intent.",
		"The air turns freezing cold as the ancient wards shudder and resist.",
	}

	var chosenOpener string
	switch strings.ToLower(tier) {
	case "critical", "success", "full success":
		chosenOpener = successOpeners[rng.Intn(len(successOpeners))]
	case "mixed", "partial", "complication":
		chosenOpener = mixedOpeners[rng.Intn(len(mixedOpeners))]
	default:
		chosenOpener = failureOpeners[rng.Intn(len(failureOpeners))]
	}

	entityWitness := ""
	if len(entities) > 0 {
		chosenEntity := entities[rng.Intn(len(entities))]
		entityWitness = fmt.Sprintf(" Nearby, [[%s]] watches the outcome with bated breath.", chosenEntity)
	}

	quotedAction := strconv.Quote(playerAction)
	return fmt.Sprintf("%s\n\nAs you declare: %s, the stones echo your effort.%s What do you do next?", chosenOpener, quotedAction, entityWitness)
}
