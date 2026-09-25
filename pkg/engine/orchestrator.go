package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// ErrGenerationStalled reports that the gm provider stopped sending deltas for
// longer than the configured chunk timeout.
var ErrGenerationStalled = errors.New("gm generation stalled")

// errStreamListener marks a failure caused by the caller's onChunk listener, so
// a fallback provider is never tried on top of a disconnected client.
var errStreamListener = errors.New("stream listener failed")

// OpeningPromptSetting is the campaign setting holding the player's own opening
// instruction. Absent means the GM invents the scene, which is the default.
const OpeningPromptSetting = "opening_prompt"

// OpeningMode is the reserved mode of a campaign's first turn.
const OpeningMode = "Opening"

// defaultChunkTimeout is the silence tolerated between deltas when a caller sets none.
const defaultChunkTimeout = 60 * time.Second

// defaultThreadsMax caps how many open threads the prompt carries when unset.
const defaultThreadsMax = 8

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
	store            *storage.Store
	timeline         *Timeline
	rulesEngine      *rules.JSEngine
	router           *harness.Router
	startLocation    string
	playerID         string
	assembler        *harness.ContextAssembler
	rulesPrompt      string
	lorePrompt       string
	extractor        *harness.Extractor
	chunkTimeout     time.Duration
	openingPrompt    string
	logger           trace.Logger
	chronicler       *Chronicler
	threadsMax       int
	continuityChecks *bool
	completion       harness.ModelProvider
	completionPolicy CompletionPolicy
	toolExecutor     ToolExecutor
	checkResolver    harness.CheckResolver
	declaredStats    map[string]core.StatSpec
	allowFreeform    bool
	toolCapability   string
	toolRounds       int
	toolObserver     func(ToolActivity)
	speechCues       harness.SpeechCueContext
}

// SetSpeechCues sets the vocal steering hints passed to the GM prompt.
func (o *TurnOrchestrator) SetSpeechCues(cues harness.SpeechCueContext) {
	o.speechCues = cues
}

// ToolExecutor runs one tool call and returns the text a model will read. A
// failed call still returns readable text, so a tool error never loses a turn.
type ToolExecutor interface {
	Execute(ctx context.Context, call harness.ToolCall) (result string, ok bool)
}

// ToolActivity is one step of tool activity a client can render as it happens.
type ToolActivity struct {
	Round   int
	Name    string
	Status  string // "running" or "done"
	Summary string
}

// SetTools attaches the executor and the role's declared capability: "auto",
// "yes", or "no".
func (o *TurnOrchestrator) SetTools(executor ToolExecutor, capability string) {
	o.toolExecutor = executor
	o.toolCapability = capability
}

// SetToolRounds caps the tool rounds in one turn. Zero or negative means unbounded
// (with a 100-round runaway safety ceiling).
func (o *TurnOrchestrator) SetToolRounds(rounds int) {
	if rounds < 0 {
		rounds = 0
	}
	o.toolRounds = rounds
}

// SetCheckResolver sets how request_check is resolved. A nil resolver restores
// the deterministic default.
func (o *TurnOrchestrator) SetCheckResolver(resolver harness.CheckResolver) {
	if resolver == nil {
		o.checkResolver = defaultCheckResolver{}
		return
	}
	o.checkResolver = resolver
}

// SetDeclaredStats sets the mechanics schema's declared stats, used to validate
// state changes. Nil means the system declares no stats.
func (o *TurnOrchestrator) SetDeclaredStats(stats map[string]core.StatSpec) {
	o.declaredStats = stats
}

// SetAllowFreeformState permits state changes to undeclared paths even when the
// system declares stats.
func (o *TurnOrchestrator) SetAllowFreeformState(allow bool) {
	o.allowFreeform = allow
}

// checkResolverOrDefault returns the configured resolver.
func (o *TurnOrchestrator) resolveCheck(ctx context.Context, req harness.CheckRequest, actor *entity.Entity) (*harness.CheckResult, error) {
	resolver := o.checkResolver
	if resolver == nil {
		resolver = defaultCheckResolver{}
	}
	return resolver.Resolve(ctx, req, actor)
}

// SetToolObserver receives tool activity as it happens, so a client can show it
// rather than waiting in silence.
func (o *TurnOrchestrator) SetToolObserver(observer func(ToolActivity)) {
	o.toolObserver = observer
}

const defaultUnboundedToolRounds = 100

func (o *TurnOrchestrator) toolRoundCap() int {
	if o.toolRounds <= 0 {
		return defaultUnboundedToolRounds
	}
	return o.toolRounds
}

// offersTools decides whether to offer a tool surface to a provider. isCaller is
// whether the provider implements harness.ToolCaller; capability "auto" follows
// it, "yes" forces, and "no" suppresses.
func (o *TurnOrchestrator) offersTools(isCaller bool) bool {
	if o.toolExecutor == nil {
		return false
	}
	switch o.toolCapability {
	case "yes":
		return true
	case "no":
		return false
	default:
		return isCaller
	}
}

