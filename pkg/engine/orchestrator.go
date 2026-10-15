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
		req.Notation = core.DefaultCheckNotation
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
//
// It reads as the phases of a turn: the prologue, the command if the input was
// one, what kind of turn this is, the prompt, a check left pending by an earlier
// turn, the streamed generation and its roll continuations, the reply's
// finalisation, then the turn's script, its records and the art it queued. Each
// phase is a method below, in this order.
func (o *TurnOrchestrator) ProcessActionStream(ctx context.Context, mode, actionInput string, onChunk func(text string) error) (*Turn, error) {
	run, err := o.beginTurn(ctx, mode, actionInput)
	if err != nil {
		return nil, err
	}
	defer run.finish()

	if turn, handled, err := o.handleSlashCommand(run.ctx, run); handled || err != nil {
		return turn, err
	}

	if err := o.resolveTurnMode(run); err != nil {
		return nil, err
	}
	o.gatherTurnContext(run)
	if err := o.assembleTurnPrompt(run.ctx, run); err != nil {
		return nil, err
	}
	o.resolvePendingCheckBeforeGeneration(run.ctx, run)
	o.installStreamObservers(run, onChunk)
	o.beginRollLoop(run)
	if err := o.runRollContinuationLoop(run.ctx, run); err != nil {
		return nil, err
	}
	if err := o.finaliseNarration(run.ctx, run); err != nil {
		return nil, err
	}

	o.buildTurn(run)
	o.startTurnExtraction(run.ctx, run)
	o.applyTurnRecords(run)
	o.assembleTurnSegments(run)
	o.annotateTurnSegments(run)
	o.checkTurnContinuity(run.ctx, run)
	if err := o.applyProposedMove(run); err != nil {
		return nil, err
	}
	if err := o.recordTurn(run.ctx, run); err != nil {
		return nil, err
	}
	o.enqueueSceneArt(run)
	o.enqueuePortraitVersions(run)
	return &run.turn, nil
}

// turnRun is the state of one turn in flight. beginTurn creates it, the phases
// of ProcessActionStream read and write it, and it is discarded when the turn
// returns. Its fields are what used to be one function's locals, named: an
// orchestrator field that outlives a turn (the parser, the roster, the logger)
// stays on the orchestrator rather than appearing here.
type turnRun struct {
	ctx       context.Context
	mode      string
	input     string
	pastTurns []Turn
	turnNum   int
	sceneOnly bool

	turnSpan     oteltrace.Span
	finaliseSpan oteltrace.Span
	started      time.Time
	outcome      string

	// Mode resolution: what this turn is, and what the model is told to do.
	isOpening        bool
	isCorrection     bool
	rollRes          *rules.RollResult
	gmDirective      string
	pendingCheck     *harness.PendingCheck
	generationPrompt string

	// Context assembly.
	locationID          string
	assembly            harness.AssembleResult
	recent              []harness.RecentTurn
	summary             string
	summaryVersion      int
	threads             []string
	workingSet          WorkingSet
	workingSetSelection []harness.Ref
	canCallTools        bool
	echoAction          bool
	openingScene        string
	openingHooks        bool

	// A check the GM proposed on an earlier turn, resolved before generation.
	resolvedPending      *harness.CheckResult
	validationEngagement string
	resolvedRef          string
	continuationOf       int

	// Generation: the streamed reply, the script it produced, and the rolls it
	// resolved along the way.
	onChunk         func(string) error
	result          streamResult
	collected       []turnstream.Event
	collectedCount  int
	rollResults     []harness.CheckResult
	rollAnchors     []rollAnchor
	narrationParts  []string
	endedOnRoll     bool
	cause           CutCause
	narration       string
	recovery        RecoveryOutcome
	stillIncomplete bool

	// The turn being built, and what the extractor found for it.
	turn           Turn
	personae       []harness.PersonaDecl
	memories       []harness.MemoryDecl
	stateChanges   []harness.StateChangeDecl
	moveRef        string
	extraction     harness.Extraction
	extractionDone chan struct{}
	extractionErr  error
}

// rollAnchor records where a resolved roll's check belongs in the turn's script:
// at the first segment the continuation produced, so the dice render where the
// roll happened rather than leading the turn.
type rollAnchor struct {
	segmentIndex int
	checkID      string
}

// maxRollContinuations bounds the roll-continuation loop, so a model that keeps
// rolling cannot run forever.
const maxRollContinuations = 3

// finish ends the turn's spans and records its duration and completion metrics,
// in the order the entry point's defers ran in when this was one function.
func (r *turnRun) finish() {
	if r.finaliseSpan != nil {
		r.finaliseSpan.End()
	}
	r.turnSpan.SetAttributes(attribute.String("turn.outcome", r.outcome))
	metrics := engineMetrics()
	attributes := otelmetric.WithAttributes(
		attribute.String("turn.mode", r.mode),
		attribute.String("turn.outcome", r.outcome),
	)
	metrics.turnDuration.Record(context.Background(), float64(time.Since(r.started).Milliseconds()), attributes)
	metrics.turnCompleted.Add(context.Background(), 1, otelmetric.WithAttributes(
		attribute.String("turn.mode", r.mode),
		attribute.String("turn.outcome", r.outcome),
	))
	r.turnSpan.End()
}

// beginTurn loads the campaign's history, works out which turn this is, sets the
// engagement cadence floor, and starts the turn's span. It returns the state the
// turn's phases share.
func (o *TurnOrchestrator) beginTurn(ctx context.Context, mode, actionInput string) (*turnRun, error) {
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
		o.logger.Event("mechanics.cadence", map[string]any{"quiet_turns": quietTurns(pastTurns)})
	}

	o.logger = trace.OrNil(o.logger)
	o.logger.Event("turn.begin", map[string]any{
		"number":      turnNum,
		"mode":        mode,
		"input_chars": len([]rune(actionInput)),
	})

	spanCtx, turnSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "turn",
		oteltrace.WithAttributes(
			attribute.String("game.id", o.gameID()),
			attribute.Int("turn.number", turnNum),
			attribute.String("turn.mode", mode),
		),
	)

	return &turnRun{
		ctx:              spanCtx,
		mode:             mode,
		input:            actionInput,
		pastTurns:        pastTurns,
		turnNum:          turnNum,
		sceneOnly:        sceneOnly,
		turnSpan:         turnSpan,
		started:          time.Now(),
		generationPrompt: actionInput,
	}, nil
}

// handleSlashCommand answers an input that is a command rather than an action:
// /undo, /recap or /go. It reports whether the input was one, so the caller can
// return the command's own turn without playing one.
func (o *TurnOrchestrator) handleSlashCommand(ctx context.Context, run *turnRun) (*Turn, bool, error) {
	trimmed := strings.TrimSpace(run.input)
	switch {
	case strings.HasPrefix(trimmed, "/undo"):
		turn, err := o.handleUndo(run)
		return turn, true, err
	case strings.HasPrefix(trimmed, "/recap"):
		turn, err := o.handleRecap(ctx, run)
		return turn, true, err
	case strings.HasPrefix(trimmed, "/go "):
		turn, err := o.handleGo(ctx, run)
		return turn, true, err
	}
	return nil, false, nil
}

