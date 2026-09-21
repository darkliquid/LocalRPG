package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type TurnOrchestrator struct {
	store       *storage.Store
	timeline    *Timeline
	rulesEngine *rules.JSEngine
	router      *harness.Router
	locationID  string
	playerID    string
	assembler   *harness.ContextAssembler
	rulesPrompt string
	lorePrompt  string
	extractor   *harness.Extractor
}

func NewTurnOrchestrator(
	store *storage.Store,
	timeline *Timeline,
	rulesEngine *rules.JSEngine,
	router *harness.Router,
	locationID string,
	playerID string,
) *TurnOrchestrator {
	return &TurnOrchestrator{
		store:       store,
		timeline:    timeline,
		rulesEngine: rulesEngine,
		router:      router,
		locationID:  locationID,
		playerID:    playerID,
		assembler:   harness.NewContextAssembler(store),
	}
}

func (o *TurnOrchestrator) SetPrompts(rulesPrompt, lorePrompt string) {
	o.rulesPrompt = rulesPrompt
	o.lorePrompt = lorePrompt
}

// SetExtractor enables per-turn entity extraction. A nil extractor records
// deterministic mentions only.
func (o *TurnOrchestrator) SetExtractor(extractor *harness.Extractor) {
	o.extractor = extractor
}

func (o *TurnOrchestrator) LoadPrompts(paths *core.PathResolver, systemID, worldID string) {
	if paths != nil {
		if systemID != "" {
			if data, err := os.ReadFile(filepath.Join(paths.SystemDir(systemID), "prompts", "rules.md")); err == nil {
				o.rulesPrompt = string(data)
			}
		}
		if worldID != "" {
			if data, err := os.ReadFile(filepath.Join(paths.WorldDir(worldID), "prompts", "lore.md")); err == nil {
				o.lorePrompt = string(data)
			}
		}
	}
}

func (o *TurnOrchestrator) ProcessAction(ctx context.Context, mode, actionInput string) (*Turn, error) {
	pastTurns, err := o.timeline.history.LoadHistory()
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	turnNum := len(pastTurns) + 1

	var rollRes *rules.RollResult
	var gmDirective string
	generationPrompt := actionInput

	// Handle /undo command
	if strings.HasPrefix(strings.TrimSpace(actionInput), "/undo") {
		if len(pastTurns) == 0 {
			return nil, fmt.Errorf("no turns to undo")
		}
		if err := o.timeline.RewindToTurn(len(pastTurns) - 1); err != nil {
			return nil, fmt.Errorf("undo failed: %w", err)
		}
		return &Turn{
			Number:    len(pastTurns) - 1,
			Timestamp: time.Now(),
			Mode:      "System",
			Input:     "/undo",
			Narration: "Undid previous turn.",
		}, nil
	}

	// Handle /gm director note or mode
	isCorrection := mode == "GM" || strings.HasPrefix(actionInput, "/gm ")
	if isCorrection {
		directiveText := strings.TrimPrefix(actionInput, "/gm ")
		gmDirective = fmt.Sprintf("[DIRECTOR CORRECTION DIRECTIVE: %s]", directiveText)
	} else if strings.EqualFold(mode, "Roll") {
		if r, err := rules.EvaluateRoll(actionInput); err == nil {
			rollRes = r
			generationPrompt = fmt.Sprintf("I rolled %s with result %d", r.Notation, r.Total)
		}
	} else if o.rulesEngine != nil {
		// Run action through mechanics hook if available
		res, err := o.rulesEngine.ExecuteAction(strings.ToLower(mode), map[string]interface{}{
			"action": actionInput,
			"player": o.playerID,
		})
		if err == nil && res != nil {
			rollRes = res.Roll
			if res.Message != "" {
				gmDirective = fmt.Sprintf("[MECHANICS RESULT: %s]", res.Message)
			}
		}
	}

	// Assemble context with system rules and world lore prompts
	contextPrompt, err := o.assembler.AssembleContextWithProfiles(o.locationID, o.playerID, generationPrompt, o.rulesPrompt, o.lorePrompt, o.timeline.VoiceProfiles())
	if err != nil {
		return nil, fmt.Errorf("assemble context: %w", err)
	}

	if gmDirective != "" {
		contextPrompt = gmDirective + "\n\n" + contextPrompt
	}

	// Generate story response from GM model
	req := harness.GenerateRequest{
		Prompt: contextPrompt,
	}

	resp, err := o.router.GenerateForRole(ctx, "gm", req)
	if err != nil {
		return nil, fmt.Errorf("gm generation failed: %w", err)
	}

	turn := Turn{
		Number:    turnNum,
		Timestamp: time.Now(),
		Mode:      mode,
		Input:     actionInput,
		Roll:      rollRes,
		Narration: resp.Text,
	}

	turn.Entities = harness.ResolveEntityMentions(o.store, o.playerID, o.locationID, turn.Narration, actionInput)

	extraction := harness.Extraction{}
	if o.extractor != nil {
		// A failed extractor must not lose the turn; the mentions above still stand.
		if result, err := o.extractor.Extract(ctx, turn.Narration); err == nil {
			extraction = *result
		}
	}

	turn.Segments = buildTurnSegments(o.store, mode, o.playerID, actionInput, turn.Narration, extraction.Dialogue)
	for _, mention := range speechMentions(turn.Segments) {
		if !containsMention(turn.Entities, mention.ID) {
			turn.Entities = append(turn.Entities, mention)
		}
	}

	if err := o.timeline.RecordTurn(&turn, extraction.Entities); err != nil {
		return nil, fmt.Errorf("record turn: %w", err)
	}

	// Trigger post-turn hooks
	if o.rulesEngine != nil {
		_ = o.rulesEngine.ExecuteTurnEnd(map[string]interface{}{"turn": turnNum})
	}

	return &turn, nil
}
