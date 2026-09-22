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

func TestTraceSettingsHaveDefaults(t *testing.T) {
	empty := &Config{}

	if got := empty.TraceLevel(); got != "off" {
		t.Errorf("TraceLevel() = %q, want off", got)
	}
	if got := empty.TracePayloadChars(); got != 20000 {
		t.Errorf("TracePayloadChars() = %d, want 20000", got)
	}
	// Generous on purpose: tracing is opt-in, so this bounds a debug session left
	// running rather than rationing normal play.
	if got := empty.TraceMaxBytes(); got != 268435456 {
		t.Errorf("TraceMaxBytes() = %d, want 268435456", got)
	}
	if got := empty.TraceMaxFiles(); got != 3 {
		t.Errorf("TraceMaxFiles() = %d, want 3", got)
	}
	if got := empty.TraceRotateCheck(); got != 200 {
		t.Errorf("TraceRotateCheck() = %d, want 200", got)
	}
	if got := empty.TraceChunkLimit(); got != 500 {
		t.Errorf("TraceChunkLimit() = %d, want 500", got)
	}

	configured := &Config{
		Preferences: PreferencesConfig{TraceLevel: "full"},
		Agents: AgentsConfig{
			TracePayloadChars: 5000,
			TraceMaxBytes:     1024,
			TraceMaxFiles:     1,
			TraceRotateCheck:  10,
			TraceChunkLimit:   20,
		},
	}
	if got := configured.TraceLevel(); got != "full" {
		t.Errorf("TraceLevel() = %q, want full", got)
	}
	if got := configured.TracePayloadChars(); got != 5000 {
		t.Errorf("TracePayloadChars() = %d, want 5000", got)
	}
	if got := configured.TraceMaxBytes(); got != 1024 {
		t.Errorf("TraceMaxBytes() = %d, want 1024", got)
	}
	if got := configured.TraceMaxFiles(); got != 1 {
		t.Errorf("TraceMaxFiles() = %d, want 1", got)
	}
	if got := configured.TraceRotateCheck(); got != 10 {
		t.Errorf("TraceRotateCheck() = %d, want 10", got)
	}
	if got := configured.TraceChunkLimit(); got != 20 {
		t.Errorf("TraceChunkLimit() = %d, want 20", got)
	}
}

func TestRecallSettingsHaveDefaults(t *testing.T) {
	empty := &Config{}

	if got := empty.SceneRecallTurns(); got != 4 {
		t.Errorf("SceneRecallTurns() = %d, want 4", got)
	}
	if got := empty.SceneRecallChars(); got != 800 {
		t.Errorf("SceneRecallChars() = %d, want 800", got)
	}
	if got := empty.RetrievalTurns(); got != 3 {
		t.Errorf("RetrievalTurns() = %d, want 3", got)
	}
	if got := empty.RetrievalChars(); got != 800 {
		t.Errorf("RetrievalChars() = %d, want 800", got)
	}
	if got := empty.RetrievalHalfLifeTurns(); got != 12 {
		t.Errorf("RetrievalHalfLifeTurns() = %d, want 12", got)
	}
}

func TestSummarySettingsHaveDefaults(t *testing.T) {
	empty := &Config{}

	// Zero means disabled, so the default must not be zero.
	if got := empty.SummaryEvery(); got != 0 {
		t.Errorf("SummaryEvery() on an empty config = %d, want 0 (disabled unless configured)", got)
	}
	if got := empty.SummaryCharLimit(); got != 2000 {
		t.Errorf("SummaryCharLimit() = %d, want 2000", got)
	}

	configured := &Config{Agents: AgentsConfig{SummaryEvery: 5, SummaryCharLimit: 500}}
	if got := configured.SummaryEvery(); got != 5 {
		t.Errorf("SummaryEvery() = %d, want 5", got)
	}
	if got := configured.SummaryCharLimit(); got != 500 {
		t.Errorf("SummaryCharLimit() = %d, want 500", got)
	}

	// The shipped defaults turn summarisation on at a cadence.
	if got := DefaultConfig().SummaryEvery(); got != 10 {
		t.Errorf("DefaultConfig().SummaryEvery() = %d, want 10", got)
	}
	if got := DefaultConfig().SummaryCharLimit(); got != 2000 {
		t.Errorf("DefaultConfig().SummaryCharLimit() = %d, want 2000", got)
	}
}