// handleUndo rewinds the campaign by one turn.
func (o *TurnOrchestrator) handleUndo(run *turnRun) (*Turn, error) {
	if len(run.pastTurns) == 0 {
		return nil, fmt.Errorf("no turns to undo")
	}
	if err := o.timeline.RewindToTurn(len(run.pastTurns) - 1); err != nil {
		return nil, fmt.Errorf("undo failed: %w", err)
	}
	return &Turn{
		Number:    len(run.pastTurns) - 1,
		Timestamp: time.Now(),
		Mode:      "System",
		Input:     "/undo",
		Narration: "Undid previous turn.",
	}, nil
}

// handleRecap answers "where were we?". A stale summary is brought up to date
// first, because asking for a recap is asking about the present tense and a
// summary can be a cadence behind. The regeneration is the same path a turn
// uses, so a failure leaves the old summary in place rather than failing the
// command.
func (o *TurnOrchestrator) handleRecap(ctx context.Context, run *turnRun) (*Turn, error) {
	chronicle := Chronicle{}
	if o.chronicler != nil {
		if due, err := o.chronicler.Due(o.gameID()); err == nil && due {
			if _, err := o.chronicler.Regenerate(ctx, o.gameID()); err != nil {
				o.logger = trace.OrNil(o.logger)
				o.logger.Event("provider.error", map[string]any{"role": "summariser", "error": err.Error()})
			}
		}
		chronicle, _ = o.chronicler.Recap(o.gameID())
	}

	narration := "This campaign has not turned far enough for a recap yet."
	if strings.TrimSpace(chronicle.Summary) != "" {
		narration = fmt.Sprintf("## Story So Far (through turn %d)\n\n%s", chronicle.ThroughTurn, chronicle.Summary)
	}

	return &Turn{
		Number:    run.turnNum,
		Timestamp: time.Now(),
		Mode:      "System",
		Input:     "/recap",
		Narration: narration,
	}, nil
}

// handleGo is an explicit move, recorded as its own system turn.
func (o *TurnOrchestrator) handleGo(ctx context.Context, run *turnRun) (*Turn, error) {
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(run.input), "/go "))

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
		Number:    run.turnNum,
		Timestamp: time.Now(),
		Mode:      "System",
		Input:     run.input,
		Narration: fmt.Sprintf("You make your way to %s.", ent.Name),
		Location:  ent.ID,
		Entities:  moveEntities,
	}
	if err := o.timeline.RecordTurnContext(ctx, &move, nil); err != nil {
		return nil, fmt.Errorf("record move: %w", err)
	}
	return &move, nil
}

// resolveTurnMode decides what kind of turn this is. The opening turn
// establishes the campaign before the player acts, and is only meaningful as
// turn one, so a later attempt is refused rather than silently rewriting the
// scene the party is already standing in. A correction carries a director note;
// a player's Roll asks for a check; anything else runs through the mechanics
// hook if one is loaded.
func (o *TurnOrchestrator) resolveTurnMode(run *turnRun) error {
	isOpening := strings.EqualFold(run.mode, OpeningMode)
	if isOpening {
		if len(run.pastTurns) > 0 {
			return fmt.Errorf("the campaign has already begun")
		}
		run.mode = OpeningMode
		// The scene is the turn's frame, not the player's action: handing it over
		// as the action made the GM respond to it instead of restating it.
		run.generationPrompt = ""
	}
	run.isOpening = isOpening

	// Handle /gm director note or mode
	isCorrection := !isOpening && (run.mode == "GM" || strings.HasPrefix(run.input, "/gm "))
	run.isCorrection = isCorrection
	if isCorrection {
		directiveText := strings.TrimPrefix(run.input, "/gm ")
		run.gmDirective = fmt.Sprintf("[DIRECTOR CORRECTION DIRECTIVE: %s]", directiveText)
	} else if !isOpening && strings.EqualFold(run.mode, "Roll") && o.mechanicsEngagement != config.EngagementOff && o.pendingCheckRef == "" {
		// A player-initiated roll is a check the player resolves: the turn ends on
		// a pending check, so the roll card can present the stakes. A Roll that
		// carries a pending ref is resolving an existing check, not asking for a
		// new one, so it falls through to the resolution path.
		req := o.playerRollRequest(run.input)
		run.pendingCheck = &harness.PendingCheck{Ref: rollRef(run.turnNum, 0), Request: req, ProposedBy: harness.ProposedByPlayer}
		run.gmDirective = fmt.Sprintf("[PLAYER ROLL REQUESTED: %s]", strings.TrimSpace(run.input))
	} else if !isOpening && o.rulesEngine != nil {
		// Run action through mechanics hook if available
		res, err := o.rulesEngine.ExecuteAction(strings.ToLower(run.mode), map[string]any{
			"action": run.input,
			"player": o.playerID,
		})
		if err == nil && res != nil {
			run.rollRes = res.Roll
			run.outcome = res.Outcome
			if res.Message != "" {
				run.gmDirective = fmt.Sprintf("[MECHANICS RESULT: %s]", res.Message)
			}
		}
	}
	return nil
}

// gatherTurnContext collects everything the prompt is built from: where the turn
// happens, the world's own advance, a window of recent turns, long memory, open
// threads and the working set, plus the decisions about tool calls and echoing
// the action that shape the request.
func (o *TurnOrchestrator) gatherTurnContext(run *turnRun) {
	// Where this turn happens: read from the player's own note, which is the single
	// source of truth, never from a value pinned at campaign open.
	run.locationID = o.currentLocation()

	// The living world advances on its own cadence, before the prompt is built,
	// so a tick's injected directive reaches the turn that triggered it.
	o.runWorldTick(run.turnNum, run.locationID)

	// Assemble context with system rules, world lore prompts, and a window of
	// recent turns, which is what keeps the narrator in the same conversation.
	run.recent = make([]harness.RecentTurn, 0, len(run.pastTurns))
	for _, turn := range run.pastTurns {
		run.recent = append(run.recent, harness.RecentTurn{
			Number:    turn.Number,
			Mode:      turn.Mode,
			Input:     turn.Input,
			Narration: turn.Narration,
		})
	}

	// Long memory is a recollection, not canon, so it is injected as one and canon
	// wins wherever they disagree.
	if o.chronicler != nil {
		if chronicle, err := o.chronicler.Recap(o.gameID()); err == nil {
			run.summary = chronicle.Summary
			run.summaryVersion = chronicle.ThroughTurn
		}
	}

	// Open threads are canon, so they are built whether or not anything in the scene
	// touches them: a thread the party has walked away from is the one most likely to
	// be forgotten.
	run.threads = make([]string, 0)
	if open, err := OpenThreads(o.store, run.turnNum); err == nil {
		for index, thread := range open {
			if index == o.threadsCap() {
				break
			}
			run.threads = append(run.threads, fmt.Sprintf("%s (%s, last advanced %d turns ago)", thread.Name, thread.Status, thread.Idle))
		}
	}

	if o.store != nil {
		if records, err := o.store.LoadWorkingSet(); err == nil && len(records) > 0 {
			run.workingSet.FromStorageRecords(records)
		}
	}
	run.workingSetSelection = run.workingSet.Select(8)

	isCaller := false
	if gmProvider, err := o.router.GetProviderForRole(config.RoleGM); err == nil && gmProvider != nil {
		if caller, ok := gmProvider.(harness.ToolCaller); ok {
			isCaller = caller.ToolCallerCapable()
		}
	}
	run.canCallTools = o.offersTools(isCaller)

	run.echoAction = o.actionEcho &&
		!run.isOpening && !run.isCorrection &&
		!strings.EqualFold(run.mode, "Roll") &&
		!strings.EqualFold(run.mode, "Say") &&
		strings.TrimSpace(run.input) != ""

	// The opening turn restates the campaign's scene. A quiet scene turn suppresses
	// the hooks only when there is a scene to restate; without one the turn still
	// establishes a scene from the world.
	if run.isOpening {
		run.openingScene = o.openingPrompt
		run.openingHooks = !run.sceneOnly || strings.TrimSpace(run.openingScene) == ""
	}
}

