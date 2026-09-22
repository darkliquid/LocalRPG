package config

import (
	"testing"
	"time"
)

func TestTurnAndChunkTimeoutsHaveDefaults(t *testing.T) {
	empty := &Config{}

	if got := empty.TurnTimeout(); got != 300*time.Second {
		t.Errorf("TurnTimeout() = %v, want 300s for an omitted setting", got)
	}
	if got := empty.ChunkTimeout(); got != 60*time.Second {
		t.Errorf("ChunkTimeout() = %v, want 60s for an omitted setting", got)
	}

	configured := &Config{Agents: AgentsConfig{TurnTimeoutSeconds: 45, ChunkTimeoutSeconds: 5}}
	if got := configured.TurnTimeout(); got != 45*time.Second {
		t.Errorf("TurnTimeout() = %v, want 45s", got)
	}
	if got := configured.ChunkTimeout(); got != 5*time.Second {
		t.Errorf("ChunkTimeout() = %v, want 5s", got)
	}
}

func TestContextLimitsHaveDefaults(t *testing.T) {
	empty := &Config{}

	if got := empty.RecentTurns(); got != 6 {
		t.Errorf("RecentTurns() = %d, want 6 for an omitted setting", got)
	}
	if got := empty.RecentTurnChars(); got != 1200 {
		t.Errorf("RecentTurnChars() = %d, want 1200 for an omitted setting", got)
	}
	// An omitted budget means unbounded, so existing configuration is unchanged.
	if got := empty.ContextBudget(); got != 0 {
		t.Errorf("ContextBudget() = %d, want 0 (unbounded)", got)
	}

	configured := &Config{Agents: AgentsConfig{
		ContextTokenBudget:  32000,
		RecentTurnWindow:    20,
		RecentTurnCharLimit: 4000,
	}}
	if got := configured.ContextBudget(); got != 32000 {
		t.Errorf("ContextBudget() = %d, want 32000", got)
	}
	if got := configured.RecentTurns(); got != 20 {
		t.Errorf("RecentTurns() = %d, want 20", got)
	}
	if got := configured.RecentTurnChars(); got != 4000 {
		t.Errorf("RecentTurnChars() = %d, want 4000", got)
	}
}
