package gui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// ErrAdvancementRefused marks a spend the campaign's rules will not allow: an
// unaffordable cost, an unmet requirement, or a closed gate. The route maps it
// to a 400 rather than a server fault.
var ErrAdvancementRefused = errors.New("advancement refused")

// computeAdvancement builds the game-state summary from a campaign's system
// schema and the player's own state. A system with no advancement block yields
// nil, so a schema-light campaign shows nothing.
func (s *Service) computeAdvancement(gameID string, systemManifest *core.SystemManifest) *AdvancementDTO {
	spec := advancementSpec(systemManifest)
	if spec == nil {
		return nil
	}
	store := s.storeOrNil(gameID)
	if store == nil {
		return nil
	}
	playerID, player := s.advancementPlayer(gameID, store)
	if player == nil {
		return nil
	}
	gameManifest, err := core.LoadGameManifest(filepath.Join(s.resolver.GameDir(gameID), "game.yaml"))
	if err != nil {
		return nil
	}

	value := advancementValue(player, spec.Currency.Stat)
	gateOpen := s.advancementGateOpen(gameManifest, spec)
	dto := &AdvancementDTO{
		Currency: spec.Currency.Stat,
		Label:    spec.Currency.Label,
		Value:    value,
		Mode:     spec.Mode,
		Pending:  advancementValue(player, engine.AdvancementPendingStat) > 0,
	}
	if spec.Mode == "track" && spec.TrackSize > 0 {
		dto.Track = &TrackDTO{Filled: value, Size: spec.TrackSize}
	}
	for _, unlock := range spec.Unlocks {
		dto.Unlocks = append(dto.Unlocks, UnlockDTO{
			ID:          unlock.ID,
			Label:       unlock.Label,
			Description: unlock.Description,
			Cost:        unlock.Cost,
			Affordable:  rules.Affordable(value, unlock),
			RequiresMet: rules.RequirementsMet(unlock, player.Tags, player.Tags),
			GateOpen:    gateOpen,
		})
	}
	_ = playerID
	return dto
}

// AdvanceUnlock spends the currency on one unlock, applying its effects, then
// returns the refreshed summary.
func (s *Service) AdvanceUnlock(ctx context.Context, gameID, unlockID string) (*AdvancementDTO, error) {
	gameDir := s.resolver.GameDir(gameID)
	gameManifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read game manifest: %w", err)
	}
	systemManifest, err := core.LoadSystemManifest(filepath.Join(s.resolver.SystemDir(gameManifest.SystemID), "system.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read system manifest: %w", err)
	}
	spec := advancementSpec(systemManifest)
	if spec == nil {
		return nil, fmt.Errorf("%w: this campaign has no advancement", ErrAdvancementRefused)
	}
	unlock, ok := findUnlock(spec, unlockID)
	if !ok {
		return nil, fmt.Errorf("%w: unknown unlock %q", ErrAdvancementRefused, unlockID)
	}

	store := s.storeOrNil(gameID)
	if store == nil {
		return nil, fmt.Errorf("%w: campaign index is unavailable", ErrAdvancementRefused)
	}
	playerID, player := s.advancementPlayer(gameID, store)
	if player == nil {
		return nil, fmt.Errorf("%w: campaign has no player note", ErrAdvancementRefused)
	}

	value := advancementValue(player, spec.Currency.Stat)
	gateOpen := s.advancementGateOpen(gameManifest, spec)
	if !rules.Affordable(value, unlock) {
		return nil, fmt.Errorf("%w: not enough %s", ErrAdvancementRefused, spec.Currency.Stat)
	}
	if !rules.RequirementsMet(unlock, player.Tags, player.Tags) {
		return nil, fmt.Errorf("%w: requirements for %q are unmet", ErrAdvancementRefused, unlock.ID)
	}
	if !gateOpen {
		return nil, fmt.Errorf("%w: advancement is gated by %s", ErrAdvancementRefused, spec.Gate)
	}

	timeline := engine.NewTimeline(s.resolver, store, engine.NewHistoryLogger(filepath.Join(gameDir, "history.jsonl")), gameID)
	bridge := rules.NewHostBridge(store, timeline, playerID)
	if err := rules.ApplyUnlock(bridge, spec, unlock, playerID, player.Tags, gateOpen); err != nil {
		return nil, fmt.Errorf("apply unlock: %w", err)
	}

	if max, err := store.MaxTurnNumber(); err == nil {
		label := unlock.Label
		if label == "" {
			label = unlock.ID
		}
		_, _ = store.SaveMemory(&entity.Memory{
			Turn:       max,
			Kind:       "advancement",
			EntityRefs: []string{playerID},
			Text:       fmt.Sprintf("Spent %d %s on %s.", unlock.Cost, spec.Currency.Stat, label),
			Importance: 4,
			Tags:       []string{"advancement"},
			Source:     entity.SourceEngine,
		})
	}

	return s.computeAdvancement(gameID, systemManifest), nil
}

// advancementPlayer resolves the player entity from the index, which carries the
// currency, tags, and pending flag the summary reads.
func (s *Service) advancementPlayer(gameID string, store *storage.Store) (string, *entity.Entity) {
	gameManifest, err := core.LoadGameManifest(filepath.Join(s.resolver.GameDir(gameID), "game.yaml"))
	if err != nil {
		return "", nil
	}
	playerID, err := engine.ResolvePlayerID(store, gameManifest)
	if err != nil || playerID == "" {
		return "", nil
	}
	player, err := store.GetEntity(playerID)
	if err != nil || player == nil {
		return "", nil
	}
	return playerID, player
}

// advancementGateOpen reports whether a gated system's gate is currently open.
// The only declared gate is "downtime", read from the campaign's settings.
func (s *Service) advancementGateOpen(gameManifest *core.GameManifest, spec *core.AdvancementSpec) bool {
	if spec.Gate == "" {
		return true
	}
	if spec.Gate != "downtime" || gameManifest == nil || gameManifest.Settings == nil {
		return false
	}
	open, _ := gameManifest.Settings["downtime"].(bool)
	return open
}

// advancementSpec returns a system's advancement schema, or nil when it has none.
func advancementSpec(systemManifest *core.SystemManifest) *core.AdvancementSpec {
	if systemManifest == nil || systemManifest.Mechanics == nil {
		return nil
	}
	return systemManifest.Mechanics.Advancement
}

func findUnlock(spec *core.AdvancementSpec, id string) (core.UnlockSpec, bool) {
	for _, unlock := range spec.Unlocks {
		if unlock.ID == id {
			return unlock, true
		}
	}
	return core.UnlockSpec{}, false
}

func advancementValue(player *entity.Entity, stat string) int {
	if player == nil || player.State == nil || stat == "" {
		return 0
	}
	raw, ok := player.State.Get(stat)
	if !ok {
		return 0
	}
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	}
	return 0
}