// assembleTurnPrompt builds the request from the gathered context, gives the
// turn-begin hook its chance to act, and folds any directive a hook injected
// into the turn's instruction to the model.
func (o *TurnOrchestrator) assembleTurnPrompt(ctx context.Context, run *turnRun) error {
	assembly, err := o.assembler.Assemble(harness.ContextRequest{
		Context:          ctx,
		LocationID:       run.locationID,
		PlayerID:         o.playerID,
		Action:           run.generationPrompt,
		OpeningScene:     run.openingScene,
		OpeningHooks:     run.openingHooks,
		PlayerName:       o.playerDisplayName(),
		ActionEcho:       run.echoAction,
		RulesPrompt:      o.rulesPrompt,
		LorePrompt:       o.lorePrompt,
		MechanicsPrompt:  o.mechanicsInstruction(),
		Profiles:         o.timeline.VoiceProfiles(),
		OmitVoiceCatalog: run.canCallTools,
		Recent:           run.recent,
		TurnNumber:       run.turnNum,
		Mode:             run.mode,
		Summary:          run.summary,
		SummaryVersion:   run.summaryVersion,
		WorkingSet:       run.workingSetSelection,
		Threads:          run.threads,
		SpeechCues:       o.speechCues,
	})
	if err != nil {
		return fmt.Errorf("assemble context: %w", err)
	}
	run.assembly = assembly

	if o.rulesEngine != nil {
		if err := o.rulesEngine.ExecuteTurnBegin(map[string]any{
			"turn":     run.turnNum,
			"location": run.locationID,
		}); err != nil {
			o.logger.Event("turn.begin_hook_error", map[string]any{"error": err.Error()})
		}
	}

	// Directives hooks injected (onAction, onWorldTick, onTurnBegin) reach the
	// prompt here. Without draining them, injectGMDirection had no effect.
	if directives := o.drainDirectives(); len(directives) > 0 {
		run.gmDirective = applyDirectives(run.gmDirective, directives)
		if o.worldTickDue(run.turnNum) {
			o.worldTick = strings.Join(directives, "\n")
		}
	}
	return nil
}

// resolvePendingCheckBeforeGeneration resolves a check the GM proposed on an
// earlier turn, so the model narrates its outcome rather than proposing it
// again. A roll a previous attempt already resolved is reused, so a retried
// request cannot change the outcome the player already saw.
func (o *TurnOrchestrator) resolvePendingCheckBeforeGeneration(ctx context.Context, run *turnRun) {
	run.turnSpan.SetAttributes(attribute.String("turn.assembled_prompt", run.assembly.Prompt))
	run.validationEngagement = o.mechanicsEngagement
	if o.pendingCheckRef == "" {
		return
	}
	run.resolvedRef = o.pendingCheckRef
	run.continuationOf = findPendingTurn(run.pastTurns, o.pendingCheckRef)
	if pending := findPendingCheck(run.pastTurns, o.pendingCheckRef); pending != nil {
		resolved := findResolvedCheck(run.pastTurns, o.pendingCheckRef)
		if resolved == nil {
			if fresh, resolveErr := o.resolveCheck(ctx, pending.Request, nil); resolveErr == nil {
				fresh.CheckID = pending.Ref
				resolved = fresh
			} else {
				o.logger.Event("pending.resolve_error", map[string]any{"error": resolveErr.Error()})
			}
		}
		if resolved != nil {
			run.resolvedPending = resolved
			run.validationEngagement = "auto"
			directive := fmt.Sprintf("[PLAYER ROLL: %s — %s]", resolved.Outcome, strings.TrimSpace(pending.Request.Stakes))
			run.gmDirective = strings.TrimSpace(directive + "\n" + run.gmDirective)
		}
	}
	o.pendingCheckRef = ""
}

// installStreamObservers wraps the caller's chunk callback twice: once to
// measure time to first token, and once to parse the reply into segments as it
// arrives. The parser wraps the TTFT listener, so the client still sees each raw
// chunk first. The player's own line, when this turn says something, is observed
// immediately so the streamer voices it before the narrator begins.
func (o *TurnOrchestrator) installStreamObservers(run *turnRun, onChunk func(text string) error) {
	run.onChunk = onChunk
	if onChunk != nil {
		started := time.Now()
		first := true
		inner := onChunk
		run.onChunk = func(text string) error {
			if first {
				first = false
				elapsed := time.Since(started).Milliseconds()
				engineMetrics().turnTTFT.Record(run.ctx, float64(elapsed))
				o.logger.Event("turn.ttft", map[string]any{"ms": elapsed})
			}
			return inner(text)
		}
	}

	// The reply is parsed as it streams, so segments and their speakers are
	// attributed while the model is still writing. The roster is seeded from the
	// store, and a persona record extends it mid-stream.
	o.roster = newRoster(o.store, o.playerID, o.playerDisplayName(), o.timeline.VoiceProfiles())
	o.parser = turnstream.NewParser(o.roster)
	if run.onChunk != nil {
		inner := run.onChunk
		run.onChunk = func(text string) error {
			if err := inner(text); err != nil {
				return err
			}
			o.observeSegments(o.parser.Feed(text))
			return nil
		}
	} else {
		run.onChunk = func(text string) error {
			o.observeSegments(o.parser.Feed(text))
			return nil
		}
	}

	// A player's spoken line leads the turn: observe it immediately so the
	// client receives its segment and the streamer voices it in the player's
	// voice before the narrator begins.
	if beat := playerSegment(run.mode, run.input, o.playerID, o.playerDisplayName()); beat != nil {
		o.observeSegments([]turnstream.Event{{
			Kind:      turnstream.KindSpeech,
			Speaker:   beat.Speaker,
			SpeakerID: beat.SpeakerID,
			Text:      beat.Text,
			Player:    true,
		}})
	}
}

// beginRollLoop anchors a check resolved before this continuation. It has no
// @roll event to place it, so it attaches to the continuation's opening
// narration, where the consequence is narrated, rather than leading the turn
// detached. The loop's other accumulators (the reply, the script, the resolved
// rolls) start empty and are filled by the loop itself.
func (o *TurnOrchestrator) beginRollLoop(run *turnRun) {
	if run.resolvedPending != nil {
		run.rollAnchors = append(run.rollAnchors, rollAnchor{segmentIndex: 0, checkID: run.resolvedPending.CheckID})
	}
}

// drainTurnEvents moves the parser's accumulated events into the turn.
// collectedCount is how many of them are already in the turn, so a recovery
// continuation's events can be appended without duplicating the ones the loop
// already took.
func (o *TurnOrchestrator) drainTurnEvents(run *turnRun) {
	if o.parser == nil {
		return
	}
	o.parser.Flush()
	events := o.parser.Events()
	if run.collectedCount < len(events) {
		run.collected = append(run.collected, events[run.collectedCount:]...)
		run.collectedCount = len(events)
	}
}

