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
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/localrpg/pkg/turnstream"
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

type TurnOrchestrator struct {
	store               *storage.Store
	timeline            *Timeline
	rulesEngine         *rules.JSEngine
	router              *harness.Router
	startLocation       string
	playerID            string
	assembler           *harness.ContextAssembler
	rulesPrompt         string
	lorePrompt          string
	mechanicsPrompt     string
	mechanicsEngagement string
	mechanicsCadence    int
	// forceToolChoice is set for a turn whose cadence floor requires a check.
	forceToolChoice bool
	// pendingCheckRef continues a turn whose GM proposed a check (ask policy).
	pendingCheckRef string
	// forcedTotal and forcedDice carry a manually entered roll for the next check:
	// a resolved pending check the player rolled with physical dice. Consumed once
	// per turn.
	forcedTotal *int
	forcedDice  []int
	// singleTurn records an interactive roll as one turn: the proposing turn is a
	// draft, completed in place when the check resolves.
	singleTurn bool
	// imageTrigger is the resolved image policy: off, scene_break, significant,
	// every_turn, or manual. Empty means scene_break (today's behaviour).
	imageTrigger  string
	triggerConfig TriggerConfig
	extractor     *harness.Extractor
	chunkTimeout  time.Duration
	openingPrompt string
	// sceneOnly marks the next turn as a quiet scene turn: it restates the
	// campaign's opening scene and adds no hooks. Consumed once per turn.
	sceneOnly        bool
	logger           trace.Logger
	chronicler       *Chronicler
	threadsMax       int
	continuityChecks *bool
	// actionEcho asks the narrator to restate the player's action before resolving
	// it; on unless SetActionEcho turns it off.
	actionEcho       bool
	completion       harness.ModelProvider
	completionPolicy CompletionPolicy
	toolExecutor     ToolExecutor
	checkResolver    harness.CheckResolver
	declaredStats    map[string]core.StatSpec
	mechanics        *core.MechanicsSpec
	// health is the declared health schema, resolved to an effect when the stat
	// reaches zero.
	health *core.HealthSpec
	// worldTickTurns is how often onWorldTick runs, in turns. Zero disables it.
	worldTickTurns int
	// worldTick records the directive an onWorldTick run injected this turn.
	worldTick string
	// rebuildMechanicsPrompt makes the resolution instruction reflect the
	// player's current stats, rebuilt each turn rather than cached.
	rebuildMechanicsPrompt bool
	allowFreeform          bool
	toolCapability         string
	toolRounds             int
	toolObserver           func(ToolActivity)
	speechCues             harness.SpeechCueContext
	usageCtx               *harness.UsageContext
	// parser, roster, and segmentObserver carry the progressive turn stream for
	// the turn in flight. The orchestrator is built per turn, so they need no
	// synchronisation.
	parser          *turnstream.Parser
	roster          *roster
	segmentObserver func(turnstream.Event)
	sceneWorker     *SceneWorker
	portraitWorker  *PortraitWorker
	worldArtStyle   string
}

// sceneReference finds the most recent illustration of the same location, so a
// provider that can condition on a reference image keeps the scene's look. A
// location with no earlier illustration returns nil.
func (o *TurnOrchestrator) sceneReference(locationID string, pastTurns []Turn) []byte {
	if o.timeline == nil || strings.TrimSpace(locationID) == "" {
		return nil
	}
	scenesDir := filepath.Join(o.timeline.GameDir(), "assets", "scenes")
	for i := len(pastTurns) - 1; i >= 0; i-- {
		if pastTurns[i].Location != locationID || pastTurns[i].Number <= 0 {
			continue
		}
		base := filepath.Join(scenesDir, fmt.Sprintf("turn-%d", pastTurns[i].Number))
		for _, ext := range []string{".png", ".webp", ".jpg", ".jpeg", ".svg"} {
			if data, err := os.ReadFile(base + ext); err == nil && len(data) > 0 {
				return data
			}
		}
	}
	return nil
}

// SetSceneWorker attaches a scene illustration worker to the orchestrator.
func (o *TurnOrchestrator) SetSceneWorker(w *SceneWorker) {
	o.sceneWorker = w
}

// SetPortraitWorker attaches a portrait generation worker to the orchestrator.
func (o *TurnOrchestrator) SetPortraitWorker(w *PortraitWorker) {
	o.portraitWorker = w
}

// SetWorldArtStyle sets the art style for scene and portrait generation.
func (o *TurnOrchestrator) SetWorldArtStyle(style string) {
	o.worldArtStyle = style
}

// SetSegmentObserver attaches a sink for parsed turn-stream events, so a client
// can render attributed segments while the model is still writing. A nil
// observer records nothing.
func (o *TurnOrchestrator) SetSegmentObserver(observer func(turnstream.Event)) {
	o.segmentObserver = observer
}

// Voice returns the voice assigned to a speaker entity, checking the live roster
// before falling back to store notes.
func (o *TurnOrchestrator) Voice(speakerID string) *entity.VoiceConfig {
	if o == nil || o.roster == nil {
		return nil
	}
	return o.roster.Voice(speakerID)
}

// observeSegments forwards parsed events, if a sink is attached.
func (o *TurnOrchestrator) observeSegments(events []turnstream.Event) {
	if o.segmentObserver == nil {
		return
	}
	for _, event := range events {
		o.segmentObserver(event)
	}
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
		o.checkResolver = nil
		return
	}
	o.checkResolver = resolver
}

