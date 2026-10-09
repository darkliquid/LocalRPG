package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

// ScenePromptContext is everything a scene prompt is composed from, so the image
// reflects what happened rather than only where the player is.
type ScenePromptContext struct {
	Cue        string   // the extractor cue or rule-break paragraph, if any
	Narration  string   // the turn's narration, for an excerpt
	Action     string   // the player's raw action
	Entities   []string // present characters, by display name
	Location   string   // the location's name
	Appearance string   // the location's authored appearance, if any
	Style      string   // the world art style
	Outcome    string   // the resolved check's outcome, if any
	// Scene is the stable look of the place, so two turns in one scene share a
	// palette and lighting rather than drifting apart.
	Scene media.SceneStyle
}

const (
	sceneNarrationCap = 200
	sceneActionCap    = 160
	sceneEntityCap    = 4
	// sceneSuffix is the fixed quality suffix every scene prompt ends with.
	sceneSuffix = "cinematic scene illustration, high quality, atmospheric lighting, detailed environment, no text, no borders"
)

// BuildScenePrompt composes the generation prompt for a turn scene illustration
// from the turn's context, each part bounded so the provider is not asked to
// reconcile a page of prose.
func BuildScenePrompt(ctx ScenePromptContext) string {
	parts := make([]string, 0, 8)

	// Subject: the cue when present, else a short narration excerpt.
	subject := strings.TrimSpace(ctx.Cue)
	if subject == "" {
		subject = sceneExcerpt(ctx.Narration, sceneNarrationCap)
	}
	if subject != "" {
		parts = append(parts, subject)
	}

	if action := sceneExcerpt(ctx.Action, sceneActionCap); action != "" {
		parts = append(parts, "action: "+action)
	}

	if len(ctx.Entities) > 0 {
		entities := ctx.Entities
		if len(entities) > sceneEntityCap {
			entities = entities[:sceneEntityCap]
		}
		parts = append(parts, "characters: "+strings.Join(entities, ", "))
	}

	if location := strings.TrimSpace(ctx.Location); location != "" {
		parts = append(parts, "location: "+location)
	}
	if appearance := strings.TrimSpace(ctx.Appearance); appearance != "" {
		parts = append(parts, appearance)
	}

	// The scene's stable look is named explicitly, so a provider renders the same
	// palette and lighting for every turn in one place.
	if clause := ctx.Scene.Clause(); clause != "" {
		parts = append(parts, clause)
	}

	if style := strings.TrimSpace(ctx.Style); style != "" {
		parts = append(parts, style)
	}
	if tone := outcomeToneWords(ctx.Outcome); tone != "" {
		parts = append(parts, tone)
	}

	parts = append(parts, sceneSuffix)
	return strings.Join(parts, ", ")
}

// ScenePromptPrefix is the stable part of a scene's prompt: the place, its
// authored appearance, its palette and lighting, and the world style. Two turns in
// one scene share it, which is the consistency a provider without image
// conditioning gets.
func ScenePromptPrefix(ctx ScenePromptContext) string {
	parts := make([]string, 0, 4)
	if location := strings.TrimSpace(ctx.Location); location != "" {
		parts = append(parts, "location: "+location)
	}
	if appearance := strings.TrimSpace(ctx.Appearance); appearance != "" {
		parts = append(parts, appearance)
	}
	if clause := ctx.Scene.Clause(); clause != "" {
		parts = append(parts, clause)
	}
	if style := strings.TrimSpace(ctx.Style); style != "" {
		parts = append(parts, style)
	}
	return strings.Join(parts, ", ")
}

// BuildScenePromptFor is the previous three-argument builder, kept for callers
// that only have a cue, a location, and a style.
func BuildScenePromptFor(visualCue string, location *entity.Entity, worldStyle string) string {
	ctx := ScenePromptContext{Cue: visualCue, Style: worldStyle}
	if location != nil {
		ctx.Location = location.Name
		ctx.Appearance = location.Appearance
	}
	return BuildScenePrompt(ctx)
}

// sceneExcerpt returns the first sentence of text, capped at limit characters.
func sceneExcerpt(text string, limit int) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if idx := strings.IndexAny(trimmed, ".!?"); idx >= 0 && idx < limit {
		trimmed = trimmed[:idx+1]
	}
	if len(trimmed) > limit {
		trimmed = strings.TrimSpace(trimmed[:limit])
	}
	return trimmed
}

// outcomeToneWords maps the common outcome families to short prompt tone words,
// and returns "" for anything else so a custom vocabulary adds no wrong mood.
func outcomeToneWords(outcome string) string {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "strong", "success", "pass", "critical", "hit":
		return "triumphant, bright"
	case "weak", "partial", "mixed", "success_with_cost":
		return "tense, uncertain"
	case "miss", "fail", "failure":
		return "ominous, shadowed"
	}
	return ""
}