// runRollContinuationLoop generates the reply and continues it while it ends on
// a @roll record. The loop is bounded, so a model that keeps rolling cannot run
// forever. A player-initiated roll ends the turn on the pending check it asked
// for, unless the model already proposed one of its own.
func (o *TurnOrchestrator) runRollContinuationLoop(ctx context.Context, run *turnRun) error {
	for attempt := 0; ; attempt++ {
		if err := o.streamTurnReply(ctx, run); err != nil {
			return err
		}
		if strings.TrimSpace(run.result.Text) != "" {
			run.narrationParts = append(run.narrationParts, run.result.Text)
		}
		o.drainTurnEvents(run)

		if !o.resolveEndingRoll(ctx, run, attempt) {
			break
		}
	}
	if run.pendingCheck != nil && run.result.PendingCheck == nil {
		run.result.PendingCheck = run.pendingCheck
	}
	run.result.Checks = append(run.result.Checks, run.rollResults...)
	return nil
}

// streamTurnReply makes one generation call. A call that cannot be made fails
// the turn, reported with the code, provider, prompt size and partial length the
// trace and the log need, and nothing is recorded for it.
func (o *TurnOrchestrator) streamTurnReply(ctx context.Context, run *turnRun) error {
	result, err := o.runGenerationLoop(ctx, &run.assembly, run.gmDirective, run.resolvedPending, run.validationEngagement, run.onChunk)
	run.result = result
	if err == nil {
		return nil
	}
	run.outcome = "error"
	run.turnSpan.SetAttributes(attribute.String("turn.raw_completion", result.Text))
	failure, _ := harness.FailureFrom(err)
	fields := map[string]any{
		"code":          generationCode(failure),
		"error":         err.Error(),
		"role":          config.RoleGM,
		"provider":      result.ProviderID,
		"prompt_chars":  len([]rune(run.input)),
		"context_chars": len([]rune(run.assembly.Prompt)),
		"elapsed_ms":    time.Since(run.started).Milliseconds(),
		"chunk_count":   result.ChunkCount,
		"partial_chars": len([]rune(result.Text)),
	}
	if failure != nil {
		fields["attempts"] = len(failure.Attempts)
		run.turnSpan.SetAttributes(
			attribute.String("turn.failure_code", string(failure.Code)),
			attribute.String("turn.failure_message", failure.Message),
			attribute.String("localrpg.generation.failure_code", string(failure.Code)),
			attribute.Int("localrpg.generation.attempts", len(failure.Attempts)),
		)
		run.turnSpan.RecordError(failure)
		run.turnSpan.SetStatus(codes.Error, string(failure.Code))
	} else {
		code := string(harness.ClassifyProviderError(err))
		run.turnSpan.SetAttributes(attribute.String("turn.failure_code", code))
		run.turnSpan.SetStatus(codes.Error, code)
		run.turnSpan.RecordError(err)
	}
	if result.Failure != nil {
		fields["finish_reason"] = result.Failure.FinishReason
	}
	o.logger.Event("generation.error", fields)
	return fmt.Errorf("gm generation failed: %w", err)
}

// resolveEndingRoll decides what a reply that ended on a @roll record means.
// Under the ask policy it becomes the pending check the player rolls; under auto
// the engine rolls at once and the model continues from the outcome. It reports
// whether the loop should continue.
func (o *TurnOrchestrator) resolveEndingRoll(ctx context.Context, run *turnRun, attempt int) bool {
	req, hasRoll := o.pendingRoll()
	run.endedOnRoll = hasRoll && run.result.PendingCheck == nil
	if !run.endedOnRoll || o.mechanicsEngagement == config.EngagementOff {
		return false
	}
	if o.mechanicsEngagement == config.EngagementAsk {
		run.result.PendingCheck = &harness.PendingCheck{Ref: rollRef(run.turnNum, 0), Request: req, ProposedBy: harness.ProposedByGM}
		return false
	}
	if attempt >= maxRollContinuations {
		o.logger.Event("roll.cap", map[string]any{"continuations": attempt})
		if resolved, resolveErr := o.resolveCheck(ctx, req, nil); resolveErr == nil {
			resolved.CheckID = rollRef(run.turnNum, len(run.rollResults))
			run.rollResults = append(run.rollResults, *resolved)
			run.result.RollOutcome = strings.TrimSpace(req.Outcomes[resolved.Outcome])
			run.result.RollCheckRef = resolved.CheckID
		}
		return false
	}
	resolved, resolveErr := o.resolveCheck(ctx, req, nil)
	if resolveErr != nil {
		o.logger.Event("roll.resolve_error", map[string]any{"error": resolveErr.Error()})
		return false
	}
	resolved.CheckID = rollRef(run.turnNum, len(run.rollResults))
	run.rollResults = append(run.rollResults, *resolved)
	run.rollAnchors = append(run.rollAnchors, rollAnchor{
		segmentIndex: len(segmentsFromEvents(run.collected)),
		checkID:      resolved.CheckID,
	})
	run.gmDirective = rollContinuationDirective(*resolved, req)
	run.resolvedPending = nil
	if o.parser != nil {
		o.parser.Reset()
		run.collectedCount = 0
	}
	return true
}

// finaliseNarration turns the streamed parts into the turn's narration. It
// records the context the reply was produced from, classifies how the reply was
// cut and recovers it, appends a resolved roll's outcome where the dice belong,
// and strips the control records that are protocol rather than story.
func (o *TurnOrchestrator) finaliseNarration(ctx context.Context, run *turnRun) error {
	_, finaliseSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "turn.finalise")
	run.finaliseSpan = finaliseSpan

	run.turnSpan.SetAttributes(
		attribute.String("turn.raw_completion", run.result.Text),
		attribute.String("localrpg.context.strategy", string(run.assembly.Context.Strategy)),
		attribute.String("localrpg.context.prefix_hash", run.assembly.Context.PrefixHash),
		attribute.Int("localrpg.provider.cached_tokens", run.assembly.Context.CachedTokens),
	)
	if strings.TrimSpace(run.result.Text) == "" && len(run.result.ToolCalls) == 0 {
		run.turnSpan.SetAttributes(attribute.String("turn.failure_code", "empty_response"))
	}
	if run.assembly.Context.Session != nil {
		run.turnSpan.SetAttributes(attribute.String("localrpg.context.session_id", run.assembly.Context.Session.ID))
	}

	if err := o.classifyAndRecoverReply(ctx, run); err != nil {
		return err
	}

	if outcome := strings.TrimSpace(run.result.RollOutcome); outcome != "" {
		if strings.TrimSpace(run.narration) == "" {
			run.narration = outcome
		} else {
			run.narration = strings.TrimSpace(run.narration) + "\n\n" + outcome
		}
	}

	o.logger.Event("generation.complete", map[string]any{
		"narration_chars": len([]rune(run.narration)),
		"finish_reason":   run.result.FinishReason,
		"cause":           run.cause.String(),
		"recovery":        string(run.recovery),
		"truncated":       run.stillIncomplete,
	})

	// Control records are protocol, not story: strip them from the prose that is
	// recorded, extracted, and shown. The parser keeps them for the declarations
	// applied below.
	run.narration = stripRecordLines(run.narration)
	return nil
}