// SetDeclaredStats sets the mechanics schema's declared stats, used to validate
// state changes. Nil means the system declares no stats.
func (o *TurnOrchestrator) SetDeclaredStats(stats map[string]core.StatSpec) {
	o.declaredStats = stats
}

// SetMechanics hands the orchestrator the system's full mechanics schema, so it
// can earn advancement and check thresholds after a turn. Nil disables it.
func (o *TurnOrchestrator) SetMechanics(spec *core.MechanicsSpec) {
	o.mechanics = spec
}

// SetAllowFreeformState permits state changes to undeclared paths even when the
// system declares stats.
func (o *TurnOrchestrator) SetAllowFreeformState(allow bool) {
	o.allowFreeform = allow
}

// SetRulesPrompt sets the system's rules text without re-reading it, so a caller
// that already cached it does not pay for the file again.
func (o *TurnOrchestrator) SetRulesPrompt(prompt string) { o.rulesPrompt = prompt }

// SetLorePrompt sets the world's lore text without re-reading it.
func (o *TurnOrchestrator) SetLorePrompt(prompt string) { o.lorePrompt = prompt }

// SetMechanicsPrompt sets the formatted mechanics instruction without rebuilding
// it, so a cached runtime can hand it over directly.
func (o *TurnOrchestrator) SetMechanicsPrompt(prompt string) { o.mechanicsPrompt = prompt }

// SetMechanicsSchema hands the orchestrator the spec and engagement so the
// resolution instruction is rebuilt each turn with the player's current stats.
func (o *TurnOrchestrator) SetMechanicsSchema(spec *core.MechanicsSpec, engagement string) {
	o.mechanics = spec
	if engagement != "" {
		o.mechanicsEngagement = engagement
	}
	o.rebuildMechanicsPrompt = true
	o.mechanicsPrompt = o.mechanicsInstruction()
}

// SetHealthSpec sets the declared health schema, resolved to an effect when the
// player's health stat reaches zero.
func (o *TurnOrchestrator) SetHealthSpec(health *core.HealthSpec) {
	o.health = health
}

// SetWorldTickTurns sets how often onWorldTick runs, in turns. Zero disables it.
func (o *TurnOrchestrator) SetWorldTickTurns(turns int) {
	if turns < 0 {
		turns = 0
	}
	o.worldTickTurns = turns
}

// SetUsageContext attaches the per-turn usage sink, so provider calls made
// during the turn are stamped with its number.
func (o *TurnOrchestrator) SetUsageContext(ctx *harness.UsageContext) { o.usageCtx = ctx }

