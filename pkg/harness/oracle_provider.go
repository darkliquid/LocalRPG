package harness

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/trace"
)

var mechanicsResultRegex = regexp.MustCompile(`\[MECHANICS RESULT:.*?Tier=([a-zA-Z]+)`)
var wikilinkPattern = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|[^\]]+)?\]\]`)

type narrativeOracleProvider struct {
	id     string
	logger trace.Logger
}

func NewNarrativeOracleProvider(id string) ModelProvider {
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

func (n *narrativeOracleProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	start := time.Now()
	text := n.craftProse(req.PromptText())
	n.logResponse(text, start)
	return &GenerateResponse{Text: text}, nil
}

func (n *narrativeOracleProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)
	start := time.Now()
	text := n.craftProse(req.PromptText())
	n.logResponse(text, start)
	out <- StreamChunk{Text: text, Done: true}
	return nil
}

func (n *narrativeOracleProvider) craftProse(prompt string) string {
	tier := "Success"
	if match := mechanicsResultRegex.FindStringSubmatch(prompt); len(match) == 2 {
		tier = match[1]
	}

	lines := strings.Split(prompt, "\n")
	playerAction := "You steel your resolve and take action."
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "Player Action:") {
			playerAction = strings.TrimPrefix(l, "Player Action:")
			playerAction = strings.TrimSpace(playerAction)
		}
	}

	// Extract wikilinks from prompt to ground the prose in current entities
	var entities []string
	matches := wikilinkPattern.FindAllStringSubmatch(prompt, -1)
	for _, m := range matches {
		if len(m) >= 2 {
			entities = append(entities, m[1])
		}
	}

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

	return fmt.Sprintf("%s\n\nAs you declare: \"%s\", the stones echo your effort.%s What do you do next?", chosenOpener, playerAction, entityWitness)
}