// classifyAndRecoverReply works out how the reply was cut and repairs it. A
// pending check is not an empty turn: the prose the model wrote before the check
// is the setup the player reads while deciding, so recovery runs for it too and
// only a genuinely empty reply is an error.
func (o *TurnOrchestrator) classifyAndRecoverReply(ctx context.Context, run *turnRun) error {
	// The final call may be cut off; earlier continuations ended on a roll and
	// are complete by construction, so only the last needs classification.
	if run.endedOnRoll {
		run.cause = cutNone
	} else {
		run.cause = o.classifyCut(run.result)
	}
	run.narration, run.recovery, run.stillIncomplete = o.recoverReply(ctx, strings.Join(run.narrationParts, "\n\n"), run.cause, run.onChunk)
	// The recovery pass streams its continuation through onChunk, so its
	// segments are in the parser now and must join the turn's script.
	o.drainTurnEvents(run)
	if strings.TrimSpace(run.narration) == "" && run.result.PendingCheck == nil {
		run.outcome = "error"
		failure := &harness.GenerationFailure{
			Code:         harness.FailureEmptyResponse,
			Message:      "gm returned no narration",
			FinishReason: run.result.FinishReason,
			ElapsedMS:    time.Since(run.started).Milliseconds(),
		}
		if run.result.Failure != nil {
			failure = run.result.Failure
		}
		run.turnSpan.SetAttributes(attribute.String("localrpg.generation.failure_code", string(failure.Code)))
		run.turnSpan.RecordError(failure)
		run.turnSpan.SetStatus(codes.Error, string(failure.Code))
		o.logger.Event("generation.error", map[string]any{
			"code":          string(failure.Code),
			"error":         failure.Message,
			"role":          config.RoleGM,
			"provider":      run.result.ProviderID,
			"prompt_chars":  len([]rune(run.input)),
			"elapsed_ms":    time.Since(run.started).Milliseconds(),
			"chunk_count":   run.result.ChunkCount,
			"partial_chars": len([]rune(run.result.Text)),
			"attempts":      len(failure.Attempts),
			"finish_reason": run.result.FinishReason,
		})
		return failure
	}
	return nil
}

// buildTurn is the record of the turn, before its script and its notes are
// attached.
func (o *TurnOrchestrator) buildTurn(run *turnRun) {
	contextPrompt := run.assembly.Prompt
	if run.gmDirective != "" {
		contextPrompt = run.gmDirective + "\n\n" + contextPrompt
	}
	run.turn = Turn{
		Number:           run.turnNum,
		Timestamp:        time.Now(),
		Mode:             run.mode,
		Input:            run.input,
		Roll:             run.rollRes,
		Narration:        run.narration,
		Location:         run.locationID,
		Outcome:          run.outcome,
		Truncated:        run.stillIncomplete,
		Recovery:         string(run.recovery),
		ContextNotes:     run.assembly.Trimmed,
		Context:          &run.assembly.Context,
		Prompt:           contextPrompt,
		ToolCalls:        run.result.Provenance,
		PendingCheck:     run.result.PendingCheck,
		Draft:            o.singleTurn && run.result.PendingCheck != nil,
		ResolvesCheckRef: run.resolvedRef,
		ContinuationOf:   run.continuationOf,
	}
}

// startTurnExtraction runs the entity extractor concurrently. Extraction is a
// model call, so it starts before the local mention and segment work and is
// awaited just before the segments are built. A failed extractor must not lose
// the turn.
func (o *TurnOrchestrator) startTurnExtraction(ctx context.Context, run *turnRun) {
	run.extractionDone = make(chan struct{})
	if o.extractor == nil {
		close(run.extractionDone)
		return
	}
	go func() {
		defer close(run.extractionDone)
		extractCtx, extractSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "extract.entities")
		defer extractSpan.End()
		if extracted, err := o.extractor.Extract(extractCtx, run.turn.Narration); err == nil {
			run.extraction = *extracted
		} else {
			run.extractionErr = err
			extractSpan.RecordError(err)
			extractSpan.SetStatus(codes.Error, string(harness.ClassifyProviderError(err)))
			o.logger.Event("extract.error", map[string]any{"error": err.Error()})
		}
		extractSpan.SetAttributes(attribute.Int("localrpg.entities.extracted", len(run.extraction.Entities)))
	}()
}

// applyTurnRecords records the local mentions and the declarations the stream
// carried inline: a persona before the line that speaks, a state change after
// the roll it follows. It then awaits the extractor, whose failure costs the
// turn nothing.
func (o *TurnOrchestrator) applyTurnRecords(run *turnRun) {
	run.turn.Entities = harness.ResolveEntityMentions(o.store, o.playerID, run.locationID, run.turn.Narration, run.input)

	run.moveRef = ""
	if o.parser != nil {
		run.personae, run.memories, run.stateChanges, run.moveRef = o.applyRecords()
		o.logRepairReport()
	}
	for _, persona := range run.personae {
		if id := entity.Slugify(persona.Name); id != "" {
			run.turn.Personae = append(run.turn.Personae, id)
		}
	}
	run.turn.Checks = run.result.Checks
	run.turn.RecordReport = o.recordReport()

	<-run.extractionDone
	if run.extractionErr != nil {
		o.logger.Event("extract.error", map[string]any{"error": run.extractionErr.Error()})
	}
}

// assembleTurnSegments turns the streamed events into the turn's playback
// script: the segments themselves, any dialogue the extractor attributed that
// the parser missed, an identity a persona revealed, the resolved rolls placed
// where they happened, and the player's own line leading it.
func (o *TurnOrchestrator) assembleTurnSegments(run *turnRun) {
	// The streamed reply was parsed as it arrived, across every continuation; its
	// events are the turn's playback script. A reply with no framing at all falls
	// back to the legacy prose parser plus the extractor. Any dialogue that slipped
	// through stream parsing as narration is rescued using extractor attributions.
	if parsed := segmentsFromEvents(run.collected); len(parsed) > 0 {
		run.turn.Segments = parsed
		if len(run.extraction.Dialogue) > 0 {
			resolve := func(candidate string) (string, bool) {
				if o.roster != nil {
					if id, ok := o.roster.Resolve(candidate); ok {
						return id, true
					}
				}
				if id := harness.ResolveSpeakerID(o.store, candidate); id != "" {
					return id, true
				}
				return proposedSpeakerID(run.extraction.Entities, candidate)
			}
			run.turn.Segments = mergeAttributions(run.turn.Segments, run.extraction.Dialogue, resolve)
		}
	} else {
		run.turn.Segments = buildTurnSegments(o.store, run.turn.Narration, run.extraction)
	}

	// Remap earlier segments if a declared persona revealed an earlier identity.
	for _, persona := range run.personae {
		if id := entity.Slugify(persona.Name); id != "" {
			if prev := strings.TrimSpace(persona.PreviousIdentity()); prev != "" {
				prevSlug := entity.Slugify(prev)
				for i := range run.turn.Segments {
					if run.turn.Segments[i].SpeakerID == prevSlug || run.turn.Segments[i].Speaker == prev {
						run.turn.Segments[i].SpeakerID = id
					}
				}
			}
		}
	}

	// A resolved roll's outcome is narrated after the prose that led to it, so the
	// chronicle can render the dice inline with the consequence.
	if outcome := strings.TrimSpace(run.result.RollOutcome); outcome != "" {
		run.turn.Segments = append(run.turn.Segments, entity.TurnSegment{
			Kind:     entity.SegmentNarration,
			Text:     outcome,
			CheckRef: run.result.RollCheckRef,
		})
	}

	// The player's own spoken line leads the turn, so it is heard in their voice
	// before the narrator answers. If the narrator's generated text already begins
	// with the player's line, attachPlayerSegment marks it rather than duplicating it.
	modelSegments := len(run.turn.Segments)
	run.turn.Segments = attachPlayerSegment(run.turn.Segments, run.mode, run.input, o.playerID, o.playerDisplayName())
	// A player segment is prepended, so every model segment shifts by one. Point
	// each resolved roll's check at the segment its continuation produced, so the
	// chronicle renders the dice where the roll happened.
	shift := len(run.turn.Segments) - modelSegments
	for _, anchor := range run.rollAnchors {
		index := anchor.segmentIndex + shift
		if index < 0 || index >= len(run.turn.Segments) {
			continue
		}
		if run.turn.Segments[index].CheckRef == "" {
			run.turn.Segments[index].CheckRef = anchor.checkID
		}
	}
}

