package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
)

func TestTurnDTOCarriesToolProvenance(t *testing.T) {
	dto := TurnDTO{
		TurnNumber: 1,
		ToolCalls:  []ToolCallDTO{{Name: "search_entities", ResultChars: 42}},
	}
	if len(dto.ToolCalls) != 1 || dto.ToolCalls[0].Name != "search_entities" {
		t.Errorf("ToolCalls = %+v", dto.ToolCalls)
	}
}

func TestTurnEventCarriesToolActivity(t *testing.T) {
	event := TurnEvent{Type: "tool", ToolName: "search_entities", ToolStatus: "done", ToolSummary: "3 matches"}
	if event.ToolName == "" || event.ToolStatus != "done" || event.ToolSummary == "" {
		t.Errorf("event = %+v", event)
	}
}

func TestToolActivityMapsToATurnEvent(t *testing.T) {
	event := toolEvent(engine.ToolActivity{Round: 1, Name: "get_entity", Status: "running"})
	if event.Type != "tool" || event.ToolName != "get_entity" || event.ToolStatus != "running" {
		t.Errorf("event = %+v", event)
	}
}
