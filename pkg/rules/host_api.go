package rules

import (
	"fmt"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type ActionResult struct {
	Success bool                   `json:"success"`
	Outcome string                 `json:"outcome,omitempty"`
	Message string                 `json:"message"`
	Roll    *RollResult            `json:"roll,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// EntityWriter persists an entity to its Markdown note and the index. It is how
// the host bridge writes without inventing a second write path beside the engine's.
type EntityWriter interface {
	SaveEntity(ent *entity.Entity) error
}

type GameHostAPI interface {
	Roll(notation string) (*RollResult, error)
	GetStat(entityID string, path string) (interface{}, error)
	SetStat(entityID string, path string, value interface{}) error
	GetEntity(entityID string) (*entity.Entity, error)
	InjectGMDirection(directive string)
	GetDirectives() []string
	Log(message string)
	GetLogs() []string
	SetLocation(locationID string) error
	GetLocation() (string, error)
	// ListStats, ListSkills, and CheckConventions expose the system's declarative
	// mechanics schema to scripts, which may derive stats and register resolvers.
	ListStats() []core.StatSpec
	ListSkills() []core.SkillSpec
	CheckConventions() core.CheckConventions
}

type DefaultHostBridge struct {
	store      *storage.Store
	writer     EntityWriter
	playerID   string
	directives []string
	logs       []string
	manifest   *core.SystemManifest
}

// NewHostBridge builds the host API a system's scripts are given. writer persists
// entities through the engine's own path; a nil writer falls back to the store,
// which is only correct for tests that do not care about the Markdown note.
func NewHostBridge(store *storage.Store, writer EntityWriter, playerID string) *DefaultHostBridge {
	return &DefaultHostBridge{
		store:      store,
		writer:     writer,
		playerID:   playerID,
		directives: make([]string, 0),
		logs:       make([]string, 0),
	}
}

func (h *DefaultHostBridge) Roll(notation string) (*RollResult, error) {
	return EvaluateRoll(notation)
}

func (h *DefaultHostBridge) GetStat(entityID string, path string) (interface{}, error) {
	ent, err := h.store.GetEntity(entityID)
	if err != nil {
		return nil, fmt.Errorf("entity %q not found: %w", entityID, err)
	}
	if ent.State == nil {
		return nil, nil
	}
	val, ok := ent.State.Get(path)
	if !ok {
		return nil, nil
	}
	return val, nil
}

func (h *DefaultHostBridge) SetStat(entityID string, path string, value interface{}) error {
	ent, err := h.store.GetEntity(entityID)
	if err != nil {
		return fmt.Errorf("entity %q not found: %w", entityID, err)
	}
	if ent.State == nil {
		ent.InitState(make(map[string]interface{}))
	}
	if err := ent.State.Set(path, value); err != nil {
		return fmt.Errorf("set stat %q: %w", path, err)
	}
	return h.persist(ent)
}

// SetLocation moves the player, writing through the same path the engine uses.
func (h *DefaultHostBridge) SetLocation(locationID string) error {
	if h.playerID == "" {
		return fmt.Errorf("set location: no player configured")
	}

	target, err := h.store.GetEntity(locationID)
	if err != nil || target == nil {
		return fmt.Errorf("set location: unknown location %q", locationID)
	}
	if target.Type != "location" {
		return fmt.Errorf("set location: %q is a %s, not a location", locationID, target.Type)
	}

	player, err := h.store.GetEntity(h.playerID)
	if err != nil || player == nil {
		return fmt.Errorf("set location: player %q not found", h.playerID)
	}

	player.Location = "[[" + locationID + "]]"
	return h.persist(player)
}

// GetLocation returns the player's current location ID.
func (h *DefaultHostBridge) GetLocation() (string, error) {
	if h.playerID == "" {
		return "", fmt.Errorf("get location: no player configured")
	}

	player, err := h.store.GetEntity(h.playerID)
	if err != nil || player == nil {
		return "", fmt.Errorf("get location: player %q not found", h.playerID)
	}
	if player.Location == "" {
		return "", nil
	}
	return entity.Slugify(entity.WikilinkTarget(player.Location)), nil
}

// persist writes an entity through the writer when one is configured. Writing
// through the store alone would never reach the Markdown note, so the next
// file-driven sync could revert it.
func (h *DefaultHostBridge) persist(ent *entity.Entity) error {
	if h.writer != nil {
		return h.writer.SaveEntity(ent)
	}
	return h.store.SaveEntity(ent)
}

func (h *DefaultHostBridge) GetEntity(entityID string) (*entity.Entity, error) {
	return h.store.GetEntity(entityID)
}

func (h *DefaultHostBridge) InjectGMDirection(directive string) {
	h.directives = append(h.directives, directive)
}

func (h *DefaultHostBridge) GetDirectives() []string {
	return h.directives
}

func (h *DefaultHostBridge) Log(message string) {
	h.logs = append(h.logs, message)
}

func (h *DefaultHostBridge) GetLogs() []string {
	return h.logs
}

// PlayerID is the entity ID the bridge treats as the player, so script
// bindings that award or spend the advancement currency know who to credit.
func (h *DefaultHostBridge) PlayerID() string { return h.playerID }

// SetManifest gives the bridge the system's declarative mechanics schema, so
// scripts can read it through ListStats/ListSkills/CheckConventions.
func (h *DefaultHostBridge) SetManifest(manifest *core.SystemManifest) {
	h.manifest = manifest
}

// ListStats returns the declared stats, or nothing when no schema is set.
func (h *DefaultHostBridge) ListStats() []core.StatSpec {
	if h.manifest == nil || h.manifest.Mechanics == nil {
		return nil
	}
	return h.manifest.Mechanics.Stats
}

// ListSkills returns the declared skills.
func (h *DefaultHostBridge) ListSkills() []core.SkillSpec {
	if h.manifest == nil || h.manifest.Mechanics == nil {
		return nil
	}
	return h.manifest.Mechanics.Skills
}

// CheckConventions returns the declared check conventions.
func (h *DefaultHostBridge) CheckConventions() core.CheckConventions {
	if h.manifest == nil || h.manifest.Mechanics == nil {
		return core.CheckConventions{}
	}
	return h.manifest.Mechanics.Checks
}