// annotateTurnSegments logs the script, folds the speakers it named into the
// turn's mentions, records a scene break, and anchors each speaker's portrait
// with the character's active version at turn time.
func (o *TurnOrchestrator) annotateTurnSegments(run *turnRun) {
	o.logger.Event("segment.build", map[string]any{
		"count":      len(run.turn.Segments),
		"kinds":      segmentKinds(run.turn.Segments),
		"speakers":   segmentSpeakers(run.turn.Segments),
		"unresolved": unresolvedSpeakers(run.turn.Segments),
	})
	for _, mention := range speechMentions(run.turn.Segments) {
		if !containsMention(run.turn.Entities, mention.ID) {
			run.turn.Entities = append(run.turn.Entities, mention)
		}
	}

	// Detect scene breaks via explicit markdown horizontal rules or extractor cues.
	if hasProseRuleBreak(run.turn.Narration) || (run.extraction.SceneBreak != nil && run.extraction.SceneBreak.Occurred) {
		run.turn.SceneBreak = true
		if run.extraction.SceneBreak != nil && run.extraction.SceneBreak.VisualCue != "" {
			run.turn.SceneBreakCue = run.extraction.SceneBreak.VisualCue
		}
	}

	// Anchor speaker portraits with the character's active version at turn time.
	for i := range run.turn.Segments {
		if run.turn.Segments[i].Kind == entity.SegmentSpeech {
			speakerID := run.turn.Segments[i].SpeakerID
			if speakerID == "" && run.turn.Segments[i].Speaker != "" {
				speakerID = harness.ResolveSpeakerID(o.store, run.turn.Segments[i].Speaker)
			}
			if speakerID == "" && run.turn.Segments[i].Speaker != "" {
				speakerID = entity.Slugify(run.turn.Segments[i].Speaker)
			}
			if speakerID != "" && o.store != nil {
				if ent, err := o.store.GetEntity(speakerID); err == nil && ent != nil {
					version := ent.PortraitVersion
					if version <= 0 && ent.Portrait != "" {
						version = 1
					}
					if version > 0 {
						run.turn.Segments[i].SpeakerPortrait = fmt.Sprintf("/api/game/%s/character/%s/portrait?v=%d", o.gameID(), speakerID, version)
					} else {
						run.turn.Segments[i].SpeakerPortrait = fmt.Sprintf("/api/game/%s/character/%s/portrait", o.gameID(), speakerID)
					}
				}
			}
		}
	}
}

// checkTurnContinuity flags prose that contradicts what the campaign knows. The
// findings are advisory: nothing is rewritten, and a correction is the player's
// to send.
func (o *TurnOrchestrator) checkTurnContinuity(ctx context.Context, run *turnRun) {
	if !o.continuityEnabled() {
		return
	}
	_, continuitySpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "continuity.check")
	findings := CheckContinuity(ContinuityInput{
		Store:      o.store,
		Turn:       &run.turn,
		LocationID: run.locationID,
		PlayerID:   o.playerID,
		Context:    run.assembly.Context,
		WorkingSet: &run.workingSet,
	})
	for _, finding := range findings {
		run.turn.ContinuityNotes = append(run.turn.ContinuityNotes, finding.Note)
	}
	continuitySpan.SetAttributes(attribute.Int("localrpg.continuity.findings", len(findings)))
	continuitySpan.End()
}

// applyProposedMove moves the party when the turn proposed a location. A move
// applies only when it resolves to a real location, and it takes effect from the
// next turn: this turn happened where it started, which is what keeps scenes
// honest. An explicit move record wins over an extracted one.
func (o *TurnOrchestrator) applyProposedMove(run *turnRun) error {
	move := strings.TrimSpace(run.moveRef)
	if move == "" {
		move = strings.TrimSpace(run.extraction.PlayerLocation)
	}
	if move != "" {
		if ent := findLocationByRef(o.store, move); ent != nil && ent.ID != run.locationID {
			if err := o.timeline.SetPlayerLocation(o.playerID, ent.ID); err != nil {
				return fmt.Errorf("apply proposed location: %w", err)
			}
		}
	}
	return nil
}

// recordTurn persists a completed turn and everything around it. Nothing is
// persisted for a cancelled or failed turn: the timeline only ever holds
// completed turns, so a disconnect is a no-op on disk.
func (o *TurnOrchestrator) recordTurn(ctx context.Context, run *turnRun) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("turn cancelled: %w", err)
	}
	o.reconcileExtractedEntities(run)
	if err := o.applyTurnStateChanges(run); err != nil {
		return err
	}
	o.runTurnLifecycle(ctx, run)
	if err := o.persistTurn(ctx, run); err != nil {
		return err
	}
	if err := o.assignPersonaVoices(run); err != nil {
		return err
	}
	if err := o.persistWorkingSet(run); err != nil {
		return err
	}
	o.logRecordedTurn(ctx, run)
	return nil
}