// checkResolverOrDefault returns the configured resolver.
func (o *TurnOrchestrator) resolveCheck(ctx context.Context, req harness.CheckRequest, actor *entity.Entity) (*harness.CheckResult, error) {
	resolver := o.checkResolver
	if resolver == nil {
		resolver = defaultCheckResolver{mechanics: o.mechanics, store: o.store}
	}
	// A forced total applies to the first check of the turn (the pending one),
	// then is consumed so it cannot colour a later roll.
	if o.forcedTotal != nil {
		req.ForcedTotal = o.forcedTotal
		o.forcedTotal = nil
	}
	if len(o.forcedDice) > 0 {
		req.ForcedDice = o.forcedDice
		o.forcedDice = nil
	}
	resolved, err := resolver.Resolve(ctx, req, actor)
	if err != nil {
		return nil, err
	}
	if resolved.CheckKind == "" {
		resolved.CheckKind = req.CheckKind
	}
	if resolved.Stakes == "" {
		resolved.Stakes = req.Stakes
	}
	if resolved.OutcomeText == "" {
		if text, ok := req.Outcomes[resolved.Outcome]; ok {
			resolved.OutcomeText = text
		}
	}
	if len(resolved.OutcomeVocabulary) == 0 && o.mechanics != nil {
		resolved.OutcomeVocabulary = o.mechanics.Checks.Outcome
	}
	return resolved, nil
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
		// The action echo is on by default, matching the config default; a caller
		// that read the config overrides it with SetActionEcho.
		actionEcho: true,
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

// SetSceneOnly marks the next turn as a quiet scene turn. It is meaningful only
// for an Opening turn, and the engine consumes it once.
func (o *TurnOrchestrator) SetSceneOnly(sceneOnly bool) {
	o.sceneOnly = sceneOnly
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

// SetActionEcho asks the narrator to open each action turn with a third-person
// restatement of the player's action.
func (o *TurnOrchestrator) SetActionEcho(enabled bool) {
	o.actionEcho = enabled
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

// SetMechanicsEngagement selects the policy the mechanics instruction reflects.
func (o *TurnOrchestrator) SetMechanicsEngagement(engagement string) {
	o.mechanicsEngagement = engagement
}

// SetMechanicsCadence sets how many quiet turns force a check; 0 disables.
func (o *TurnOrchestrator) SetMechanicsCadence(turns int) {
	o.mechanicsCadence = turns
}

// SetPendingCheckRef continues the turn whose pending check has this ref: the
// engine resolves it and the GM adjudicates the result.
func (o *TurnOrchestrator) SetPendingCheckRef(ref string) {
	o.pendingCheckRef = ref
}

// SetForcedTotal makes the next check resolve to this entered dice total instead
// of rolling, so a manually entered die result is honoured. It is consumed once
// per turn.
func (o *TurnOrchestrator) SetForcedTotal(total *int) {
	o.forcedTotal = total
}

// SetManualDice makes the next check resolve from the dice a player entered
// rather than from a roll. It is consumed once per turn.
func (o *TurnOrchestrator) SetManualDice(dice []int) {
	o.forcedDice = dice
}

// SetSingleTurnMode records an interactive roll as one turn: a turn that ends on
// a pending check is written as a draft, and resolving the check completes it in
// place rather than appending a continuation turn.
func (o *TurnOrchestrator) SetSingleTurnMode(single bool) {
	o.singleTurn = single
}

// SetImageTrigger sets the policy deciding when a turn image is generated. Empty
// keeps today's scene-break behaviour.
func (o *TurnOrchestrator) SetImageTrigger(policy string) {
	o.imageTrigger = policy
	if o.triggerConfig.NarrationThreshold == 0 {
		o.triggerConfig = DefaultTriggerConfig()
	}
}

// playerRollRequest builds the check a player-initiated roll asks for, from the
// system's declared conventions: the actor is the player, the notation comes from
// the conventions, and the stakes are the player's own words.
func (o *TurnOrchestrator) playerRollRequest(input string) harness.CheckRequest {
	req := harness.CheckRequest{
		Actor:     o.playerID,
		CheckKind: "do",
		Stakes:    strings.TrimSpace(input),
	}
	if o.mechanics != nil {
		req.Notation = o.mechanics.Checks.Notation
	}
	if req.Notation == "" {
		req.Notation = "2d6"
	}
	return req
}

func (o *TurnOrchestrator) LoadPrompts(paths *core.PathResolver, systemID, worldID string) {
	if paths != nil {
		if systemID != "" {
			sysDir := paths.SystemDir(systemID)
			if data, err := os.ReadFile(filepath.Join(sysDir, "prompts", "rules.md")); err == nil {
				o.rulesPrompt = string(data)
			}
			// A system ships mechanics when it has a script or a declarative
			// block. Either way the GM needs to know when to call a check.
			var spec *core.MechanicsSpec
			if manifest, err := core.LoadSystemManifest(filepath.Join(sysDir, "system.yaml")); err == nil {
				spec = manifest.Mechanics
			}
			o.mechanics = spec
			_, scriptErr := os.Stat(filepath.Join(sysDir, "mechanics.js"))
			if scriptErr == nil || spec != nil {
				engagement := o.mechanicsEngagement
				if engagement == "" {
					engagement = "auto"
				}
				o.SetMechanicsSchema(spec, engagement)
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
	sceneOnly := o.sceneOnly
	o.sceneOnly = false
	if o.usageCtx != nil {
		o.usageCtx.SetTurn(turnNum)
	}

	// The engagement cadence floor: after enough quiet turns, force a check.
	o.forceToolChoice = false
	if shouldForceCheck(o.mechanicsEngagement, o.mechanicsCadence, pastTurns) {
		o.forceToolChoice = true
		o.logger.Event("mechanics.cadence", map[string]interface{}{"quiet_turns": quietTurns(pastTurns)})
	}

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
	var pendingCheck *harness.PendingCheck
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
		// The scene is the turn's frame, not the player's action: handing it over
		// as the action made the GM respond to it instead of restating it.
		generationPrompt = ""
	}

	// Handle /gm director note or mode
	isCorrection := !isOpening && (mode == "GM" || strings.HasPrefix(actionInput, "/gm "))
	if isCorrection {
		directiveText := strings.TrimPrefix(actionInput, "/gm ")
		gmDirective = fmt.Sprintf("[DIRECTOR CORRECTION DIRECTIVE: %s]", directiveText)
	} else if !isOpening && strings.EqualFold(mode, "Roll") && o.mechanicsEngagement != "off" && o.pendingCheckRef == "" {
		// A player-initiated roll is a check the player resolves: the turn ends on
		// a pending check, so the roll card can present the stakes. A Roll that
		// carries a pending ref is resolving an existing check, not asking for a
		// new one, so it falls through to the resolution path.
		req := o.playerRollRequest(actionInput)
		pendingCheck = &harness.PendingCheck{Ref: rollRef(turnNum, 0), Request: req, ProposedBy: "player"}
		gmDirective = fmt.Sprintf("[PLAYER ROLL REQUESTED: %s]", strings.TrimSpace(actionInput))
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

	// The living world advances on its own cadence, before the prompt is built,
	// so a tick's injected directive reaches the turn that triggered it.
	o.runWorldTick(turnNum, locationID)

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

	echoAction := o.actionEcho &&
		!isOpening && !isCorrection &&
		!strings.EqualFold(mode, "Roll") &&
		!strings.EqualFold(mode, "Say") &&
		strings.TrimSpace(actionInput) != ""

	// The opening turn restates the campaign's scene. A quiet scene turn suppresses
	// the hooks only when there is a scene to restate; without one the turn still
	// establishes a scene from the world.
	openingScene := ""
	openingHooks := false
	if isOpening {
		openingScene = o.openingPrompt
		openingHooks = !sceneOnly || strings.TrimSpace(openingScene) == ""
	}

	assembly, err := o.assembler.Assemble(harness.ContextRequest{
		Context:          ctx,
		LocationID:       locationID,
		PlayerID:         o.playerID,
		Action:           generationPrompt,
		OpeningScene:     openingScene,
		OpeningHooks:     openingHooks,
		PlayerName:       o.playerDisplayName(),
		ActionEcho:       echoAction,
		RulesPrompt:      o.rulesPrompt,
		LorePrompt:       o.lorePrompt,
		MechanicsPrompt:  o.mechanicsInstruction(),
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

	// Directives hooks injected (onAction, onWorldTick, onTurnBegin) reach the
	// prompt here. Without draining them, injectGMDirection had no effect.
	if directives := o.drainDirectives(); len(directives) > 0 {
		gmDirective = applyDirectives(gmDirective, directives)
		if o.worldTickDue(turnNum) {
			o.worldTick = strings.Join(directives, "\n")
		}
	}

	turnSpan.SetAttributes(attribute.String("turn.assembled_prompt", assembly.Prompt))
	// A pending GM-proposed check is resolved here, before generation, so the
	// model narrates its outcome rather than proposing it again.
	var resolvedPending *harness.CheckResult
	validationEngagement := o.mechanicsEngagement
	resolvedRef := ""
	continuationOf := 0
	if o.pendingCheckRef != "" {
		resolvedRef = o.pendingCheckRef
		continuationOf = findPendingTurn(pastTurns, o.pendingCheckRef)
		if pending := findPendingCheck(pastTurns, o.pendingCheckRef); pending != nil {
			// A roll a previous attempt already resolved is reused, so a retried
			// request cannot change the outcome the player already saw.
			resolved := findResolvedCheck(pastTurns, o.pendingCheckRef)
			if resolved == nil {
				if fresh, resolveErr := o.resolveCheck(ctx, pending.Request, nil); resolveErr == nil {
					fresh.CheckID = pending.Ref
					resolved = fresh
				} else {
					o.logger.Event("pending.resolve_error", map[string]interface{}{"error": resolveErr.Error()})
				}
			}
			if resolved != nil {
				resolvedPending = resolved
				validationEngagement = "auto"
				directive := fmt.Sprintf("[PLAYER ROLL: %s — %s]", resolved.Outcome, strings.TrimSpace(pending.Request.Stakes))
				gmDirective = strings.TrimSpace(directive + "\n" + gmDirective)
			}
		}
		o.pendingCheckRef = ""
	}

	if onChunk != nil {
		started := time.Now()
		first := true
		inner := onChunk
		onChunk = func(text string) error {
			if first {
				first = false
				elapsed := time.Since(started).Milliseconds()
				engineMetrics().turnTTFT.Record(ctx, float64(elapsed))
				o.logger.Event("turn.ttft", map[string]interface{}{"ms": elapsed})
			}
			return inner(text)
		}
	}

	// The reply is parsed as it streams, so segments and their speakers are
	// attributed while the model is still writing. The roster is seeded from the
	// store, and a persona record extends it mid-stream. The parser wraps the
	// TTFT listener, so the client still sees each raw chunk first.
	o.roster = newRoster(o.store, o.playerID, o.playerDisplayName(), o.timeline.VoiceProfiles())
	o.parser = turnstream.NewParser(o.roster)
	if onChunk != nil {
		inner := onChunk
		onChunk = func(text string) error {
			if err := inner(text); err != nil {
				return err
			}
			o.observeSegments(o.parser.Feed(text))
			return nil
		}
	} else {
		onChunk = func(text string) error {
			o.observeSegments(o.parser.Feed(text))
			return nil
		}
	}

	// A player's spoken line leads the turn: observe it immediately so the
	// client receives its segment and the streamer voices it in the player's
	// voice before the narrator begins.
	if beat := playerSegment(mode, actionInput, o.playerID, o.playerDisplayName()); beat != nil {
		o.observeSegments([]turnstream.Event{{
			Kind:      turnstream.KindSpeech,
			Speaker:   beat.Speaker,
			SpeakerID: beat.SpeakerID,
			Text:      beat.Text,
			Player:    true,
		}})
	}

	cause := cutNone
	var narration string
	var recovery RecoveryOutcome
	var stillIncomplete bool

	// A reply may end on a @roll record, in which case the engine resolves it and
	// asks the model to continue from the outcome. The loop is bounded so a model
	// that keeps rolling cannot run forever.
	var result streamResult
	var collected []turnstream.Event
	var rollResults []harness.CheckResult
	var narrationParts []string
	const maxRollContinuations = 3
	endedOnRoll := false
	// rollAnchors record where each resolved roll's check belongs in the turn's
	// script: at the first segment the continuation produced, so the dice render
	// where the roll happened rather than leading the turn.
	type rollAnchor struct {
		segmentIndex int
		checkID      string
	}
	var rollAnchors []rollAnchor
	// collectedCount is how many of the parser's events are already in collected,
	// so a recovery continuation's events can be appended without duplicating the
	// ones the loop already took.
	collectedCount := 0
	drainEvents := func() {
		if o.parser == nil {
			return
		}
		o.parser.Flush()
		events := o.parser.Events()
		if collectedCount < len(events) {
			collected = append(collected, events[collectedCount:]...)
			collectedCount = len(events)
		}
	}
	for attempt := 0; ; attempt++ {
		result, err = o.runGenerationLoop(ctx, &assembly, gmDirective, resolvedPending, validationEngagement, onChunk)
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

		if strings.TrimSpace(result.Text) != "" {
			narrationParts = append(narrationParts, result.Text)
		}
		drainEvents()

		// A @roll record ends the call. Under the ask policy it becomes a pending
		// check the player rolls; under auto the engine rolls at once and the model
		// continues from the outcome.
		req, hasRoll := o.pendingRoll()
		endedOnRoll = hasRoll && result.PendingCheck == nil
		if !endedOnRoll || o.mechanicsEngagement == "off" {
			break
		}
		if o.mechanicsEngagement == "ask" {
			result.PendingCheck = &harness.PendingCheck{Ref: rollRef(turnNum, 0), Request: req, ProposedBy: "gm"}
			break
		}
		if attempt >= maxRollContinuations {
			o.logger.Event("roll.cap", map[string]interface{}{"continuations": attempt})
			if resolved, resolveErr := o.resolveCheck(ctx, req, nil); resolveErr == nil {
				resolved.CheckID = rollRef(turnNum, len(rollResults))
				rollResults = append(rollResults, *resolved)
				result.RollOutcome = strings.TrimSpace(req.Outcomes[resolved.Outcome])
				result.RollCheckRef = resolved.CheckID
			}
			break
		}
		resolved, resolveErr := o.resolveCheck(ctx, req, nil)
		if resolveErr != nil {
			o.logger.Event("roll.resolve_error", map[string]interface{}{"error": resolveErr.Error()})
			break
		}
		resolved.CheckID = rollRef(turnNum, len(rollResults))
		rollResults = append(rollResults, *resolved)
		rollAnchors = append(rollAnchors, rollAnchor{
			segmentIndex: len(segmentsFromEvents(collected)),
			checkID:      resolved.CheckID,
		})
		gmDirective = rollContinuationDirective(*resolved, req)
		resolvedPending = nil
		if o.parser != nil {
			o.parser.Reset()
			collectedCount = 0
		}
	}
	// A player-initiated roll ends the turn on the pending check it asked for,
	// unless the model already proposed one of its own.
	if pendingCheck != nil && result.PendingCheck == nil {
		result.PendingCheck = pendingCheck
	}
	result.Checks = append(result.Checks, rollResults...)

	_, finaliseSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "turn.finalise")
	defer finaliseSpan.End()

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

	// A pending check is not an empty turn: the prose the model wrote before the
	// check is the setup the player reads while deciding, so recovery runs for it
	// too and only a genuinely empty reply is an error.
	{
		// The final call may be cut off; earlier continuations ended on a roll and
		// are complete by construction, so only the last needs classification.
		if endedOnRoll {
			cause = cutNone
		} else {
			cause = o.classifyCut(result)
		}
		narration, recovery, stillIncomplete = o.recoverReply(ctx, strings.Join(narrationParts, "\n\n"), cause, onChunk)
		// The recovery pass streams its continuation through onChunk, so its
		// segments are in the parser now and must join the turn's script.
		drainEvents()
		if strings.TrimSpace(narration) == "" && result.PendingCheck == nil {
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

	if outcome := strings.TrimSpace(result.RollOutcome); outcome != "" {
		if strings.TrimSpace(narration) == "" {
			narration = outcome
		} else {
			narration = strings.TrimSpace(narration) + "\n\n" + outcome
		}
	}

	o.logger.Event("generation.complete", map[string]interface{}{
		"narration_chars": len([]rune(narration)),
		"finish_reason":   result.FinishReason,
		"cause":           cause.String(),
		"recovery":        string(recovery),
		"truncated":       stillIncomplete,
	})

	// Control records are protocol, not story: strip them from the prose that is
	// recorded, extracted, and shown. The parser keeps them for the declarations
	// applied below.
	narration = stripRecordLines(narration)

	turn := Turn{
		Number:           turnNum,
		Timestamp:        time.Now(),
		Mode:             mode,
		Input:            actionInput,
		Roll:             rollRes,
		Narration:        narration,
		Location:         locationID,
		Outcome:          outcome,
		Truncated:        stillIncomplete,
		Recovery:         string(recovery),
		ContextNotes:     assembly.Trimmed,
		Context:          &assembly.Context,
		Prompt:           contextPrompt,
		ToolCalls:        result.Provenance,
		PendingCheck:     result.PendingCheck,
		Draft:            o.singleTurn && result.PendingCheck != nil,
		ResolvesCheckRef: resolvedRef,
		ContinuationOf:   continuationOf,
	}

	extraction := harness.Extraction{}
	var extractionErr error
	extractionDone := make(chan struct{})
	// Extraction is a model call, so start it before the local mention and
	// segment work and await it just before segments are built. A failed
	// extractor must not lose the turn.
	if o.extractor != nil {
		go func() {
			defer close(extractionDone)
			extractCtx, extractSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "extract.entities")
			defer extractSpan.End()
			if extracted, err := o.extractor.Extract(extractCtx, turn.Narration); err == nil {
				extraction = *extracted
			} else {
				extractionErr = err
				extractSpan.RecordError(err)
				extractSpan.SetStatus(codes.Error, string(harness.ClassifyProviderError(err)))
				o.logger.Event("extract.error", map[string]interface{}{"error": err.Error()})
			}
			extractSpan.SetAttributes(attribute.Int("localrpg.entities.extracted", len(extraction.Entities)))
		}()
	} else {
		close(extractionDone)
	}

	turn.Entities = harness.ResolveEntityMentions(o.store, o.playerID, locationID, turn.Narration, actionInput)

	var personae []harness.PersonaDecl
	var memories []harness.MemoryDecl
	var stateChanges []harness.StateChangeDecl
	moveRef := ""
	if o.parser != nil {
		// The progressive stream carries its declarations inline: a persona
		// before the line that speaks, a state change after the roll it follows.
		personae, memories, stateChanges, moveRef = o.applyRecords()
		o.logRepairReport()
	}
	for _, persona := range personae {
		if id := entity.Slugify(persona.Name); id != "" {
			turn.Personae = append(turn.Personae, id)
		}
	}
	turn.Checks = result.Checks
	turn.RecordReport = o.recordReport()

	<-extractionDone
	if extractionErr != nil {
		o.logger.Event("extract.error", map[string]interface{}{"error": extractionErr.Error()})
	}

	// The streamed reply was parsed as it arrived, across every continuation; its
	// events are the turn's playback script. A reply with no framing at all falls
	// back to the legacy prose parser plus the extractor. Any dialogue that slipped
	// through stream parsing as narration is rescued using extractor attributions.
	events := collected
	if parsed := segmentsFromEvents(events); len(parsed) > 0 {
		turn.Segments = parsed
		if len(extraction.Dialogue) > 0 {
			resolve := func(candidate string) (string, bool) {
				if o.roster != nil {
					if id, ok := o.roster.Resolve(candidate); ok {
						return id, true
					}
				}
				if id := harness.ResolveSpeakerID(o.store, candidate); id != "" {
					return id, true
				}
				return proposedSpeakerID(extraction.Entities, candidate)
			}
			turn.Segments = mergeAttributions(turn.Segments, extraction.Dialogue, resolve)
		}
	} else {
		turn.Segments = buildTurnSegments(o.store, turn.Narration, extraction)
	}

	// Remap earlier segments if a declared persona revealed an earlier identity.
	for _, persona := range personae {
		if id := entity.Slugify(persona.Name); id != "" {
			if prev := strings.TrimSpace(persona.PreviousIdentity()); prev != "" {
				prevSlug := entity.Slugify(prev)
				for i := range turn.Segments {
					if turn.Segments[i].SpeakerID == prevSlug || turn.Segments[i].Speaker == prev {
						turn.Segments[i].SpeakerID = id
					}
				}
			}
		}
	}

	// A resolved roll's outcome is narrated after the prose that led to it, so the
	// chronicle can render the dice inline with the consequence.
	if outcome := strings.TrimSpace(result.RollOutcome); outcome != "" {
		turn.Segments = append(turn.Segments, entity.TurnSegment{
			Kind:     entity.SegmentNarration,
			Text:     outcome,
			CheckRef: result.RollCheckRef,
		})
	}

	// The player's own spoken line leads the turn, so it is heard in their voice
	// before the narrator answers. If the narrator's generated text already begins
	// with the player's line, attachPlayerSegment marks it rather than duplicating it.
	modelSegments := len(turn.Segments)
	turn.Segments = attachPlayerSegment(turn.Segments, mode, actionInput, o.playerID, o.playerDisplayName())
	// A player segment is prepended, so every model segment shifts by one. Point
	// each resolved roll's check at the segment its continuation produced, so the
	// chronicle renders the dice where the roll happened.
	shift := len(turn.Segments) - modelSegments
	for _, anchor := range rollAnchors {
		index := anchor.segmentIndex + shift
		if index < 0 || index >= len(turn.Segments) {
			continue
		}
		if turn.Segments[index].CheckRef == "" {
			turn.Segments[index].CheckRef = anchor.checkID
		}
	}

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

	// Detect scene breaks via explicit markdown horizontal rules or extractor cues.
	if hasProseRuleBreak(turn.Narration) || (extraction.SceneBreak != nil && extraction.SceneBreak.Occurred) {
		turn.SceneBreak = true
		if extraction.SceneBreak != nil && extraction.SceneBreak.VisualCue != "" {
			turn.SceneBreakCue = extraction.SceneBreak.VisualCue
		}
	}

	// Anchor speaker portraits with the character's active version at turn time.
	for i := range turn.Segments {
		if turn.Segments[i].Kind == entity.SegmentSpeech {
			speakerID := turn.Segments[i].SpeakerID
			if speakerID == "" && turn.Segments[i].Speaker != "" {
				speakerID = harness.ResolveSpeakerID(o.store, turn.Segments[i].Speaker)
			}
			if speakerID == "" && turn.Segments[i].Speaker != "" {
				speakerID = entity.Slugify(turn.Segments[i].Speaker)
			}
			if speakerID != "" && o.store != nil {
				if ent, err := o.store.GetEntity(speakerID); err == nil && ent != nil {
					version := ent.PortraitVersion
					if version <= 0 && ent.Portrait != "" {
						version = 1
					}
					if version > 0 {
						turn.Segments[i].SpeakerPortrait = fmt.Sprintf("/api/game/%s/character/%s/portrait?v=%d", o.gameID(), speakerID, version)
					} else {
						turn.Segments[i].SpeakerPortrait = fmt.Sprintf("/api/game/%s/character/%s/portrait", o.gameID(), speakerID)
					}
				}
			}
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
	// started, which is what keeps scenes honest. An explicit move record wins over
	// an extracted one.
	move := strings.TrimSpace(moveRef)
	if move == "" {
		move = strings.TrimSpace(extraction.PlayerLocation)
	}
	if move != "" {
		if ent := findLocationByRef(o.store, move); ent != nil && ent.ID != locationID {
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
	if len(stateChanges) > 0 && o.rulesEngine != nil {
		notes, err := rules.ApplyStateChanges(o.rulesEngine.HostAPI(), stateChanges, o.declaredStats, o.allowFreeform)
		if err != nil {
			return nil, fmt.Errorf("apply state changes: %w", err)
		}
		// A change the engine could not apply is recorded rather than fatal: the GM
		// has already narrated its consequence, and a reference the index does not
		// hold must not cost the player the turn they just played.
		for _, note := range notes {
			trace.OrNil(o.logger).Event("mechanics.state_change_skipped", map[string]interface{}{"note": note})
		}
	}

	// Earn advancement after the turn's own state changes, so the award is part
	// of the same record and a threshold level sees the final values.
	o.applyAdvancement(ctx, &turn)

	turn.WorldTick = o.worldTick

	// Health is resolved twice around the post-turn hooks: once before them, so
	// the hook context carries the effects the turn's own changes caused, and once
	// after, so a hook that drives a stat to zero is seen on this same turn rather
	// than the next.
	pendingEffects := o.healthOutcomes(&turn)
	o.runTurnEndHooks(turnNum, &turn, pendingEffects)
	finalEffects := o.healthOutcomes(&turn)
	turn.HealthEffects = append(turn.HealthEffects, mergeHealthEffects(pendingEffects, finalEffects)...)

	// In single-turn mode a resolved check completes the draft turn in place
	// rather than appending a continuation, so the fiction stays one record.
	if o.singleTurn && continuationOf != 0 {
		turn.Number = continuationOf
		if err := o.timeline.ReplaceTurnContextStructured(ctx, &turn, extraction.Entities, personae, memories, result.Checks); err != nil {
			return nil, fmt.Errorf("replace turn: %w", err)
		}
	} else if err := o.timeline.RecordTurnContextStructured(ctx, &turn, extraction.Entities, personae, memories, result.Checks); err != nil {
		return nil, fmt.Errorf("record turn: %w", err)
	}

	if exec, ok := o.toolExecutor.(interface {
		AssignedVoices() map[string]config.VoiceProfile
	}); ok {
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

	if o.sceneWorker != nil && o.shouldIllustrate(turn, pastTurns) {
		var locEntity *entity.Entity
		if turn.Location != "" && o.store != nil {
			locEntity, _ = o.store.GetEntity(turn.Location)
		}
		cue := turn.SceneBreakCue
		if cue == "" {
			cue = ExtractSceneCue(turn.Narration)
		}
		sceneCtx := ScenePromptContext{
			Cue:       cue,
			Narration: turn.Narration,
			Action:    turn.Input,
			Location:  turn.Location,
			Style:     o.worldArtStyle,
			Entities:  o.presentEntityNames(&turn),
		}
		if locEntity != nil {
			sceneCtx.Location = locEntity.Name
			sceneCtx.Appearance = locEntity.Appearance
		}
		if len(turn.Checks) > 0 {
			sceneCtx.Outcome = turn.Checks[0].Outcome
		}
		// The scene's stable look keeps successive images of one place together;
		// the reference image is the previous illustration of the same location.
		sceneCtx.Scene = media.NewSceneStyle(turn.Location, o.worldArtStyle, sceneCtx.Appearance)
		scenePrompt := BuildScenePrompt(sceneCtx)
		o.sceneWorker.EnqueueScene(o.gameID(), turn.Number, SceneJob{
			Prompt:    scenePrompt,
			Reference: o.sceneReference(turn.Location, pastTurns),
		})
	}

	if o.portraitWorker != nil {
		for _, raw := range extraction.Entities {
			if raw.AppearanceChanged && o.store != nil {
				matched := harness.MatchExistingEntity(o.store, &raw)
				if matched != nil && entity.IsCharacterType(matched.Type) {
					o.portraitWorker.EnqueueVersion(o.gameID(), matched, o.worldArtStyle, true)
				}
			}
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
	// Checks are the checks the turn resolved.
	Checks []harness.CheckResult
	// PendingCheck is set when the model proposed a check under the ask policy and
	// the turn ends awaiting the player's roll.
	PendingCheck *harness.PendingCheck
	// RollOutcome is the narration text a resolved @roll record produced, and
	// RollCheckRef names the check it belongs to, so the segment can render the
	// dice inline.
	RollOutcome  string
	RollCheckRef string
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
func (o *TurnOrchestrator) runGenerationLoop(ctx context.Context, assembly *harness.AssembleResult, gmDirective string, resolvedPending *harness.CheckResult, engagement string, onChunk func(string) error) (streamResult, error) {
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
	var recordedAttempts []harness.Attempt
	if resolvedPending != nil {
		checks = append(checks, *resolvedPending)
	}
	withdrawn := false

	for round := 0; round <= o.toolRoundCap(); round++ {
		offerTools := canCallTools && round < o.toolRoundCap() && !withdrawn && !o.overBudget(messages)

		// Prompt is kept for a caller or provider that only reads a string: it is
		// the assembled context, exactly as the single-prompt path sent it, so
		// existing behaviour and tests are unchanged. A provider that can call
		// tools reads Messages instead.
		request := harness.GenerateRequest{Messages: messages, Prompt: contextPrompt}
		if offerTools {
			mode := engagement
			if mode == "" {
				mode = "auto"
			}
			request.Tools = append(harness.ToolSpecs(), harness.TurnToolSpecsFor(mode)...)
		}
		if round == 0 && o.forceToolChoice && resolvedPending == nil {
			// Force a tool call when the provider can, otherwise nudge the prompt.
			if offerTools {
				request.ToolChoice = "required"
			} else {
				contextPrompt = forceCheckNudge + "\n\n" + contextPrompt
				request.Prompt = contextPrompt
				if len(messages) > 0 {
					last := &messages[len(messages)-1]
					last.Content = strings.TrimSpace(last.Content + "\n\n" + forceCheckNudge)
				}
			}
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
		var result streamResult
		var genErr error
		repairAttempts := 0
		maxRepairs := o.completionMaxRepairAttempts()
		genReq := request

		for {
			roundStarted := time.Now()
			// Each round's prose is provisional until the round is chosen: only the
			// last round becomes the turn. Resetting the parser discards a round that
			// narrated before calling a tool, so the finalised segments match the
			// recorded narration.
			if o.parser != nil {
				o.parser.Reset()
			}
			result, genErr = o.generateRequest(roundCtx, genReq, onChunk)
			roundDuration := float64(time.Since(roundStarted).Milliseconds())
			roundAttributes := otelmetric.WithAttributes(
				attribute.String("localrpg.role", "gm"),
				attribute.Int("localrpg.round", round),
			)
			roundSpan.SetAttributes(attribute.String("turn.raw_completion", result.Text))

			// Check if reply is malformed (empty stream or malformed reply)
			isMalformed := (result.Failure != nil && result.Failure.Code == harness.FailureEmptyResponse) ||
				(genErr == nil && classifyReply(result, turnstream.RepairReport{}) == ProblemMalformed)

			if isMalformed && repairAttempts < maxRepairs {
				detail := "empty response"
				if result.Failure != nil && result.Failure.Message != "" {
					detail = result.Failure.Message
				}
				recordedAttempts = append(recordedAttempts, harness.Attempt{
					Role:     "gm",
					Provider: provider.ID(),
					Code:     harness.FailureParseError,
					Detail:   detail,
				})
				repairAttempts++
				nudge := repairInstruction(ProblemMalformed, detail)
				messages = append(messages, harness.Message{Role: "user", Content: nudge})
				genReq.Messages = messages
				if strings.TrimSpace(genReq.Prompt) != "" {
					genReq.Prompt = genReq.Prompt + "\n\n" + nudge
				}
				continue
			}

			if genErr != nil {
				code := harness.ClassifyProviderError(genErr)
				if result.Failure != nil && result.Failure.Code != "" {
					code = result.Failure.Code
				}
				if len(recordedAttempts) > 0 && result.Failure != nil {
					result.Failure.Attempts = append(recordedAttempts, result.Failure.Attempts...)
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
				roundSpan.RecordError(genErr)
				roundSpan.SetStatus(codes.Error, string(code))
				roundSpan.End()
				return result, genErr
			}
			engineMetrics().providerDuration.Record(context.Background(), roundDuration, roundAttributes)
			if result.Failure != nil {
				if len(recordedAttempts) > 0 {
					result.Failure.Attempts = append(recordedAttempts, result.Failure.Attempts...)
				}
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
			break
		}
		roundSpan.End()

		// A reply carrying calls while tools were not offered is a protocol quirk:
		// its text is the answer, and the calls are dropped and traced.
		if len(result.ToolCalls) > 0 && !offerTools {
			o.logger.Event("tool.stray", map[string]interface{}{"round": round, "calls": len(result.ToolCalls)})
			result.ToolCalls = nil
			result.Provenance = provenance
			result.Checks = checks
			return result, nil
		}
		if len(result.ToolCalls) == 0 {
			result.Provenance = provenance
			result.Checks = checks
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
				case "propose_check":
					if o.mechanicsEngagement != "ask" {
						messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: "error: propose_check is only available when mechanics are set to ask"})
						continue
					}
					req, parseErr := harness.ParseCheckRequest(call.Arguments)
					if parseErr != nil {
						messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: "error: " + parseErr.Error()})
						continue
					}
					result.PendingCheck = &harness.PendingCheck{Ref: call.ID, Request: *req, ProposedBy: "gm"}
					result.Provenance = provenance
					result.ToolCalls = nil
					return result, nil
				default:
					messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: "error: unsupported turn tool " + call.Name})
					continue
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
				Content: "Tools are no longer available for this turn. Complete the turn now by providing your narration as text.",
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

func hasProseRuleBreak(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			return true
		}
	}
	return false
}
