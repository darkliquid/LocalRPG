package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// ErrGenerationStalled reports that the gm provider stopped sending deltas for
// longer than the configured chunk timeout.
var ErrGenerationStalled = errors.New("gm generation stalled")

// OpeningPromptSetting is the campaign setting holding the player's own opening
// instruction. Absent means the GM invents the scene, which is the default.
const OpeningPromptSetting = "opening_prompt"

// OpeningMode is the reserved mode of a campaign's first turn.
const OpeningMode = "Opening"

// defaultChunkTimeout is the silence tolerated between deltas when a caller sets none.
const defaultChunkTimeout = 60 * time.Second

// OpeningPrompt reads a campaign's configured opening instruction, or "" when
// the GM should invent the scene.
func OpeningPrompt(manifest *core.GameManifest) string {
	if manifest == nil || manifest.Settings == nil {
		return ""
	}
	prompt, _ := manifest.Settings[OpeningPromptSetting].(string)
	return strings.TrimSpace(prompt)
}

// openingDirective is the instruction the GM receives as the campaign's first
// turn. It establishes the scene without deciding the protagonist's own actions,
// which is the one thing a narrator must not take away from a player.
func openingDirective(prompt string) string {
	var sb strings.Builder
	sb.WriteString("[OPENING SCENE]\n")
	sb.WriteString("Establish the opening of this campaign.\n")
	sb.WriteString("Describe where the protagonist is, what they can perceive, and one thing that invites action.\n")
	sb.WriteString("Introduce at most one present character, using their established name.\n")
	sb.WriteString("Do not decide the protagonist's actions, thoughts, or feelings.\n")
	if trimmed := strings.TrimSpace(prompt); trimmed != "" {
		sb.WriteString("\n" + trimmed + "\n")
	}
	return sb.String()
}

type TurnOrchestrator struct {
	store         *storage.Store
	timeline      *Timeline
	rulesEngine   *rules.JSEngine
	router        *harness.Router
	startLocation string
	playerID      string
	assembler     *harness.ContextAssembler
	rulesPrompt   string
	lorePrompt    string
	extractor     *harness.Extractor
	chunkTimeout  time.Duration
	openingPrompt string
}