// reconcileExtractedEntities says which of the turn's extracted beings it
// introduced rather than only which it mentioned. MatchExistingEntity answers
// whether a note already exists.
func (o *TurnOrchestrator) reconcileExtractedEntities(run *turnRun) {
	matched := make([]string, 0, len(run.extraction.Entities))
	created := make([]string, 0, len(run.extraction.Entities))
	for i := range run.extraction.Entities {
		proposed := run.extraction.Entities[i]
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
	o.logger.Event("extraction.reconcile", map[string]any{
		"matched": matched,
		"created": created,
	})
}

// applyTurnStateChanges applies the state changes the turn declared, after
// RecordTurn has created and voiced the entities it introduced. A change the
// engine could not apply is recorded rather than fatal: the GM has already
// narrated its consequence, and a reference the index does not hold must not
// cost the player the turn they just played.
func (o *TurnOrchestrator) applyTurnStateChanges(run *turnRun) error {
	if len(run.stateChanges) == 0 || o.rulesEngine == nil {
		return nil
	}
	notes, err := rules.ApplyStateChanges(o.rulesEngine.HostAPI(), run.stateChanges, o.declaredStats, o.allowFreeform)
	if err != nil {
		return fmt.Errorf("apply state changes: %w", err)
	}
	for _, note := range notes {
		trace.OrNil(o.logger).Event("mechanics.state_change_skipped", map[string]any{"note": note})
	}
	return nil
}

// runTurnLifecycle closes the turn's mechanics. Advancement is earned after the
// turn's own state changes, so the award is part of the same record and a
// threshold level sees the final values. Health is resolved twice around the
// post-turn hooks: once before them, so the hook context carries the effects the
// turn's own changes caused, and once after, so a hook that drives a stat to
// zero is seen on this same turn rather than the next.
func (o *TurnOrchestrator) runTurnLifecycle(ctx context.Context, run *turnRun) {
	o.applyAdvancement(ctx, &run.turn)

	run.turn.WorldTick = o.worldTick

	pendingEffects := o.healthOutcomes(&run.turn)
	o.runTurnEndHooks(run.turnNum, &run.turn, pendingEffects)
	finalEffects := o.healthOutcomes(&run.turn)
	run.turn.HealthEffects = append(run.turn.HealthEffects, mergeHealthEffects(pendingEffects, finalEffects)...)
}

// persistTurn writes the turn. In single-turn mode a resolved check completes
// the draft turn in place rather than appending a continuation, so the fiction
// stays one record.
func (o *TurnOrchestrator) persistTurn(ctx context.Context, run *turnRun) error {
	if o.singleTurn && run.continuationOf != 0 {
		run.turn.Number = run.continuationOf
		if err := o.timeline.ReplaceTurnContextStructured(ctx, &run.turn, run.extraction.Entities, run.personae, run.memories, run.result.Checks); err != nil {
			return fmt.Errorf("replace turn: %w", err)
		}
		return nil
	}
	if err := o.timeline.RecordTurnContextStructured(ctx, &run.turn, run.extraction.Entities, run.personae, run.memories, run.result.Checks); err != nil {
		return fmt.Errorf("record turn: %w", err)
	}
	return nil
}

// assignPersonaVoices gives a persona the voice the tool executor assigned it.
// Synthesis must not begin until the turn is recorded, or a character invented
// in this turn would be read in the narrator's voice.
func (o *TurnOrchestrator) assignPersonaVoices(run *turnRun) error {
	exec, ok := o.toolExecutor.(interface {
		AssignedVoices() map[string]config.VoiceProfile
	})
	if !ok {
		return nil
	}
	assigned := exec.AssignedVoices()
	for _, persona := range run.personae {
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
				if err := o.timeline.SaveEntity(ent); err != nil {
					return fmt.Errorf("save assigned voice for entity %q: %w", slugID, err)
				}
			}
		}
	}
	return nil
}

// persistWorkingSet reweights the campaign's active entities and threads with
// what this turn referenced.
func (o *TurnOrchestrator) persistWorkingSet(run *turnRun) error {
	if o.store == nil {
		return nil
	}
	var allTurnRefs []harness.Ref
	if run.turn.Context != nil {
		allTurnRefs = run.turn.Context.Refs
	}
	run.workingSet.Apply(run.turnNum, allTurnRefs)
	if err := o.store.ReplaceWorkingSet(run.workingSet.ToStorageRecords()); err != nil {
		return fmt.Errorf("replace working set: %w", err)
	}
	return nil
}

// logRecordedTurn records what the turn wrote, for the trace.
func (o *TurnOrchestrator) logRecordedTurn(ctx context.Context, run *turnRun) {
	trace.LogEvent(ctx, o.logger, "record.turn", map[string]any{
		"number":          run.turn.Number,
		"location":        run.turn.Location,
		"entities":        len(run.turn.Entities),
		"outcome":         run.turn.Outcome,
		"narration_chars": len([]rune(run.turn.Narration)),
	})
}

// enqueueSceneArt queues the scene image the turn earned, with the prompt that
// describes it and the reference image that keeps one place's look stable.
func (o *TurnOrchestrator) enqueueSceneArt(run *turnRun) {
	if o.sceneWorker == nil || !o.shouldIllustrate(run.turn, run.pastTurns) {
		return
	}
	var locEntity *entity.Entity
	if run.turn.Location != "" && o.store != nil {
		locEntity, _ = o.store.GetEntity(run.turn.Location)
	}
	cue := run.turn.SceneBreakCue
	if cue == "" {
		cue = ExtractSceneCue(run.turn.Narration)
	}
	sceneCtx := ScenePromptContext{
		Cue:       cue,
		Narration: run.turn.Narration,
		Action:    run.turn.Input,
		Location:  run.turn.Location,
		Style:     o.worldArtStyle,
		Cast:      SceneCast(o.store, &run.turn),
	}
	if locEntity != nil {
		sceneCtx.Location = locEntity.Name
		sceneCtx.Appearance = locEntity.Appearance
		sceneCtx.CurrentAppearance = locEntity.CurrentAppearance
	}
	if len(run.turn.Checks) > 0 {
		sceneCtx.Outcome = run.turn.Checks[0].Outcome
	}
	// The scene's stable look keeps successive images of one place together;
	// the reference image is the previous illustration of the same location.
	sceneCtx.Scene = media.NewSceneStyle(run.turn.Location, o.worldArtStyle, sceneCtx.Appearance)
	scenePrompt := BuildScenePrompt(sceneCtx)
	o.sceneWorker.EnqueueScene(o.gameID(), run.turn.Number, SceneJob{
		Prompt:    scenePrompt,
		Reference: o.sceneReference(run.turn.Location, run.pastTurns),
	})
}

// enqueuePortraitVersions queues a new portrait version for anyone the turn gave
// a new appearance to.
func (o *TurnOrchestrator) enqueuePortraitVersions(run *turnRun) {
	if o.portraitWorker == nil {
		return
	}
	for _, raw := range run.extraction.Entities {
		if raw.AppearanceChanged && o.store != nil {
			matched := harness.MatchExistingEntity(o.store, &raw)
			if matched != nil && entity.IsCharacterType(matched.Type) {
				o.portraitWorker.EnqueueVersion(o.gameID(), matched, o.worldArtStyle, true)
			}
		}
	}
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

// streamAccumulator is what one provider stream builds up as it arrives: the text so
// far, the last finish reason it gave, the tool calls it reported, and how many chunks
// carried anything at all. The count is kept because an empty response is a failure the
// caller retries, and a stream of chunks that carried nothing is still not an answer.
type streamAccumulator struct {
	text         strings.Builder
	finishReason string
	toolCalls    []harness.ToolCall
	sawText      bool
	chunkCount   int
}

// result is the stream's answer when it ended cleanly.
func (a *streamAccumulator) result(providerID string) streamResult {
	return streamResult{
		Text:         a.text.String(),
		FinishReason: a.finishReason,
		ToolCalls:    a.toolCalls,
		ProviderID:   providerID,
		ChunkCount:   a.chunkCount,
	}
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

	acc := &streamAccumulator{}

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
			return interruptedStream(acc, provider.ID(), started, fmt.Errorf("%w after %s", ErrGenerationStalled, timeout))

		case chunk, ok := <-chunks:
			if !ok {
				if err := <-streamErr; err != nil {
					// A cancelled client is final: the turn will be discarded, so
					// there is no point keeping text nobody is waiting for.
					if errors.Is(err, context.Canceled) {
						return streamResult{}, err
					}
					return interruptedStream(acc, provider.ID(), started, err)
				}
				return finishStream(acc, provider.ID(), started)
			}
			if chunk.Error != nil {
				<-streamErr
				return interruptedStream(acc, provider.ID(), started, chunk.Error)
			}

			absorbChunk(acc, chunk)
			resetIdleTimer(idle, timeout)
			if err := forwardChunkText(acc, chunk, onChunk); err != nil {
				return streamResult{}, err
			}
		}
	}
}