// presentEntityNames resolves the display names of the entities a turn involved,
// so the scene can show them. It caps the list so the prompt stays short.
func (o *TurnOrchestrator) presentEntityNames(turn *Turn) []string {
	seen := make(map[string]bool, len(turn.Entities)+len(turn.Segments))
	names := make([]string, 0, sceneEntityCap)
	add := func(id string) {
		if id == "" || seen[id] || len(names) >= sceneEntityCap {
			return
		}
		seen[id] = true
		name := id
		if o.store != nil {
			if ent, err := o.store.GetEntity(id); err == nil && ent != nil && ent.Name != "" {
				name = ent.Name
			}
		}
		names = append(names, name)
	}
	for _, mention := range turn.Entities {
		add(mention.ID)
	}
	for _, segment := range turn.Segments {
		add(segment.SpeakerID)
	}
	return names
}

// ExtractSceneCue extracts a concise visual cue from narration text following a scene break delimiter.
func ExtractSceneCue(narration string) string {
	lines := strings.Split(narration, "\n")
	foundRule := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !foundRule {
			if trimmed == "---" || trimmed == "***" || trimmed == "___" {
				foundRule = true
			}
			continue
		}
		if trimmed != "" {
			if len(trimmed) > 200 {
				return trimmed[:200]
			}
			return trimmed
		}
	}
	// Fallback to first non-empty line
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && trimmed != "---" && trimmed != "***" && trimmed != "___" {
			if len(trimmed) > 200 {
				return trimmed[:200]
			}
			return trimmed
		}
	}
	return ""
}

// SceneGenerator abstracts image generation for scene illustrations.
type SceneGenerator interface {
	GenerateImage(ctx context.Context, prompt string) ([]byte, error)
}

// SceneJob is one queued illustration: the prompt, and the previous image of the
// same scene when one exists and the provider can condition on it.
type SceneJob struct {
	Prompt    string
	Reference []byte
}

// SceneWorker manages asynchronous queued scene illustration generation.
type SceneWorker struct {
	mu         sync.Mutex
	resolver   *core.PathResolver
	generator  SceneGenerator
	procedural SceneGenerator
	inFlight   map[string]bool
	onReady    func(gameID string, turnNumber int, relPath string)

	// budget bounds this worker's generations, and save persists every change so
	// the allowance survives a reload.
	budget ImageBudget
	save   func(ImageBudget)
	// price reports the cost of one image, when the ledger can price it.
	price func() (int64, bool)
	// metered marks a provider that charges per image, and approval is the
	// campaign's policy for one: auto generates, ask waits for the player.
	metered  bool
	approval string

	onSkipped func(gameID string, turnNumber int, reason string)
	onPending func(gameID string, turnNumber int, job SceneJob)
}

// NewSceneWorker creates a new SceneWorker.
func NewSceneWorker(resolver *core.PathResolver, gen SceneGenerator) *SceneWorker {
	return &SceneWorker{
		resolver:  resolver,
		generator: gen,
		inFlight:  make(map[string]bool),
	}
}

// SetBudget bounds this worker's generations. The worker checks it before calling
// the provider, charges it on success, and reports every change through save, so
// the allowance survives a reload.
func (w *SceneWorker) SetBudget(budget ImageBudget, save func(ImageBudget)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.budget = budget
	w.save = save
}

// SetProceduralFallback draws a scene when the budget is spent, so a beat is never
// imageless because of a limit. The fallback is free and is not charged.
func (w *SceneWorker) SetProceduralFallback(gen SceneGenerator) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.procedural = gen
}

// SetPrice reports the cost of one image, when the ledger can price it. An
// unpriced provider records images only.
func (w *SceneWorker) SetPrice(price func() (int64, bool)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.price = price
}

// SetApproval sets the campaign's image approval policy and whether the provider
// is metered. With ask on a metered provider a generation waits for the player.
func (w *SceneWorker) SetApproval(mode string, metered bool) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.approval = mode
	w.metered = metered
}

// SetOnSkipped reports a generation that the budget or the approval policy did
// not allow, so a caller can trace it.
func (w *SceneWorker) SetOnSkipped(fn func(gameID string, turnNumber int, reason string)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onSkipped = fn
}

// SetOnPending reports a generation awaiting the player's approval.
func (w *SceneWorker) SetOnPending(fn func(gameID string, turnNumber int, job SceneJob)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onPending = fn
}