func NewTurnOrchestrator(
	store *storage.Store,
	timeline *Timeline,
	rulesEngine *rules.JSEngine,
	router *harness.Router,
	startLocation string,
	playerID string,
) *TurnOrchestrator {
	return &TurnOrchestrator{
		store:         store,
		timeline:      timeline,
		rulesEngine:   rulesEngine,
		router:        router,
		startLocation: startLocation,
		playerID:      playerID,
		assembler:     harness.NewContextAssembler(store),
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

// SetChunkTimeout bounds the silence tolerated between narration deltas. Zero
// restores the default, so a misconfigured value cannot disable the watchdog.
func (o *TurnOrchestrator) SetChunkTimeout(timeout time.Duration) {
	if timeout <= 0 {
		timeout = defaultChunkTimeout
	}
	o.chunkTimeout = timeout
}

// SetOpeningPrompt supplies the player's own opening instruction, used when the
// campaign's first turn runs in OpeningMode.
func (o *TurnOrchestrator) SetOpeningPrompt(prompt string) {
	o.openingPrompt = prompt
}

// currentLocation resolves where this turn is happening. The player note wins
// because it is the single source of truth; the last recorded turn follows, so a
// note that points at a deleted location cannot teleport the party back to where
// the campaign opened; the pinned start location is the bootstrap for a campaign
// with no history at all.
func (o *TurnOrchestrator) currentLocation() string {
	if o.playerID != "" {
		if player, err := o.store.GetEntity(o.playerID); err == nil && player != nil && player.Location != "" {
			if ent := findLocationByRef(o.store, player.Location); ent != nil {
				return ent.ID
			}
		}
	}

	if previous := o.previousLocation(); previous != "" {
		return previous
	}

	if o.startLocation != "" {
		if ent := findLocationByRef(o.store, o.startLocation); ent != nil {
			return ent.ID
		}
	}

	return ""
}

// CurrentLocationName returns the display name of where the party is, for
// clients that show it without loading the entity themselves.
func (o *TurnOrchestrator) CurrentLocationName() string {
	id := o.currentLocation()
	if id == "" {
		return ""
	}
	if ent, err := o.store.GetEntity(id); err == nil && ent != nil && ent.Name != "" {
		return ent.Name
	}
	return id
}

// previousLocation is the most recent location a turn recorded, scanning back past
// turns that predate location tracking.
func (o *TurnOrchestrator) previousLocation() string {
	turns, err := o.timeline.history.LoadHistory()
	if err != nil {
		return ""
	}

	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Location != "" {
			return turns[i].Location
		}
	}
	return ""
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

// ProcessActionStream runs a turn, reporting narration deltas as they arrive. It
// is the implementation; ProcessAction is the same pipeline without a listener.
// A non-nil error from onChunk aborts before anything is recorded, which is how a
// client that has disconnected stops generation rather than letting it finish into
// nothing.
func (o *TurnOrchestrator) ProcessActionStream(ctx context.Context, mode, actionInput string, onChunk func(text string) error) (*Turn, error) {
	pastTurns, err := o.timeline.history.LoadHistory()
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	turnNum := len(pastTurns) + 1

	var rollRes *rules.RollResult
	var outcome string
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

	// Handle /go command: an explicit move, recorded as its own system turn.
	if strings.HasPrefix(strings.TrimSpace(actionInput), "/go ") {
		target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(actionInput), "/go "))

		ent := findLocationByRef(o.store, target)
		if ent == nil {
			return nil, fmt.Errorf("unknown location %q", target)
		}

		if err := o.timeline.SetPlayerLocation(o.playerID, ent.ID); err != nil {
			return nil, fmt.Errorf("move failed: %w", err)
		}

		// The player is a party to their own move, so the move records them the way
		// every generated turn does.
		moveEntities := make([]entity.Mention, 0, 2)
		if o.playerID != "" {
			moveEntities = append(moveEntities, entity.Mention{ID: o.playerID, Kind: entity.MentionPlayer})
		}
		moveEntities = append(moveEntities, entity.Mention{ID: ent.ID, Kind: entity.MentionLocation})

		move := Turn{
			Number:    turnNum,
			Timestamp: time.Now(),
			Mode:      "System",
			Input:     actionInput,
			Narration: fmt.Sprintf("You make your way to %s.", ent.Name),
			Location:  ent.ID,
			Entities:  moveEntities,
		}
		if err := o.timeline.RecordTurn(&move, nil); err != nil {
			return nil, fmt.Errorf("record move: %w", err)
		}
		return &move, nil
	}

	// The opening turn establishes the campaign before the player acts. It is only
	// meaningful as turn one, so a later attempt is refused rather than silently
	// rewriting the scene the party is already standing in.
	isOpening := strings.EqualFold(mode, OpeningMode)
	if isOpening {
		if len(pastTurns) > 0 {
			return nil, fmt.Errorf("the campaign has already begun")
		}
		mode = OpeningMode
		generationPrompt = openingDirective(o.openingPrompt)
	}

	// Handle /gm director note or mode
	isCorrection := !isOpening && (mode == "GM" || strings.HasPrefix(actionInput, "/gm "))
	if isCorrection {
		directiveText := strings.TrimPrefix(actionInput, "/gm ")
		gmDirective = fmt.Sprintf("[DIRECTOR CORRECTION DIRECTIVE: %s]", directiveText)
	} else if !isOpening && strings.EqualFold(mode, "Roll") {
		if r, err := rules.EvaluateRoll(actionInput); err == nil {
			rollRes = r
			generationPrompt = fmt.Sprintf("I rolled %s with result %d", r.Notation, r.Total)
		}
	} else if !isOpening && o.rulesEngine != nil {
		// Run action through mechanics hook if available
		res, err := o.rulesEngine.ExecuteAction(strings.ToLower(mode), map[string]interface{}{
			"action": actionInput,
			"player": o.playerID,
		})
		if err == nil && res != nil {
			rollRes = res.Roll
			outcome = res.Outcome
			if res.Message != "" {
				gmDirective = fmt.Sprintf("[MECHANICS RESULT: %s]", res.Message)
			}
		}
	}

	// Where this turn happens: read from the player's own note, which is the single
	// source of truth, never from a value pinned at campaign open.
	locationID := o.currentLocation()

	// Assemble context with system rules and world lore prompts
	contextPrompt, err := o.assembler.AssembleContextWithProfiles(locationID, o.playerID, generationPrompt, o.rulesPrompt, o.lorePrompt, o.timeline.VoiceProfiles())
	if err != nil {
		return nil, fmt.Errorf("assemble context: %w", err)
	}

	if gmDirective != "" {
		contextPrompt = gmDirective + "\n\n" + contextPrompt
	}

	narration, err := o.generate(ctx, contextPrompt, onChunk)
	if err != nil {
		return nil, fmt.Errorf("gm generation failed: %w", err)
	}

	turn := Turn{
		Number:    turnNum,
		Timestamp: time.Now(),
		Mode:      mode,
		Input:     actionInput,
		Roll:      rollRes,
		Narration: narration,
		Location:  locationID,
		Outcome:   outcome,
	}

	if strings.TrimSpace(turn.Narration) == "" {
		return nil, fmt.Errorf("gm returned no narration")
	}

	turn.Entities = harness.ResolveEntityMentions(o.store, o.playerID, locationID, turn.Narration, actionInput)

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

	// A move proposed by extraction applies only when it resolves to a real
	// location, and it takes effect from the next turn: this turn happened where it
	// started, which is what keeps scenes honest.
	if ref := strings.TrimSpace(extraction.PlayerLocation); ref != "" {
		if ent := findLocationByRef(o.store, ref); ent != nil && ent.ID != locationID {
			if err := o.timeline.SetPlayerLocation(o.playerID, ent.ID); err != nil {
				return nil, fmt.Errorf("apply proposed location: %w", err)
			}
		}
	}

	// Nothing is persisted for a cancelled or failed turn: the timeline only ever
	// holds completed turns, so a disconnect is a no-op on disk.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("turn cancelled: %w", err)
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

// ProcessAction runs a turn without reporting narration as it arrives.
func (o *TurnOrchestrator) ProcessAction(ctx context.Context, mode, actionInput string) (*Turn, error) {
	return o.ProcessActionStream(ctx, mode, actionInput, nil)
}

// generate streams the GM's reply, forwarding each delta and accumulating the text.
// A provider that goes silent for longer than the chunk timeout is abandoned: the
// stream context is cancelled, the provider's goroutines are drained, and the turn
// fails without being recorded, which is what keeps a hung model from holding the
// campaign forever.
func (o *TurnOrchestrator) generate(ctx context.Context, prompt string, onChunk func(string) error) (string, error) {
	timeout := o.chunkTimeout
	if timeout <= 0 {
		timeout = defaultChunkTimeout
	}

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	chunks := make(chan harness.StreamChunk, 32)
	streamErr := make(chan error, 1)
	go func() {
		streamErr <- o.router.StreamForRole(streamCtx, "gm", harness.GenerateRequest{Prompt: prompt}, chunks)
	}()

	idle := time.NewTimer(timeout)
	defer idle.Stop()

	var sb strings.Builder
	for {
		select {
		case <-idle.C:
			cancel()
			// Draining until the provider closes lets its goroutines exit rather
			// than block forever on a channel nobody reads.
			go func() {
				for range chunks {
				}
			}()
			<-streamErr
			return "", fmt.Errorf("%w after %s", ErrGenerationStalled, timeout)

		case chunk, ok := <-chunks:
			if !ok {
				if err := <-streamErr; err != nil {
					return "", err
				}
				return sb.String(), nil
			}
			if chunk.Error != nil {
				<-streamErr
				return "", chunk.Error
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(timeout)

			if chunk.Text == "" {
				continue
			}
			sb.WriteString(chunk.Text)
			if onChunk != nil {
				if err := onChunk(chunk.Text); err != nil {
					return "", err
				}
			}
		}
	}
}