// absorbChunk records what a chunk reports about the stream as a whole: the tool calls
// it carries, and the reason it gives for finishing.
func absorbChunk(acc *streamAccumulator, chunk harness.StreamChunk) {
	if len(chunk.ToolCalls) > 0 {
		acc.toolCalls = chunk.ToolCalls
	}
	if chunk.FinishReason != "" {
		acc.finishReason = chunk.FinishReason
	}
}

// forwardChunkText counts the chunk, accumulates its text, and hands the text to the
// caller. A chunk carrying neither text nor tool calls is not a chunk of anything, so it
// is not counted.
func forwardChunkText(acc *streamAccumulator, chunk harness.StreamChunk, onChunk func(string) error) error {
	if chunk.Text != "" || len(chunk.ToolCalls) > 0 {
		acc.chunkCount++
	}
	if chunk.Text == "" {
		return nil
	}
	acc.text.WriteString(chunk.Text)
	acc.sawText = true
	if onChunk != nil {
		if err := onChunk(chunk.Text); err != nil {
			return fmt.Errorf("%w: %w", errStreamListener, err)
		}
	}
	return nil
}

// resetIdleTimer restarts the stall timer for another timeout. A tick that has already
// fired is drained first, so the next select sees the new deadline rather than a tick
// that was already spent.
func resetIdleTimer(idle *time.Timer, timeout time.Duration) {
	if !idle.Stop() {
		select {
		case <-idle.C:
		default:
		}
	}
	idle.Reset(timeout)
}

// finishStream is what a stream that closed cleanly amounts to: everything it produced,
// or an empty response when it produced nothing, which the caller retries rather than
// records as a turn.
func finishStream(acc *streamAccumulator, providerID string, started time.Time) (streamResult, error) {
	if strings.TrimSpace(acc.text.String()) == "" && len(acc.toolCalls) == 0 {
		return streamResult{
			ProviderID: providerID,
			ChunkCount: acc.chunkCount,
			Failure: &harness.GenerationFailure{
				Code:      harness.FailureEmptyResponse,
				Message:   "provider returned no text",
				ElapsedMS: time.Since(started).Milliseconds(),
			},
		}, nil
	}
	return acc.result(providerID), nil
}

// interruptedStream is a stream that ended early. One that had already produced text is
// kept intact, so the turn can be repaired rather than lost; one that had not is a hard
// failure, classified from the error that ended it.
func interruptedStream(acc *streamAccumulator, providerID string, started time.Time, err error) (streamResult, error) {
	result := streamResult{ProviderID: providerID, ChunkCount: acc.chunkCount}
	if acc.sawText {
		result.Text = acc.text.String()
		result.FinishReason = acc.finishReason
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

// generateRequest streams one request through the gm role, falling back to the
// configured fallback provider when the primary fails before producing any text.
func (o *TurnOrchestrator) generateRequest(ctx context.Context, req harness.GenerateRequest, onChunk func(string) error) (streamResult, error) {
	provider, err := o.router.GetProviderForRole(config.RoleGM)
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

	if fallback, ok := o.router.FallbackForRole(config.RoleGM); ok {
		fallbackResult, fallbackErr := o.stream(ctx, fallback, req, onChunk)
		if fallbackErr == nil && fallbackResult.Failure == nil {
			return fallbackResult, nil
		}
		switch {
		case fallbackResult.Failure != nil:
			primaryFailure.Attempts = append(primaryFailure.Attempts, harness.Attempt{
				Role: config.RoleGM, Provider: fallbackResult.ProviderID,
				Code: fallbackResult.Failure.Code, Detail: fallbackResult.Failure.Message,
			})
		case fallbackErr != nil:
			primaryFailure.Attempts = append(primaryFailure.Attempts, harness.Attempt{
				Role: config.RoleGM, Provider: fallback.ID(),
				Code: harness.ClassifyProviderError(fallbackErr), Detail: fallbackErr.Error(),
			})
		}
	}
	if len(primaryFailure.Attempts) == 0 {
		primaryFailure.Attempts = []harness.Attempt{{
			Role: config.RoleGM, Provider: result.ProviderID, Code: primaryFailure.Code, Detail: primaryFailure.Message,
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

	provider, err := o.router.GetProviderForRole(config.RoleGM)
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
					if err := onChunk(resp.Text); err != nil {
						return streamResult{}, fmt.Errorf("%w: %w", errStreamListener, err)
					}
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
			o.logger.Event("context.session_fallback", map[string]any{
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
				o.logger.Event("context.cache_fallback", map[string]any{
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
						if err := onChunk(text); err != nil {
							return streamResult{}, fmt.Errorf("%w: %w", errStreamListener, err)
						}
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
		o.logger.Event("tool.round", map[string]any{
			"round":               round,
			"offered":             offerTools,
			"conversation_tokens": conversationTokens(messages),
			"budget":              o.contextBudget(),
		})

		roundCtx, roundSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/engine").Start(ctx, "provider.generate",
			oteltrace.WithAttributes(
				attribute.String("localrpg.role", config.RoleGM),
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
				attribute.String("localrpg.role", config.RoleGM),
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
					Role:     config.RoleGM,
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
					attribute.String("localrpg.role", config.RoleGM),
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
					attribute.String("localrpg.role", config.RoleGM),
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
			o.logger.Event("tool.stray", map[string]any{"round": round, "calls": len(result.ToolCalls)})
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
			o.logger.Event("tool.prose_discarded", map[string]any{
				"round": round,
				"chars": len([]rune(result.Text)),
			})
		}

		messages = append(messages, harness.Message{Role: "assistant", ToolCalls: result.ToolCalls})
		for _, call := range result.ToolCalls {
			trace.LogEvent(ctx, o.logger, "tool.call", map[string]any{
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
					encoded, err := json.Marshal(resolved)
					if err != nil {
						messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: "error: failed to encode check result: " + err.Error()})
						continue
					}
					messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: string(encoded)})
					o.notifyTool(ToolActivity{Round: round, Name: call.Name, Status: "done", Summary: resolved.Outcome})
					continue
				case "propose_check":
					if o.mechanicsEngagement != config.EngagementAsk {
						messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: "error: propose_check is only available when mechanics are set to ask"})
						continue
					}
					req, parseErr := harness.ParseCheckRequest(call.Arguments)
					if parseErr != nil {
						messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: "error: " + parseErr.Error()})
						continue
					}
					result.PendingCheck = &harness.PendingCheck{Ref: call.ID, Request: *req, ProposedBy: harness.ProposedByGM}
					result.Provenance = provenance
					result.ToolCalls = nil
					return result, nil
				default:
					messages = append(messages, harness.Message{Role: "tool", ToolCallID: call.ID, Content: "error: unsupported turn tool " + call.Name})
					continue
				}
			}

			if !offerTools {
				o.logger.Event("tool.stray", map[string]any{"round": round, "call": call.Name})
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
			trace.LogEvent(ctx, o.logger, "tool.result", map[string]any{
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
	return runes / harness.RunesPerToken
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
