package rules

import (
	"fmt"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type ActionResult struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Roll    *RollResult            `json:"roll,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
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
}

type DefaultHostBridge struct {
	store      *storage.Store
	directives []string
	logs       []string
}

func NewHostBridge(store *storage.Store) *DefaultHostBridge {
	return &DefaultHostBridge{
		store:      store,
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