func NewTurnOrchestrator(
	store *storage.Store,
	timeline *Timeline,
	rulesEngine *rules.JSEngine,
	router *harness.Router,
	startLocation string,
	playerID string,
) *TurnOrchestrator {
	o := &TurnOrchestrator{
		store:         store,
		timeline:      timeline,
		rulesEngine:   rulesEngine,
		router:        router,
		startLocation: startLocation,
		playerID:      playerID,
		assembler:     harness.NewContextAssembler(store),
	}
	// A loaded rules engine resolves checks from the system's declared schema and
	// its own js resolvers; otherwise the deterministic default stands in.
	if rulesEngine != nil {
		o.checkResolver = rulesEngine
	}
	return o
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

// SetLogger attaches a trace sink. A nil logger records nothing. The assembler is
// told too, because it emits context.assembled from inside itself.
func (o *TurnOrchestrator) SetLogger(logger trace.Logger) {
	o.logger = trace.OrNil(logger)
	o.assembler.SetLogger(o.logger)
}

// ContextLimits reports the limits the assembler is using, so a caller can prove
// configuration reached it rather than assuming it did.
func (o *TurnOrchestrator) ContextLimits() harness.ContextLimits {
	return o.assembler.Limits()
}

// SetChronicler gives the orchestrator the campaign's long memory. The summary is
// read at assembly time rather than cached, because the chronicle is written by
// another goroutine and a turn must use whatever was current when it started.
func (o *TurnOrchestrator) SetChronicler(chronicler *Chronicler) {
	o.chronicler = chronicler
}

// gameID is the campaign this orchestrator plays, which the chronicle belongs to.
func (o *TurnOrchestrator) gameID() string {
	if o.timeline == nil {
		return ""
	}
	return o.timeline.GameID()
}

// SetContextLimits applies the configured prompt budget and recall window.
func (o *TurnOrchestrator) SetContextLimits(limits harness.ContextLimits) {
	o.assembler.SetLimits(limits)
}

// SetOpeningPrompt supplies the player's own opening instruction, used when the
// campaign's first turn runs in OpeningMode.
func (o *TurnOrchestrator) SetOpeningPrompt(prompt string) {
	o.openingPrompt = prompt
}

// SetThreadsMax caps how many open threads the prompt carries.
func (o *TurnOrchestrator) SetThreadsMax(max int) {
	o.threadsMax = max
}

// threadsCap is the configured cap, or a sane one when nothing set it.
func (o *TurnOrchestrator) threadsCap() int {
	if o.threadsMax <= 0 {
		return defaultThreadsMax
	}
	return o.threadsMax
}

// SetContinuityChecks turns the deterministic continuity pass on or off.
func (o *TurnOrchestrator) SetContinuityChecks(enabled bool) {
	o.continuityChecks = &enabled
}

// continuityEnabled reports whether the pass should run.
func (o *TurnOrchestrator) continuityEnabled() bool {
	return o.continuityChecks != nil && *o.continuityChecks
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

	o.logger = trace.OrNil(o.logger)
	o.logger.Event("turn.begin", map[string]interface{}{
		"number":      turnNum,
		"mode":        mode,
		"input_chars": len([]rune(actionInput)),
	})

	ctx, turnSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "turn",
		oteltrace.WithAttributes(
			attribute.String("game.id", o.gameID()),
			attribute.Int("turn.number", turnNum),
			attribute.String("turn.mode", mode),
		),
	)
	defer turnSpan.End()
	turnStarted := time.Now()

	var rollRes *rules.RollResult
	var outcome string
	var gmDirective string
	generationPrompt := actionInput

	// The outcome is only known later, so it is attached at return along with
	// the turn's duration and completion metrics.
	defer func() {
		turnSpan.SetAttributes(attribute.String("turn.outcome", outcome))
		metrics := engineMetrics()
		attributes := otelmetric.WithAttributes(
			attribute.String("turn.mode", mode),
			attribute.String("turn.outcome", outcome),
		)
		metrics.turnDuration.Record(context.Background(), float64(time.Since(turnStarted).Milliseconds()), attributes)
		metrics.turnCompleted.Add(context.Background(), 1, otelmetric.WithAttributes(
			attribute.String("turn.mode", mode),
			attribute.String("turn.outcome", outcome),
		))
	}()

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

	// /recap answers "where were we?". A stale summary is brought up to date first,
	// because asking for a recap is asking about the present tense and a summary can
	// be a cadence behind. The regeneration is the same path a turn uses, so a
	// failure leaves the old summary in place rather than failing the command.
	if strings.HasPrefix(strings.TrimSpace(actionInput), "/recap") {
		chronicle := Chronicle{}
		if o.chronicler != nil {
			if due, err := o.chronicler.Due(o.gameID()); err == nil && due {
				if _, err := o.chronicler.Regenerate(ctx, o.gameID()); err != nil {
					o.logger = trace.OrNil(o.logger)
					o.logger.Event("provider.error", map[string]interface{}{"role": "summariser", "error": err.Error()})
				}
			}
			chronicle, _ = o.chronicler.Recap(o.gameID())
		}

		narration := "This campaign has not turned far enough for a recap yet."
		if strings.TrimSpace(chronicle.Summary) != "" {
			narration = fmt.Sprintf("## Story So Far (through turn %d)\n\n%s", chronicle.ThroughTurn, chronicle.Summary)
		}

		return &Turn{
			Number:    turnNum,
			Timestamp: time.Now(),
			Mode:      "System",
			Input:     "/recap",
			Narration: narration,
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
		if err := o.timeline.RecordTurnContext(ctx, &move, nil); err != nil {
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
		// A player-initiated roll is a proposal, not an executed result: the GM
		// either adopts it with request_check or dismisses it, and its decision is
		// authoritative. This is the player pre-empting being asked to roll.
		proposed := strings.TrimSpace(actionInput)
		if proposed == "" {
			proposed = "a check"
		}
		gmDirective = fmt.Sprintf("[PROPOSED CHECK: %s by %s]", proposed, o.playerID)
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

	// Assemble context with system rules, world lore prompts, and a window of
	// recent turns, which is what keeps the narrator in the same conversation.
	recent := make([]harness.RecentTurn, 0, len(pastTurns))
	for _, turn := range pastTurns {
		recent = append(recent, harness.RecentTurn{
			Number:    turn.Number,
			Mode:      turn.Mode,
			Input:     turn.Input,
			Narration: turn.Narration,
		})
	}

	// Long memory is a recollection, not canon, so it is injected as one and canon
	// wins wherever they disagree.
	summary := ""
	summaryVersion := 0
	if o.chronicler != nil {
		if chronicle, err := o.chronicler.Recap(o.gameID()); err == nil {
			summary = chronicle.Summary
			summaryVersion = chronicle.ThroughTurn
		}
	}

	// Open threads are canon, so they are built whether or not anything in the scene
	// touches them: a thread the party has walked away from is the one most likely to
	// be forgotten.
	threads := make([]string, 0)
	if open, err := OpenThreads(o.store, turnNum); err == nil {
		for index, thread := range open {
			if index == o.threadsCap() {
				break
			}
			threads = append(threads, fmt.Sprintf("%s (%s, last advanced %d turns ago)", thread.Name, thread.Status, thread.Idle))
		}
	}

	var workingSet WorkingSet
	if o.store != nil {
		if records, err := o.store.LoadWorkingSet(); err == nil && len(records) > 0 {
			workingSet.FromStorageRecords(records)
		}
	}
	workingSetSelection := workingSet.Select(8)

	isCaller := false
	if gmProvider, err := o.router.GetProviderForRole("gm"); err == nil && gmProvider != nil {
		if caller, ok := gmProvider.(harness.ToolCaller); ok {
			isCaller = caller.ToolCallerCapable()
		}
	}
	canCallTools := o.offersTools(isCaller)

	assembly, err := o.assembler.Assemble(harness.ContextRequest{
		Context:          ctx,
		LocationID:       locationID,
		PlayerID:         o.playerID,
		Action:           generationPrompt,
		RulesPrompt:      o.rulesPrompt,
		LorePrompt:       o.lorePrompt,
		Profiles:         o.timeline.VoiceProfiles(),
		OmitVoiceCatalog: canCallTools,
		Recent:           recent,
		TurnNumber:       turnNum,
		Mode:             mode,
		Summary:          summary,
		SummaryVersion:   summaryVersion,
		WorkingSet:       workingSetSelection,
		Threads:          threads,
		SpeechCues:       o.speechCues,
	})
	if err != nil {
		return nil, fmt.Errorf("assemble context: %w", err)
	}

	if o.rulesEngine != nil {
		if err := o.rulesEngine.ExecuteTurnBegin(map[string]interface{}{
			"turn":     turnNum,
			"location": locationID,
		}); err != nil {
			o.logger.Event("turn.begin_hook_error", map[string]interface{}{"error": err.Error()})
		}
	}

	turnSpan.SetAttributes(attribute.String("turn.assembled_prompt", assembly.Prompt))
	result, err := o.runGenerationLoop(ctx, &assembly, gmDirective, onChunk)
	if err != nil {
		outcome = "error"
		turnSpan.SetAttributes(attribute.String("turn.raw_completion", result.Text))
		failure, _ := harness.FailureFrom(err)
		fields := map[string]interface{}{
			"code":          generationCode(failure),
			"error":         err.Error(),
			"role":          "gm",
			"provider":      result.ProviderID,
			"prompt_chars":  len([]rune(actionInput)),
			"context_chars": len([]rune(assembly.Prompt)),
			"elapsed_ms":    time.Since(turnStarted).Milliseconds(),
			"chunk_count":   result.ChunkCount,
			"partial_chars": len([]rune(result.Text)),
		}
		if failure != nil {
			fields["attempts"] = len(failure.Attempts)
			turnSpan.SetAttributes(
				attribute.String("turn.failure_code", string(failure.Code)),
				attribute.String("turn.failure_message", failure.Message),
				attribute.String("localrpg.generation.failure_code", string(failure.Code)),
				attribute.Int("localrpg.generation.attempts", len(failure.Attempts)),
			)
			turnSpan.RecordError(failure)
			turnSpan.SetStatus(codes.Error, string(failure.Code))
		} else {
			code := string(harness.ClassifyProviderError(err))
			turnSpan.SetAttributes(attribute.String("turn.failure_code", code))
			turnSpan.SetStatus(codes.Error, code)
			turnSpan.RecordError(err)
		}
		if result.Failure != nil {
			fields["finish_reason"] = result.Failure.FinishReason
		}
		o.logger.Event("generation.error", fields)
		return nil, fmt.Errorf("gm generation failed: %w", err)
	}

	turnSpan.SetAttributes(
		attribute.String("turn.raw_completion", result.Text),
		attribute.String("localrpg.context.strategy", string(assembly.Context.Strategy)),
		attribute.String("localrpg.context.prefix_hash", assembly.Context.PrefixHash),
		attribute.Int("localrpg.provider.cached_tokens", assembly.Context.CachedTokens),
	)
	if strings.TrimSpace(result.Text) == "" && len(result.ToolCalls) == 0 {
		turnSpan.SetAttributes(attribute.String("turn.failure_code", "empty_response"))
	}
	if assembly.Context.Session != nil {
		turnSpan.SetAttributes(attribute.String("localrpg.context.session_id", assembly.Context.Session.ID))
	}

	contextPrompt := assembly.Prompt
	if gmDirective != "" {
		contextPrompt = gmDirective + "\n\n" + contextPrompt
	}

	if result.FallbackReason != "" {
		o.logger.Event("turn.protocol_fallback", map[string]interface{}{"reason": result.FallbackReason})
	}

	cause := cutNone
	var narration string
	var recovery RecoveryOutcome
	var stillIncomplete bool
	if result.Submission == nil {
		cause = o.classifyCut(result)
		narration, recovery, stillIncomplete = o.recoverReply(ctx, result.Text, cause, onChunk)
		if strings.TrimSpace(narration) == "" {
			outcome = "error"
			failure := &harness.GenerationFailure{
				Code:         harness.FailureEmptyResponse,
				Message:      "gm returned no narration",
				FinishReason: result.FinishReason,
				ElapsedMS:    time.Since(turnStarted).Milliseconds(),
			}
			if result.Failure != nil {
				failure = result.Failure
			}
			turnSpan.SetAttributes(attribute.String("localrpg.generation.failure_code", string(failure.Code)))
			turnSpan.RecordError(failure)
			turnSpan.SetStatus(codes.Error, string(failure.Code))
			o.logger.Event("generation.error", map[string]interface{}{
				"code":          string(failure.Code),
				"error":         failure.Message,
				"role":          "gm",
				"provider":      result.ProviderID,
				"prompt_chars":  len([]rune(actionInput)),
				"elapsed_ms":    time.Since(turnStarted).Milliseconds(),
				"chunk_count":   result.ChunkCount,
				"partial_chars": len([]rune(result.Text)),
				"attempts":      len(failure.Attempts),
				"finish_reason": result.FinishReason,
			})
			return nil, failure
		}
	}

	o.logger.Event("generation.complete", map[string]interface{}{
		"narration_chars": len([]rune(narration)),
		"finish_reason":   result.FinishReason,
		"cause":           cause.String(),
		"recovery":        string(recovery),
		"truncated":       stillIncomplete,
	})

	turn := Turn{
		Number:       turnNum,
		Timestamp:    time.Now(),
		Mode:         mode,
		Input:        actionInput,
		Roll:         rollRes,
		Narration:    narration,
		Location:     locationID,
		Outcome:      outcome,
		Truncated:    stillIncomplete,
		Recovery:     string(recovery),
		ContextNotes: assembly.Trimmed,
		Context:      &assembly.Context,
		Prompt:       contextPrompt,
		ToolCalls:    result.Provenance,
	}

	turn.Entities = harness.ResolveEntityMentions(o.store, o.playerID, locationID, turn.Narration, actionInput)

	structured := result.Submission != nil
	extraction := harness.Extraction{}
	var personae []harness.PersonaDecl
	var memories []harness.MemoryDecl
	if structured {
		extraction = extractionFromSubmission(result.Submission)
		personae = result.Submission.Personae
		memories = result.Submission.Memories
		turn.Verdict = &result.Submission.Verdict
		turn.Rejected = result.Submission.Verdict.Feasibility == harness.FeasibilityImpossible
		turn.Checks = result.Checks
		for _, persona := range personae {
			if id := entity.Slugify(persona.Name); id != "" {
				turn.Personae = append(turn.Personae, id)
			}
		}
	} else if o.extractor != nil {
		extractCtx, extractSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "extract.entities")
		// A failed extractor must not lose the turn; the mentions above still stand.
		if extracted, err := o.extractor.Extract(extractCtx, turn.Narration); err == nil {
			extraction = *extracted
		} else {
			extractSpan.RecordError(err)
			extractSpan.SetStatus(codes.Error, string(harness.ClassifyProviderError(err)))
			o.logger.Event("extract.error", map[string]interface{}{"error": err.Error()})
		}
		extractSpan.SetAttributes(
			attribute.Int("localrpg.entities.extracted", len(extraction.Entities)),
		)
		extractSpan.End()
	}

	if structured {
		narrationText, segs := buildSegments(result.Submission, o.speakerResolver(result.Submission))
		if narrationText != "" {
			turn.Narration = narrationText
		}
		turn.Segments = segs
	} else {
		turn.Segments = buildTurnSegments(o.store, turn.Narration, extraction)
	}

	// The player's own spoken line leads the turn, so it is heard in their voice
	// before the narrator answers. If the narrator's generated text already begins
	// with the player's line, attachPlayerSegment marks it rather than duplicating it.
	turn.Segments = attachPlayerSegment(turn.Segments, mode, actionInput, o.playerID, o.playerDisplayName())

	o.logger.Event("segment.build", map[string]interface{}{
		"count":      len(turn.Segments),
		"kinds":      segmentKinds(turn.Segments),
		"speakers":   segmentSpeakers(turn.Segments),
		"unresolved": unresolvedSpeakers(turn.Segments),
	})
	for _, mention := range speechMentions(turn.Segments) {
		if !containsMention(turn.Entities, mention.ID) {
			turn.Entities = append(turn.Entities, mention)
		}
	}

	if o.continuityEnabled() {
		_, continuitySpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "continuity.check")
		findings := CheckContinuity(ContinuityInput{
			Store:      o.store,
			Turn:       &turn,
			LocationID: locationID,
			PlayerID:   o.playerID,
			Context:    assembly.Context,
			WorkingSet: &workingSet,
		})
		for _, finding := range findings {
			turn.ContinuityNotes = append(turn.ContinuityNotes, finding.Note)
		}
		continuitySpan.SetAttributes(attribute.Int("localrpg.continuity.findings", len(findings)))
		continuitySpan.End()
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

	// MatchExistingEntity answers whether a note already exists, so a turn's trace
	// can say which beings it introduced rather than only which it mentioned.
	matched := make([]string, 0, len(extraction.Entities))
	created := make([]string, 0, len(extraction.Entities))
	for i := range extraction.Entities {
		proposed := extraction.Entities[i]
		if existing := harness.MatchExistingEntity(o.store, &proposed); existing != nil {
			matched = append(matched, existing.ID)
			continue
		}
		id := proposed.ID
		if id == "" {
			id = entity.Slugify(proposed.Name)
		}
		created = append(created, id)
	}
	o.logger.Event("extraction.reconcile", map[string]interface{}{
		"matched": matched,
		"created": created,
	})

	// RecordTurn creates and voices the entities the turn introduced. Synthesis
	// must not begin until this returns, or a character invented in this turn
	// would be read in the narrator's voice.
	if structured && len(result.Submission.StateChanges) > 0 && o.rulesEngine != nil {
		if err := rules.ApplyStateChanges(o.rulesEngine.HostAPI(), result.Submission.StateChanges, o.declaredStats, o.allowFreeform); err != nil {
			return nil, fmt.Errorf("apply state changes: %w", err)
		}
	}

	if err := o.timeline.RecordTurnContextStructured(ctx, &turn, extraction.Entities, personae, memories, result.Checks); err != nil {
		return nil, fmt.Errorf("record turn: %w", err)
	}

	if exec, ok := o.toolExecutor.(interface{ AssignedVoices() map[string]config.VoiceProfile }); ok {
		assigned := exec.AssignedVoices()
		for _, persona := range personae {
			slugID := entity.Slugify(persona.Name)
			if prof, has := assigned[slugID]; has {
				if ent, err := o.store.GetEntity(slugID); err == nil && ent != nil {
					ent.Voice = &entity.VoiceConfig{
						Provider:   prof.Provider,
						VoiceID:    prof.VoiceID,
						Pitch:      prof.Pitch,
						SpeechRate: prof.SpeechRate,
						Options:    prof.Options,
					}
					_ = o.timeline.SaveEntity(ent)
				}
			}
		}
	}

	if o.store != nil {
		var allTurnRefs []harness.Ref
		if turn.Context != nil {
			allTurnRefs = turn.Context.Refs
		}
		workingSet.Apply(turnNum, allTurnRefs)
		_ = o.store.ReplaceWorkingSet(workingSet.ToStorageRecords())
	}

	trace.LogEvent(ctx, o.logger, "record.turn", map[string]interface{}{
		"number":          turn.Number,
		"location":        turn.Location,
		"entities":        len(turn.Entities),
		"outcome":         turn.Outcome,
		"narration_chars": len([]rune(turn.Narration)),
	})

	// Trigger post-turn hooks
	if o.rulesEngine != nil {
		entityIDs := make([]string, 0, len(turn.Entities))
		for _, mention := range turn.Entities {
			entityIDs = append(entityIDs, mention.ID)
		}
		hookCtx := map[string]interface{}{
			"turn":      turnNum,
			"narration": turn.Narration,
			"entities":  entityIDs,
			"checks":    len(turn.Checks),
		}
		if turn.Verdict != nil {
			hookCtx["verdict"] = string(turn.Verdict.Feasibility)
		}
		if err := o.rulesEngine.ExecuteTurnEnd(hookCtx); err != nil {
			o.logger.Event("turn.end_hook_error", map[string]interface{}{"error": err.Error()})
		}
	}

	return &turn, nil
}

// ProcessAction runs a turn without reporting narration as it arrives.
func (o *TurnOrchestrator) ProcessAction(ctx context.Context, mode, actionInput string) (*Turn, error) {
	return o.ProcessActionStream(ctx, mode, actionInput, nil)
}

// generate streams the GM's reply, forwarding each delta and accumulating the text.
// It returns the provider's finish reason alongside the text, so a reply cut off by
// a token limit can be marked as truncated rather than presented as complete. A
// provider that goes silent for longer than the chunk timeout is abandoned: the
// stream context is cancelled, the provider's goroutines are drained, and the turn
// fails without being recorded, which is what keeps a hung model from holding the
// campaign forever.
// streamResult is what one provider stream produced: its accumulated text, the
// provider's finish reason when it reported one, and the failure that stopped it
// after some text had already arrived.
type streamResult struct {
	Text         string
	FinishReason string
	Interrupted  error
	ToolCalls    []harness.ToolCall
	// Provenance is what the turn looked up, compactly: name and result size.
	// Arguments and results live in the trace, not in the campaign's history.
	Provenance []ToolCallRecord
	// Failure is the bounded generation failure when the stream produced no
	// usable text. ProviderID names the provider that failed. ChunkCount is how
	// many non-empty chunks arrived, for diagnostics.
	Failure    *harness.GenerationFailure
	ProviderID string
	ChunkCount int
	// Submission is the structured turn when the GM called submit_turn, and
	// Checks are the checks it resolved with request_check. FallbackReason is set
	// when a structured turn failed validation twice and prose was used instead.
	Submission     *harness.TurnSubmission
	Checks         []harness.CheckResult
	FallbackReason string
}

// generationCode reads a bounded failure code for logging, defaulting to a
// provider error when the failure is not typed.
func generationCode(failure *harness.GenerationFailure) string {
	if failure == nil {
		return string(harness.FailureProviderError)
	}
	return string(failure.Code)
}

// stream pumps one provider stream, forwarding each delta to onChunk and
// accumulating the text. A failure after text has arrived is returned as
// Interrupted with the text intact, so a turn can be repaired rather than lost;
// a failure before any text is a hard failure, as is a listener error.
func (o *TurnOrchestrator) stream(ctx context.Context, provider harness.ModelProvider, req harness.GenerateRequest, onChunk func(string) error) (streamResult, error) {
	timeout := o.chunkTimeout
	if timeout <= 0 {
		timeout = defaultChunkTimeout
	}

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	started := time.Now()

	chunks := make(chan harness.StreamChunk, 32)
	streamErr := make(chan error, 1)
	go func() {
		streamErr <- provider.Stream(streamCtx, req, chunks)
	}()

	idle := time.NewTimer(timeout)
	defer idle.Stop()

	var sb strings.Builder
	finishReason := ""
	var toolCalls []harness.ToolCall
	sawText := false
	chunkCount := 0
	interrupted := func(err error) (streamResult, error) {
		result := streamResult{ProviderID: provider.ID(), ChunkCount: chunkCount}
		if sawText {
			result.Text = sb.String()
			result.FinishReason = finishReason
			result.Interrupted = err
			return result, nil
		}
		code := harness.ClassifyProviderError(err)
		if errors.Is(err, ErrGenerationStalled) {
			code = harness.FailureTimeout
		}
		result.Failure = &harness.GenerationFailure{
			Code:      code,
			Message:   err.Error(),
			ElapsedMS: time.Since(started).Milliseconds(),
			Cause:     err,
		}
		return result, err
	}

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
			return interrupted(fmt.Errorf("%w after %s", ErrGenerationStalled, timeout))

		case chunk, ok := <-chunks:
			if !ok {
				if err := <-streamErr; err != nil {
					// A cancelled client is final: the turn will be discarded, so
					// there is no point keeping text nobody is waiting for.
					if errors.Is(err, context.Canceled) {
						return streamResult{}, err
					}
					return interrupted(err)
				}
				if strings.TrimSpace(sb.String()) == "" && len(toolCalls) == 0 {
					return streamResult{
						ProviderID: provider.ID(),
						ChunkCount: chunkCount,
						Failure: &harness.GenerationFailure{
							Code:      harness.FailureEmptyResponse,
							Message:   "provider returned no text",
							ElapsedMS: time.Since(started).Milliseconds(),
						},
					}, nil
				}
				return streamResult{Text: sb.String(), FinishReason: finishReason, ToolCalls: toolCalls, ProviderID: provider.ID(), ChunkCount: chunkCount}, nil
			}
			if chunk.Error != nil {
				<-streamErr
				return interrupted(chunk.Error)
			}
			if len(chunk.ToolCalls) > 0 {
				toolCalls = chunk.ToolCalls
			}
			if chunk.FinishReason != "" {
				finishReason = chunk.FinishReason
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(timeout)

			if chunk.Text != "" || len(chunk.ToolCalls) > 0 {
				chunkCount++
			}

			if chunk.Text == "" {
				continue
			}
			sb.WriteString(chunk.Text)
			sawText = true
			if onChunk != nil {
				if err := onChunk(chunk.Text); err != nil {
					return streamResult{}, fmt.Errorf("%w: %w", errStreamListener, err)
				}
			}
		}
	}
}

// generate streams the gm reply through stream, falling back to the configured
// fallback provider when the primary fails before producing any text.
func (o *TurnOrchestrator) generate(ctx context.Context, prompt string, onChunk func(string) error) (streamResult, error) {
	return o.generateRequest(ctx, harness.GenerateRequest{Prompt: prompt}, onChunk)
}

// generateRequest streams one request through the gm role, falling back to the
// configured fallback provider when the primary fails before producing any text.
func (o *TurnOrchestrator) generateRequest(ctx context.Context, req harness.GenerateRequest, onChunk func(string) error) (streamResult, error) {
	provider, err := o.router.GetProviderForRole("gm")
	if err != nil {
		return streamResult{
			Failure: &harness.GenerationFailure{
				Code:    harness.FailureProviderUnavailable,
				Message: err.Error(),
			},
		}, err
	}

	result, err := o.stream(ctx, provider, req, onChunk)
	if errors.Is(err, errStreamListener) || ctx.Err() != nil {
		return result, err
	}
	// A hard error or an empty reply both justify the fallback.
	if err == nil && result.Failure == nil {
		return result, nil
	}

	primaryFailure := result.Failure
	if primaryFailure == nil {
		primaryFailure = &harness.GenerationFailure{
			Code:    harness.ClassifyProviderError(err),
			Message: err.Error(),
			Cause:   err,
		}
		result.Failure = primaryFailure
	}

	if fallback, ok := o.router.FallbackForRole("gm"); ok {
		fallbackResult, fallbackErr := o.stream(ctx, fallback, req, onChunk)
		if fallbackErr == nil && fallbackResult.Failure == nil {
			return fallbackResult, nil
		}
		switch {
		case fallbackResult.Failure != nil:
			primaryFailure.Attempts = append(primaryFailure.Attempts, harness.Attempt{
				Role: "gm", Provider: fallbackResult.ProviderID,
				Code: fallbackResult.Failure.Code, Detail: fallbackResult.Failure.Message,
			})
		case fallbackErr != nil:
			primaryFailure.Attempts = append(primaryFailure.Attempts, harness.Attempt{
				Role: "gm", Provider: fallback.ID(),
				Code: harness.ClassifyProviderError(fallbackErr), Detail: fallbackErr.Error(),
			})
		}
	}
	if len(primaryFailure.Attempts) == 0 {
		primaryFailure.Attempts = []harness.Attempt{{
			Role: "gm", Provider: result.ProviderID, Code: primaryFailure.Code, Detail: primaryFailure.Message,
		}}
	}
	return result, primaryFailure
}

// runGenerationLoop runs the turn as a bounded conversation. It offers tools only
// while the role can call them, the rounds are not spent, and the conversation is
// within budget; otherwise the model is told tools are unavailable and asked to
// answer. The result is the last reply that carried no tool calls. With no
// executor attached the loop makes exactly one call, shaped as it was before
// tools existed, so nothing changes for a provider that cannot call them.
func (o *TurnOrchestrator) runGenerationLoop(ctx context.Context, assembly *harness.AssembleResult, gmDirective string, onChunk func(string) error) (streamResult, error) {
	contextPrompt := assembly.Prompt
	if gmDirective != "" {
		contextPrompt = gmDirective + "\n\n" + contextPrompt
	}

	provider, err := o.router.GetProviderForRole("gm")
	if err != nil {
		return streamResult{}, err
	}

	caps := harness.Describe(provider)
	modelName := provider.ID()
	if m, ok := provider.(interface{ Model() string }); ok && m.Model() != "" {
		modelName = m.Model()
	}

	turnNum := assembly.Context.TurnNumber
	var storedSession *harness.ProviderSession
	if turnNum > 1 && o.store != nil {
		if raw, _, err := o.store.GetTurnContext(turnNum - 1); err == nil && len(raw) > 0 {
			var prevCtx harness.TurnContext
			if err := json.Unmarshal(raw, &prevCtx); err == nil && prevCtx.Session != nil {
				storedSession = prevCtx.Session
			}
		}
	}

	isCaller := false
	if caller, ok := provider.(harness.ToolCaller); ok {
		isCaller = caller.ToolCallerCapable()
	}
	canCallTools := o.offersTools(isCaller)

	strategy := harness.SelectStrategy(caps, storedSession, turnNum-1, assembly.Context.PrefixHash, modelName)
	if canCallTools && strategy == harness.StrategyServerSession {
		if caps.ContextCache && assembly.Context.PrefixHash != "" {
			strategy = harness.StrategyCachedPrefix
		} else {
			strategy = harness.StrategyFullPrompt
		}
	}
	assembly.Context.Strategy = strategy

	// Server session strategy: try ContinueSession when tools are not offered
	if !canCallTools && strategy == harness.StrategyServerSession {
		if sessProvider, ok := provider.(harness.SessionProvider); ok && storedSession != nil {
			deltaPrompt := assembly.DeltaPrompt
			if gmDirective != "" {
				deltaPrompt = gmDirective + "\n\n" + deltaPrompt
			}
			deltaReq := harness.GenerateRequest{
				Prompt: deltaPrompt,
				System: o.rulesPrompt,
			}
			handle := &harness.SessionHandle{
				ID:          storedSession.ID,
				ThroughTurn: storedSession.ThroughTurn,
			}
			resp, err := sessProvider.ContinueSession(ctx, handle, deltaReq)
			if err == nil {
				if onChunk != nil && resp.Text != "" {
					_ = onChunk(resp.Text)
				}
				assembly.Context.CachedTokens = resp.CachedTokens
				newID := resp.SessionID
				if newID == "" {
					newID = storedSession.ID
				}
				assembly.Context.Session = &harness.ProviderSession{
					Provider:    provider.ID(),
					ID:          newID,
					ThroughTurn: turnNum,
					Model:       modelName,
					PrefixHash:  assembly.Context.PrefixHash,
				}
				return streamResult{Text: resp.Text}, nil
			}
			// Session continuation failed: fallback to full_prompt
			o.logger.Event("context.session_fallback", map[string]interface{}{
				"error":    err.Error(),
				"from":     string(harness.StrategyServerSession),
				"fallback": string(harness.StrategyFullPrompt),
			})
			assembly.Context.Strategy = harness.StrategyFullPrompt
		}
	}

	// Cached prefix strategy: ensure cache
	if strategy == harness.StrategyCachedPrefix {
		if cacher, ok := provider.(harness.ContextCacher); ok {
			_, err := cacher.EnsureCache(ctx, assembly.PrefixPrompt, 1*time.Hour)
			if err != nil {
				o.logger.Event("context.cache_fallback", map[string]interface{}{
					"error":    err.Error(),
					"fallback": string(harness.StrategyFullPrompt),
				})
				assembly.Context.Strategy = harness.StrategyFullPrompt
			}
		}
	}

	// If provider supports sessions, try StartSession when starting or falling back (only when tools are not offered)
	if !canCallTools {
		if sessProvider, ok := provider.(harness.SessionProvider); ok {
			fullReq := harness.GenerateRequest{
				Prompt: contextPrompt,
				System: o.rulesPrompt,
			}
			handle, err := sessProvider.StartSession(ctx, fullReq)
			if err == nil && handle != nil {
				text := ""
				if handle.Response != nil {
					text = handle.Response.Text
					if onChunk != nil && text != "" {
						_ = onChunk(text)
					}
				}
				assembly.Context.CachedTokens = handle.CachedTokens
				assembly.Context.Session = &harness.ProviderSession{
					Provider:    provider.ID(),
					ID:          handle.ID,
					ThroughTurn: turnNum,
					Model:       modelName,
					PrefixHash:  assembly.Context.PrefixHash,
				}
				if text != "" {
					return streamResult{Text: text}, nil
				}
			}
		}
	}

	// One user turn carrying the assembled context keeps the request identical to
	// the single-prompt path for a provider that ignores messages.
	messages := []harness.Message{{Role: "user", Content: contextPrompt}}

	var provenance []ToolCallRecord
	var checks []harness.CheckResult
	submitAttempts := 0
	withdrawn := false

	for round := 0; round <= o.toolRoundCap(); round++ {
		offerTools := canCallTools && round < o.toolRoundCap() && !withdrawn && !o.overBudget(messages)

		// Prompt is kept for a caller or provider that only reads a string: it is
		// the assembled context, exactly as the single-prompt path sent it, so
		// existing behaviour and tests are unchanged. A provider that can call
		// tools reads Messages instead.
		request := harness.GenerateRequest{Messages: messages, Prompt: contextPrompt}
		if offerTools {
			request.Tools = append(harness.ToolSpecs(), harness.TurnToolSpecs()...)
		}
		o.logger.Event("tool.round", map[string]interface{}{
			"round":               round,
			"offered":             offerTools,
			"conversation_tokens": conversationTokens(messages),
			"budget":              o.contextBudget(),
		})

		roundCtx, roundSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "provider.generate",
			oteltrace.WithAttributes(
				attribute.String("localrpg.role", "gm"),
				attribute.String("provider.id", provider.ID()),
				attribute.String("turn.assembled_prompt", contextPrompt),
				attribute.Int("localrpg.round", round),
				attribute.Bool("localrpg.tools_offered", offerTools),
			),
		)
		roundStarted := time.Now()
		result, err := o.generateRequest(roundCtx, request, onChunk)
		roundDuration := float64(time.Since(roundStarted).Milliseconds())
		roundAttributes := otelmetric.WithAttributes(
			attribute.String("localrpg.role", "gm"),
			attribute.Int("localrpg.round", round),
		)
		roundSpan.SetAttributes(attribute.String("turn.raw_completion", result.Text))
		if err != nil {
			code := harness.ClassifyProviderError(err)
			if result.Failure != nil && result.Failure.Code != "" {
				code = result.Failure.Code
			}
			engineMetrics().providerErrors.Add(context.Background(), 1, otelmetric.WithAttributes(
				attribute.String("localrpg.role", "gm"),
				attribute.String("error.kind", string(code)),
				attribute.String("gen_ai.system", provider.ID()),
			))
			engineMetrics().providerDuration.Record(context.Background(), roundDuration, roundAttributes)
			roundSpan.SetAttributes(
				attribute.String("turn.failure_code", string(code)),
				attribute.String("localrpg.generation.failure_code", string(code)),
			)
			roundSpan.RecordError(err)
			roundSpan.SetStatus(codes.Error, string(code))
			roundSpan.End()
			return result, err
		}
		engineMetrics().providerDuration.Record(context.Background(), roundDuration, roundAttributes)
		if result.Failure != nil {
			engineMetrics().providerErrors.Add(context.Background(), 1, otelmetric.WithAttributes(
				attribute.String("localrpg.role", "gm"),
				attribute.String("error.kind", string(result.Failure.Code)),
				attribute.String("gen_ai.system", provider.ID()),
			))
			roundSpan.SetAttributes(
				attribute.String("turn.failure_code", string(result.Failure.Code)),
				attribute.String("turn.failure_message", result.Failure.Message),
				attribute.String("localrpg.generation.failure_code", string(result.Failure.Code)),
			)
			roundSpan.RecordError(result.Failure)
			roundSpan.SetStatus(codes.Error, string(result.Failure.Code))
			roundSpan.End()
			return result, result.Failure
		}
		roundSpan.End()

		hasSubmitTurn := false
		for _, call := range result.ToolCalls {
			if call.Name == "submit_turn" {
				hasSubmitTurn = true
				break
			}
		}

		// A reply carrying calls while tools were not offered is a protocol quirk:
		// its text is the answer, and the calls are dropped and traced.
		// However, if the model called submit_turn, that is the terminal submission
		// of the turn and must be honored rather than discarded.
		if len(result.ToolCalls) > 0 && !offerTools && !hasSubmitTurn {
			o.logger.Event("tool.stray", map[string]interface{}{"round": round, "calls": len(result.ToolCalls)})
			result.ToolCalls = nil
			result.Provenance = provenance
			return result, nil
		}
		if len(result.ToolCalls) == 0 {
			result.Provenance = provenance
			return result, nil
		}

		// Prose in a tool round is the model thinking out loud, and its order
		// relative to the result is undefined, so it is discarded and traced.
		if strings.TrimSpace(result.Text) != "" {
			o.logger.Event("tool.prose_discarded", map[string]interface{}{
				"round": round,
				"chars": len([]rune(result.Text)),
			})
		}

		messages = append(messages, harness.Message{Role: "assistant", ToolCalls: result.ToolCalls})
		for _, call := range result.ToolCalls {
			trace.LogEvent(ctx, o.logger, "tool.call", map[string]interface{}{
				"round":           round,
				"name":            call.Name,
				"arguments":       call.Arguments,
				"arguments_chars": len([]rune(call.Arguments)),
			})
			o.notifyTool(ToolActivity{Round: round, Name: call.Name, Status: "running"})

			// Turn tools are handled here, not by the query executor: a check
			// resolves through the rules layer and feeds its result back, and a
			// submission ends the loop.
			if harness.IsTurnTool(call.Name) {
				switch call.Name {
				case "request_check":
					req, parseErr := harness.ParseCheckRequest(call.Arguments)
					if parseErr != nil {
						messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: "error: " + parseErr.Error()})
						continue
					}
					resolved, resolveErr := o.resolveCheck(ctx, *req, nil)
					if resolveErr != nil {
						messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: "error: " + resolveErr.Error()})
						continue
					}
					// The GM references this id in its segments, so it must be stable
					// and known to the model: the tool call id it chose serves that.
					if call.ID != "" {
						resolved.CheckID = call.ID
					}
					resolved.Actor = req.Actor
					resolved.Target = req.Target
					checks = append(checks, *resolved)
					encoded, _ := json.Marshal(resolved)
					messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: string(encoded)})
					o.notifyTool(ToolActivity{Round: round, Name: call.Name, Status: "done", Summary: resolved.Outcome})
					continue
				case "submit_turn":
					sub, parseErr := harness.ParseSubmission(call.Arguments)
					if parseErr != nil {
						messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: "error: " + parseErr.Error()})
						continue
					}
					if vErr := validateSubmission(sub, checks, o.declaredStats); vErr != nil {
						o.logger.Event("turn.protocol_error", map[string]interface{}{"detail": vErr.Error()})
						submitAttempts++
						if submitAttempts >= 2 {
							result.Submission = nil
							result.FallbackReason = vErr.Error()
							result.Provenance = provenance
							result.ToolCalls = nil
							return result, nil
						}
						messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: "protocol validation failed: " + vErr.Error()})
						continue
					}
					result.Submission = sub
					result.Checks = checks
					result.Provenance = provenance
					result.ToolCalls = nil
					return result, nil
				}
			}

			if !offerTools {
				o.logger.Event("tool.stray", map[string]interface{}{"round": round, "call": call.Name})
				continue
			}

			started := time.Now()
			toolCtx, toolSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "tool.call",
				oteltrace.WithAttributes(
					attribute.String("localrpg.tool.name", call.Name),
					attribute.Int("localrpg.round", round),
				),
			)
			output, ok := o.toolExecutor.Execute(toolCtx, call)
			toolSpan.SetAttributes(attribute.Bool("localrpg.tool.ok", ok), attribute.Int("localrpg.tool.bytes", len(output)))
			if !ok {
				toolSpan.SetStatus(codes.Error, "tool execution failed")
			}
			toolSpan.End()
			engineMetrics().toolDuration.Record(context.Background(), float64(time.Since(started).Milliseconds()),
				otelmetric.WithAttributes(
					attribute.String("localrpg.tool.name", call.Name),
					attribute.Bool("localrpg.tool.ok", ok),
				))
			trace.LogEvent(ctx, o.logger, "tool.result", map[string]interface{}{
				"name":        call.Name,
				"ok":          ok,
				"bytes":       len(output),
				"duration_ms": time.Since(started).Milliseconds(),
				"result":      output,
			})
			o.notifyTool(ToolActivity{Round: round, Name: call.Name, Status: "done", Summary: toolSummary(ok, output)})

			provenance = append(provenance, ToolCallRecord{Name: call.Name, ResultChars: len([]rune(output))})
			messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: output})
		}

		// After the final allowed round, or once the budget is crossed, tools are
		// withdrawn and the model is told so as a readable result rather than a
		// silent stop it would retry.
		if round+1 >= o.toolRoundCap() || o.overBudget(messages) {
			withdrawn = true
			messages = append(messages, harness.Message{
				Role:    "tool",
				Content: "Tools are no longer available for this turn. Complete and submit the turn now (call submit_turn or provide your narration as text).",
			})
		}
	}

	return streamResult{}, fmt.Errorf("tool loop ended without an answer")
}

