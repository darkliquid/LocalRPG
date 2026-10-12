package harness

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// defaultSummaryChars matches the configured default, so a summariser built without
// a limit still cannot inject an unbounded block.
const defaultSummaryChars = 2000

// SummaryTurn is one turn as the summariser sees it.
type SummaryTurn struct {
	Number    int
	Mode      string
	Input     string
	Narration string
}

// summariserPrompt asks for the things a later turn needs and forbids the thing
// that would poison canon: invention.
const summariserPrompt = `## TASK
Rewrite the story so far, incorporating the new turns. This is a recollection for a
narrator, not prose for the player.

## RULES
- Preserve names, places, promises, oaths, debts, and unresolved threads exactly.
- Preserve state changes: who was wounded, what was spent, what was lit or doused.
- Drop verbatim dialogue and scene-setting that a later turn does not need.
- Never invent events, characters, or outcomes that the turns do not contain.
- Write plain prose, no headings, no lists, no bullet points.
- Keep it under %d characters.

## PREVIOUS SUMMARY
%s

## NEW TURNS
%s

## REWRITTEN SUMMARY`

// Summariser turns a range of turns into a compact recollection. It is the only
// model call the memory system makes, and it is always detached from a turn.
type Summariser struct {
	provider  ModelProvider
	charLimit int
	logger    trace.Logger
}

func NewSummariser(provider ModelProvider) *Summariser {
	return &Summariser{provider: provider, charLimit: defaultSummaryChars}
}

func (s *Summariser) SetLogger(logger trace.Logger) {
	s.logger = trace.OrNil(logger)
}

// SetCharLimit caps the summary. Zero restores the default, so a misconfigured
// value cannot remove the bound.
func (s *Summariser) SetCharLimit(limit int) {
	if limit <= 0 {
		limit = defaultSummaryChars
	}
	s.charLimit = limit
}

// Summarise asks for a rewritten recollection. A failure is returned rather than
// swallowed, because the caller must leave through_turn alone so the next trigger
// tries again.
func (s *Summariser) Summarise(ctx context.Context, previous string, turns []SummaryTurn) (string, error) {
	if s.provider == nil {
		return "", fmt.Errorf("summarise: no provider")
	}

	start := time.Now()
	s.logger = trace.OrNil(s.logger)

	var transcript strings.Builder
	for _, turn := range turns {
		if input := strings.TrimSpace(turn.Input); input != "" {
			fmt.Fprintf(&transcript, "Turn %d - Player [%s]: %s\n", turn.Number, turn.Mode, input)
		} else {
			fmt.Fprintf(&transcript, "Turn %d - [%s]\n", turn.Number, turn.Mode)
		}
		if narration := strings.TrimSpace(turn.Narration); narration != "" {
			fmt.Fprintf(&transcript, "Narrator: %s\n\n", narration)
		}
	}

	prompt := fmt.Sprintf(summariserPrompt, s.charLimit, strings.TrimSpace(previous), strings.TrimSpace(transcript.String()))

	response, err := s.provider.Generate(ctx, GenerateRequest{Prompt: prompt})
	if err != nil {
		s.logger.Event("provider.error", map[string]any{"role": "summariser", "error": err.Error()})
		return "", fmt.Errorf("summarise %d turn(s): %w", len(turns), err)
	}

	summary := truncateForCap(strings.TrimSpace(response.Text), s.charLimit)

	s.logger.Event("summary.regenerate", map[string]any{
		"turns":       len(turns),
		"chars":       len([]rune(summary)),
		"char_limit":  s.charLimit,
		"duration_ms": time.Since(start).Milliseconds(),
	})

	return summary, nil
}

// truncateForCap returns at most max runes, reserving room for the marker so the
// result honours the configured cap instead of overshooting it by the marker's
// length.
func truncateForCap(value string, max int) string {
	if max <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= max {
		return value
	}

	const marker = "..."
	keep := max - len([]rune(marker))
	if keep < 0 {
		keep = 0
	}
	return string(runes[:keep]) + marker
}

// SummariserFromConfig resolves the role that writes summaries. It mirrors the
// extractor: an absent role inherits gm, `disabled` turns summaries off entirely,
// and `inherit` follows the named role.
func SummariserFromConfig(cfg *config.Config, router *Router, logger trace.Logger) *Summariser {
	if cfg == nil || router == nil {
		return nil
	}

	roleCfg, configured := cfg.Agents.Roles[config.RoleExtractor]
	if !configured {
		roleCfg = config.AgentRoleConfig{Type: "inherit", InheritFrom: config.RoleGM}
	}

	summariser := (*Summariser)(nil)

	switch roleCfg.Type {
	case "disabled":
		return nil
	case "inherit", "":
		source := roleCfg.InheritFrom
		if source == "" {
			source = config.RoleGM
		}
		provider, err := router.GetProviderForRole(source)
		if err != nil {
			return nil
		}
		summariser = NewSummariser(provider)
	default:
		provider, err := NewModelProviderWithLogger(config.RoleExtractor, ProviderConfig{
			Type:           roleCfg.Type,
			BuiltinName:    roleCfg.BuiltinName,
			Command:        roleCfg.Command,
			Args:           roleCfg.Args,
			Endpoint:       roleCfg.Endpoint,
			Model:          roleCfg.Model,
			APIKey:         roleCfg.APIKey,
			Temperature:    roleCfg.Temperature,
			MaxTokens:      roleCfg.MaxTokens,
			ThinkingBudget: roleCfg.ThinkingBudget,
			TopP:           roleCfg.TopP,
			TopK:           roleCfg.TopK,
			SharedAPIKey:   cfg.Providers.Gemini.APIKey,
		}, logger)
		if err != nil {
			return nil
		}
		summariser = NewSummariser(provider)
	}

	summariser.SetLogger(logger)
	summariser.SetCharLimit(cfg.SummaryCharLimit())
	return summariser
}
