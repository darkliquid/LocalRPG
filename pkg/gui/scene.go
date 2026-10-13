// Scene and character imagery: generation on demand, the budget that bounds it, and the events that report it.

package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/pricing"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// configureSceneBudget gives a scene worker the campaign's image allowance, its
// approval policy, and the free procedural fallback, so a generation respects the
// budget in one place.
func (s *Service) configureSceneBudget(worker *engine.SceneWorker, gameID string, cfg *config.Config) {
	manifest, err := core.LoadGameManifest(filepath.Join(s.resolver.GameDir(gameID), "game.yaml"))
	if err != nil || manifest == nil {
		return
	}

	worker.SetBudget(engine.ImageBudgetFromManifest(manifest), func(next engine.ImageBudget) {
		s.saveImageBudget(gameID, next)
	})
	worker.SetApproval(engine.ImageApprovalMode(manifest), imageProviderMetered(cfg.Media.Image))
	worker.SetPrice(imagePrice(cfg))
	worker.SetProceduralFallback(media.NewProceduralArtClient())
	worker.SetOnSkipped(func(gID string, turnNumber int, reason string) {
		trace.OrNil(s.logger).Event("turn.image_skipped", map[string]any{
			"turn": turnNumber, "reason": reason,
		})
		s.broadcastSceneImagePending(gID, turnNumber, reason)
	})
	// ask mode on a metered provider withholds the image until the player agrees,
	// so it has to say so. Without this the scene is simply absent and nothing
	// reports why.
	worker.SetOnPending(func(gID string, turnNumber int, _ engine.SceneJob) {
		trace.OrNil(s.logger).Event("turn.image_pending", map[string]any{
			"turn": turnNumber, "reason": "awaiting_approval",
		})
		s.broadcastSceneImagePending(gID, turnNumber, "awaiting_approval")
	})
}

// saveImageBudget writes a campaign's image counters back, so the allowance
// survives a reload.
func (s *Service) saveImageBudget(gameID string, budget engine.ImageBudget) {
	path := filepath.Join(s.resolver.GameDir(gameID), "game.yaml")
	manifest, err := core.LoadGameManifest(path)
	if err != nil || manifest == nil {
		return
	}
	engine.SetImageBudget(manifest, budget)
	if err := core.SaveGameManifest(path, manifest); err != nil {
		trace.OrNil(s.logger).Event("turn.image_budget_save_failed", map[string]any{"error": err.Error()})
	}
}

// imageProviderMetered reports whether the configured image provider charges per
// image, so the approval policy can apply to it.
func imageProviderMetered(cfg config.ImageConfig) bool {
	key, ok := media.ImageKeyFor(cfg)
	if !ok {
		return false
	}
	reg, ok := provider.Lookup(string(key.Parent()))
	if !ok {
		return false
	}
	for _, feature := range reg.Descriptor.Features {
		if feature == provider.FeatureMetered {
			return true
		}
	}
	return false
}

// imagePrice reports the cost of one image from the ledger, when it can price the
// configured provider. An unpriced provider records images only.
func imagePrice(cfg *config.Config) func() (int64, bool) {
	key, ok := media.ImageKeyFor(cfg.Media.Image)
	if !ok {
		return nil
	}
	return func() (int64, bool) {
		price := pricing.Resolve(string(key), cfg.Media.Image.Model, cfg)
		if price == (pricing.Price{}) {
			return 0, false
		}
		return int64(price.PerRequest), true
	}
}

// broadcastSceneImagePending tells the clients that a turn's illustration was not
// generated automatically, so a viewer can ask for it.
func (s *Service) broadcastSceneImagePending(gameID string, turnNumber int, reason string) {
	s.portraitMu.Lock()
	var listeners []func(TurnEvent)
	if m := s.portraitListeners[gameID]; m != nil {
		for _, fn := range m {
			listeners = append(listeners, fn)
		}
	}
	s.portraitMu.Unlock()

	evt := TurnEvent{Type: "scene_image_pending", TurnNumber: turnNumber, Message: reason}
	for _, fn := range listeners {
		fn(evt)
	}
}