// notifyTool reports activity if a client asked to see it.
func (o *TurnOrchestrator) notifyTool(activity ToolActivity) {
	if o.toolObserver != nil {
		o.toolObserver(activity)
	}
}

// toolSummary is the short human line a client renders.
func toolSummary(ok bool, output string) string {
	if !ok {
		return "failed"
	}
	if len(output) == 0 {
		return "no result"
	}
	return fmt.Sprintf("%d characters", len([]rune(output)))
}

// conversationTokens estimates the live conversation's size. Four runes per token
// is deliberately crude: the budget is a guardrail, not an accounting ledger.
func conversationTokens(messages []harness.Message) int {
	runes := 0
	for _, message := range messages {
		runes += len([]rune(message.Content))
	}
	return runes / 4
}

// contextBudget is the configured token ceiling, or 0 for unbounded.
func (o *TurnOrchestrator) contextBudget() int {
	return o.assembler.Limits().TokenBudget
}

// overBudget reports whether the live conversation has crossed the budget.
func (o *TurnOrchestrator) overBudget(messages []harness.Message) bool {
	budget := o.contextBudget()
	if budget <= 0 {
		return false
	}
	return conversationTokens(messages) > budget
}

func segmentKinds(segments []entity.TurnSegment) []string {
	kinds := make([]string, 0, len(segments))
	for _, segment := range segments {
		kinds = append(kinds, segment.Kind)
	}
	return kinds
}

func segmentSpeakers(segments []entity.TurnSegment) []string {
	speakers := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment.Speaker != "" {
			speakers = append(speakers, segment.Speaker)
		}
	}
	return speakers
}

func unresolvedSpeakers(segments []entity.TurnSegment) []string {
	unresolved := make([]string, 0)
	for _, segment := range segments {
		if segment.Kind == entity.SegmentSpeech && segment.SpeakerID == "" {
			unresolved = append(unresolved, segment.Speaker)
		}
	}
	return unresolved
}
