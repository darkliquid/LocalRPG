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