func (s *Service) broadcastSceneImageReady(gameID string, turnNumber int, relPath string) {
	s.portraitMu.Lock()
	var listeners []func(TurnEvent)
	if m := s.portraitListeners[gameID]; m != nil {
		for _, fn := range m {
			listeners = append(listeners, fn)
		}
	}
	s.portraitMu.Unlock()

	evt := TurnEvent{
		Type:       "scene_image",
		TurnNumber: turnNumber,
		ImageURL:   fmt.Sprintf("/api/game/%s/turn/%d/scene-image", gameID, turnNumber),
	}
	for _, fn := range listeners {
		fn(evt)
	}
}

func parsePortraitVersionFromPath(relPath string) int {
	base := filepath.Base(relPath)
	idx := strings.LastIndex(base, "-v")
	if idx == -1 {
		return 0
	}
	dot := strings.LastIndex(base, ".")
	if dot == -1 || dot <= idx+2 {
		return 0
	}
	vStr := base[idx+2 : dot]
	v, err := strconv.Atoi(vStr)
	if err != nil {
		return 0
	}
	return v
}

// GenerateTurnSceneImage enqueues a scene image for a turn on demand, regardless
// of the trigger policy, so a player can illustrate a beat the policy skipped.
func (s *Service) GenerateTurnSceneImage(ctx context.Context, gameID string, turnNumber int) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}
	cfg := s.configMgr.Get()
	if cfg.Media.Image.Type == "" || cfg.Media.Image.Type == "disabled" {
		return fmt.Errorf("image generation is disabled")
	}
	imgClient, err := imageClientFactory(cfg.Media.Image, cfg.Providers.Gemini.APIKey)
	if err != nil || imgClient == nil {
		return fmt.Errorf("no image provider is configured")
	}
	turn, err := s.findTurn(gameID, turnNumber)
	if err != nil {
		return err
	}

	store := s.storeOrNil(gameID)
	var locEntity *entity.Entity
	if turn.Location != "" && store != nil {
		locEntity, _ = store.GetEntity(turn.Location)
	}
	cue := turn.SceneBreakCue
	if cue == "" {
		cue = engine.ExtractSceneCue(turn.Narration)
	}
	sceneCtx := engine.ScenePromptContext{
		Cue:       cue,
		Narration: turn.Narration,
		Action:    turn.Input,
		Location:  turn.Location,
		Style:     s.worldArtStyle(gameID),
		Cast:      engine.SceneCast(store, turn),
	}
	if locEntity != nil {
		sceneCtx.Location = locEntity.Name
		sceneCtx.Appearance = locEntity.Appearance
		sceneCtx.CurrentAppearance = locEntity.CurrentAppearance
	}
	if len(turn.Checks) > 0 {
		sceneCtx.Outcome = turn.Checks[0].Outcome
	}
	prompt := engine.BuildScenePrompt(sceneCtx)

	worker := engine.NewSceneWorker(s.resolver, imgClient)
	worker.SetOnReady(func(gID string, n int, relPath string) {
		s.broadcastSceneImageReady(gID, n, relPath)
	})
	worker.Enqueue(gameID, turnNumber, prompt)
	return nil
}

// GetTurnSceneImage returns the generated scene illustration for a specific turn.
func (s *Service) GetTurnSceneImage(ctx context.Context, gameID string, turnNumber int) ([]byte, string, error) {
	scenesDir := filepath.Join(s.resolver.GameDir(gameID), "assets", "scenes")
	for _, ext := range []string{".png", ".webp", ".jpg", ".jpeg", ".svg"} {
		p := filepath.Join(scenesDir, fmt.Sprintf("turn-%d%s", turnNumber, ext))
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return data, imageContentType(data), nil
		}
	}
	return nil, "", fmt.Errorf("scene image for turn %d not found", turnNumber)
}

// sceneArtResolver builds the one scene-art resolver for a campaign: the image client
// with the shared key, the world's art style, and the campaign's cache. The route that
// serves scene art and an export that carries it both use it, so a bundle shows the
// images the app already has rather than generating its own.
func (s *Service) sceneArtResolver(gameID string) *media.ArtStore {
	client, sceneCfg, err := s.imageRegistry().ForPurpose(config.PurposeScene)
	if err != nil {
		return nil
	}
	params := sceneCfg.Type + ":" + sceneCfg.Model
	layerCurrent := s.configMgr.Get().LocationArtAppearance() != config.LocationArtAppearanceOriginal
	return media.NewArtStore(client, media.NewContentCache(s.resolver.CacheDir()), s.worldArtStyle(gameID), params, layerCurrent)
}