// Budget reports the budget as it stands, with the charges this worker has added.
func (w *SceneWorker) Budget() ImageBudget {
	if w == nil {
		return ImageBudget{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.budget
}

// SetOnReady registers a callback invoked when a scene image has been generated and saved.
func (w *SceneWorker) SetOnReady(fn func(gameID string, turnNumber int, relPath string)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onReady = fn
}

// Enqueue asynchronously triggers scene image generation for a turn if not already in flight.
func (w *SceneWorker) Enqueue(gameID string, turnNumber int, prompt string) {
	w.EnqueueScene(gameID, turnNumber, SceneJob{Prompt: prompt})
}

// EnqueueScene enqueues a scene generation with an optional reference image, so a
// provider that can condition on one keeps the scene's look.
func (w *SceneWorker) EnqueueScene(gameID string, turnNumber int, job SceneJob) {
	if w == nil || w.generator == nil || gameID == "" || turnNumber <= 0 || strings.TrimSpace(job.Prompt) == "" {
		return
	}

	key := fmt.Sprintf("%s:%d", gameID, turnNumber)
	w.mu.Lock()
	if w.inFlight[key] {
		w.mu.Unlock()
		return
	}
	w.inFlight[key] = true
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.inFlight, key)
			w.mu.Unlock()
		}()

		w.draw(context.Background(), gameID, turnNumber, job)
	}()
}

// scenePlan decides how a job is drawn: by the provider and charged, by the
// procedural fallback because the budget is spent, or not at all because the
// player has not approved it.
type scenePlan struct {
	gen     SceneGenerator
	charge  bool
	pending bool
	reason  string
}

// plan applies the budget and the approval policy in one place, so every path
// that generates a scene respects them.
func (w *SceneWorker) plan() scenePlan {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.budget.Exhausted() {
		if w.procedural != nil {
			return scenePlan{gen: w.procedural}
		}
		return scenePlan{reason: "budget_exhausted"}
	}
	if w.approval == ImageApprovalAsk && w.metered {
		return scenePlan{pending: true}
	}
	return scenePlan{gen: w.generator, charge: true}
}

// draw runs one queued job through the plan, reporting what it did.
func (w *SceneWorker) draw(ctx context.Context, gameID string, turnNumber int, job SceneJob) {
	plan := w.plan()
	switch {
	case plan.pending:
		w.notifyPending(gameID, turnNumber, job)
	case plan.gen == nil:
		w.notifySkipped(gameID, turnNumber, plan.reason)
	default:
		if _, err := w.writeScene(ctx, gameID, turnNumber, job, plan.gen); err != nil {
			return
		}
		if plan.charge {
			w.charge()
		}
	}
}

// charge records one generated image and persists the budget.
func (w *SceneWorker) charge() {
	w.mu.Lock()
	var cost int64
	if w.price != nil {
		if micros, ok := w.price(); ok {
			cost = micros
		}
	}
	w.budget.Charge(cost)
	budget, save := w.budget, w.save
	w.mu.Unlock()
	if save != nil {
		save(budget)
	}
}

func (w *SceneWorker) notifySkipped(gameID string, turnNumber int, reason string) {
	w.mu.Lock()
	fn := w.onSkipped
	w.mu.Unlock()
	if fn != nil {
		fn(gameID, turnNumber, reason)
	}
}

func (w *SceneWorker) notifyPending(gameID string, turnNumber int, job SceneJob) {
	w.mu.Lock()
	fn := w.onPending
	w.mu.Unlock()
	if fn != nil {
		fn(gameID, turnNumber, job)
	}
}

func (w *SceneWorker) writeScene(ctx context.Context, gameID string, turnNumber int, job SceneJob, gen SceneGenerator) (string, error) {
	imgBytes, err := w.generate(ctx, gen, job)
	if err != nil {
		return "", fmt.Errorf("generate scene image: %w", err)
	}
	if len(imgBytes) == 0 {
		return "", fmt.Errorf("scene generator returned no image")
	}

	ext := media.ArtExtension(imgBytes)
	if ext == "" {
		ext = ".png"
	}

	filename := fmt.Sprintf("turn-%d%s", turnNumber, ext)
	relPath := filepath.Join("assets", "scenes", filename)
	fullPath := filepath.Join(w.resolver.GameDir(gameID), relPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("create scene dir: %w", err)
	}
	if err := os.WriteFile(fullPath, imgBytes, 0644); err != nil {
		return "", fmt.Errorf("write scene image: %w", err)
	}

	w.mu.Lock()
	cb := w.onReady
	w.mu.Unlock()
	if cb != nil {
		cb(gameID, turnNumber, relPath)
	}
	return relPath, nil
}

// generate draws a scene, conditioning on the reference image when the generator
// can take one. A provider without conditioning gets the stable prompt instead.
func (w *SceneWorker) generate(ctx context.Context, gen SceneGenerator, job SceneJob) ([]byte, error) {
	if gen == nil {
		return nil, fmt.Errorf("no scene generator")
	}
	if len(job.Reference) > 0 {
		if conditioner, ok := gen.(media.SceneConditioner); ok {
			img, err := conditioner.GenerateSceneWithReference(ctx, media.SceneRequestFromPrompt(job.Prompt), job.Reference)
			if err == nil && len(img) > 0 {
				return img, nil
			}
			// A conditioner that fails falls back to the plain path rather than
			// losing the beat's image.
		}
	}
	return gen.GenerateImage(ctx, job.Prompt)
}
