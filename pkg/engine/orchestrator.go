package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type TurnOrchestrator struct {
	store         *storage.Store
	history       *HistoryLogger
	rulesEngine   *rules.JSEngine
	router        *harness.Router
	locationID    string
	playerID      string
	assembler     *harness.ContextAssembler
	rulesPrompt   string
	lorePrompt    string
	voiceProfiles []config.VoiceProfile
}

func NewTurnOrchestrator(
	store *storage.Store,
	history *HistoryLogger,
	rulesEngine *rules.JSEngine,
	router *harness.Router,
	locationID string,
	playerID string,
) *TurnOrchestrator {
	return &TurnOrchestrator{
		store:       store,
		history:     history,
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

func (o *TurnOrchestrator) SetVoiceProfiles(profiles []config.VoiceProfile) {
	o.voiceProfiles = profiles
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
	pastTurns, err := o.history.LoadHistory()
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	turnNum := len(pastTurns) + 1

	var rollRes *rules.RollResult
	var gmDirective string

	// Handle /undo command
	if strings.HasPrefix(strings.TrimSpace(actionInput), "/undo") {
		if len(pastTurns) == 0 {
			return nil, fmt.Errorf("no turns to undo")
		}
		if err := o.history.RewindToTurn(len(pastTurns) - 1); err != nil {
			return nil, fmt.Errorf("undo failed: %w", err)
		}
		return &Turn{
			Number:    len(pastTurns) - 1,
			Timestamp: time.Now(),
			Mode:      "System",
			Input:     "/undo",
			Output:    "Undid previous turn.",
		}, nil
	}

	// Handle /gm director note or mode
	isCorrection := mode == "GM" || strings.HasPrefix(actionInput, "/gm ")
	if isCorrection {
		directiveText := strings.TrimPrefix(actionInput, "/gm ")
		gmDirective = fmt.Sprintf("[DIRECTOR CORRECTION DIRECTIVE: %s]", directiveText)
	} else if strings.EqualFold(mode, "Roll") {
		// Evaluate dice roll
		r, err := rules.EvaluateRoll(actionInput)
		if err == nil {
			rollRes = r
			actionInput = fmt.Sprintf("I rolled %s with result %d", r.Notation, r.Total)
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

	// Assemble context with system rules, world lore prompts, and voice profiles
	contextPrompt, err := o.assembler.AssembleContextWithProfiles(o.locationID, o.playerID, actionInput, o.rulesPrompt, o.lorePrompt, o.voiceProfiles)
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
		Output:    resp.Text,
	}

	// Append to history
	if err := o.history.AppendTurn(turn); err != nil {
		return nil, fmt.Errorf("append turn: %w", err)
	}

	// Trigger post-turn hooks
	if o.rulesEngine != nil {
		_ = o.rulesEngine.ExecuteTurnEnd(map[string]interface{}{"turn": turnNum})
	}

	return &turn, nil
}
