package config

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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

func TestCompletionSettingsHaveDefaults(t *testing.T) {
	empty := &Config{}

	if got := empty.CompletionMode(); got != "auto" {
		t.Errorf("CompletionMode() = %q, want auto for an omitted setting", got)
	}
	if got := empty.CompletionAttempts(); got != 1 {
		t.Errorf("CompletionAttempts() = %d, want 1", got)
	}
	if got := empty.CompletionTailChars(); got != 1500 {
		t.Errorf("CompletionTailChars() = %d, want 1500", got)
	}
	if got := empty.CompletionMinChars(); got != 24 {
		t.Errorf("CompletionMinChars() = %d, want 24", got)
	}
	if got := empty.CompletionTimeout(); got != 45*time.Second {
		t.Errorf("CompletionTimeout() = %v, want 45s", got)
	}

	configured := &Config{Agents: AgentsConfig{Completion: CompletionConfig{
		Mode:               "trim",
		MaxAttempts:        3,
		TailChars:          800,
		MinIncompleteChars: 40,
		TimeoutSeconds:     10,
	}}}
	if got := configured.CompletionMode(); got != "trim" {
		t.Errorf("CompletionMode() = %q, want trim", got)
	}
	if got := configured.CompletionAttempts(); got != 3 {
		t.Errorf("CompletionAttempts() = %d, want 3", got)
	}
	if got := configured.CompletionTailChars(); got != 800 {
		t.Errorf("CompletionTailChars() = %d, want 800", got)
	}
	if got := configured.CompletionMinChars(); got != 40 {
		t.Errorf("CompletionMinChars() = %d, want 40", got)
	}
	if got := configured.CompletionTimeout(); got != 10*time.Second {
		t.Errorf("CompletionTimeout() = %v, want 10s", got)
	}

	// An unrecognised mode falls back to auto rather than silently disabling.
	unknown := &Config{Agents: AgentsConfig{Completion: CompletionConfig{Mode: "banana"}}}
	if got := unknown.CompletionMode(); got != "auto" {
		t.Errorf("CompletionMode() = %q, want auto for an unknown mode", got)
	}
}

func TestDefaultConfigCarriesCompletionKnobs(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Agents.Completion.Mode != "auto" {
		t.Errorf("default completion mode = %q, want auto", cfg.Agents.Completion.Mode)
	}
	role, ok := cfg.Agents.Roles[RoleCompletion]
	if !ok {
		t.Fatalf("default config has no %q role", RoleCompletion)
	}
	if role.Type != "inherit" || role.InheritFrom != RoleGM {
		t.Errorf("completion role = %+v, want inherit gm", role)
	}
}

func TestVoiceProfileOptionsAndMeteredRoundTrip(t *testing.T) {
	profile := VoiceProfile{
		ID:      "hushed",
		VoiceID: "bf_emma",
		Options: map[string]interface{}{"stability": 0.35, "model": "eleven_multilingual_v2"},
	}
	encoded, err := yaml.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	var decoded VoiceProfile
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal profile: %v", err)
	}
	if decoded.Options["stability"] != 0.35 || decoded.Options["model"] != "eleven_multilingual_v2" {
		t.Errorf("options did not round-trip: %v", decoded.Options)
	}

	off := false
	cfg := TTSConfig{Metered: &off}
	encoded, err = yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal tts config: %v", err)
	}
	var decodedCfg TTSConfig
	if err := yaml.Unmarshal(encoded, &decodedCfg); err != nil {
		t.Fatalf("unmarshal tts config: %v", err)
	}
	if decodedCfg.Metered == nil || *decodedCfg.Metered {
		t.Errorf("metered did not round-trip: %v", decodedCfg.Metered)
	}

	// A config that never set the pointer omits it entirely.
	plain, err := yaml.Marshal(TTSConfig{})
	if err != nil {
		t.Fatalf("marshal plain config: %v", err)
	}
	if strings.Contains(string(plain), "metered") {
		t.Errorf("an unset metered must be omitted, got %s", plain)
	}
}

func TestTTSConfigOptionsRoundTrip(t *testing.T) {
	cfg := TTSConfig{Options: map[string]interface{}{"stability": 0.4, "model": "eleven_multilingual_v2"}}
	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded TTSConfig
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Options["stability"] != 0.4 || decoded.Options["model"] != "eleven_multilingual_v2" {
		t.Errorf("options did not round-trip: %v", decoded.Options)
	}

	plain, err := yaml.Marshal(TTSConfig{})
	if err != nil {
		t.Fatalf("marshal plain: %v", err)
	}
	if strings.Contains(string(plain), "options") {
		t.Errorf("an unset options map must be omitted, got %s", plain)
	}
}

func TestToolCapabilityAndBounds(t *testing.T) {
	empty := &Config{}

	if got := empty.RoleSupportsTools("gm"); got != "auto" {
		t.Errorf("RoleSupportsTools = %q, want auto", got)
	}
	if got := empty.ToolRounds(); got != 0 {
		t.Errorf("ToolRounds = %d, want 0", got)
	}
	if got := empty.ToolResultChars(); got != 4000 {
		t.Errorf("ToolResultChars = %d, want 4000", got)
	}

	configured := &Config{Agents: AgentsConfig{
		Roles: map[string]AgentRoleConfig{"gm": {Type: "http", SupportsTools: "no"}},
	}}
	if got := configured.RoleSupportsTools("gm"); got != "no" {
		t.Errorf("RoleSupportsTools = %q, want the configured no", got)
	}

	unknown := &Config{Agents: AgentsConfig{Roles: map[string]AgentRoleConfig{"gm": {SupportsTools: "banana"}}}}
	if got := unknown.RoleSupportsTools("gm"); got != "auto" {
		t.Errorf("RoleSupportsTools = %q, want auto for an unknown value", got)
	}

	bounded := &Config{Agents: AgentsConfig{ToolRounds: 2, ToolResultChars: 500}}
	if got := bounded.ToolRounds(); got != 2 {
		t.Errorf("ToolRounds = %d, want 2", got)
	}
	if got := bounded.ToolResultChars(); got != 500 {
		t.Errorf("ToolResultChars = %d, want 500", got)
	}
}

func TestConfigParsesGeminiProviderAndRoleTunables(t *testing.T) {
	yamlStr := `
providers:
  gemini:
    api_key: "test-shared-gemini-key"
agents:
  default_role: "gm"
  roles:
    gm:
      type: "builtin"
      builtin_name: "gemini"
      model: "gemini-2.5-flash"
      thinking_budget: 0
      top_p: 0.95
      top_k: 40
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if cfg.Providers.Gemini.APIKey != "test-shared-gemini-key" {
		t.Errorf("expected shared api_key 'test-shared-gemini-key', got %q", cfg.Providers.Gemini.APIKey)
	}
	role := cfg.Agents.Roles["gm"]
	if role.ThinkingBudget == nil || *role.ThinkingBudget != 0 {
		t.Errorf("expected thinking_budget 0, got %v", role.ThinkingBudget)
	}
	if role.TopP == nil || *role.TopP != 0.95 {
		t.Errorf("expected top_p 0.95, got %v", role.TopP)
	}
	if role.TopK == nil || *role.TopK != 40 {
		t.Errorf("expected top_k 40, got %v", role.TopK)
	}
}

func TestConfigParsesImageTunables(t *testing.T) {
	yamlData := `
version: "1"
paths:
  systems: "./systems"
  worlds: "./worlds"
  games: "./games"
  cache: "./cache"
media:
  image:
    type: "gemini"
    model: "imagen-3.0-generate-002"
    aspect_ratio: "16:9"
    person_generation: "ALLOW_ADULT"
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}

	if cfg.Media.Image.AspectRatio != "16:9" {
		t.Errorf("expected aspect_ratio 16:9, got %q", cfg.Media.Image.AspectRatio)
	}
	if cfg.Media.Image.PersonGeneration != "ALLOW_ADULT" {
		t.Errorf("expected person_generation ALLOW_ADULT, got %q", cfg.Media.Image.PersonGeneration)
	}
}

func TestDefaultTelemetryIsDisabled(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Telemetry.Enabled {
		t.Fatal("telemetry must default to disabled")
	}
	if cfg.Telemetry.Endpoint != "localhost:4317" || !cfg.Telemetry.Traces {
		t.Fatalf("unexpected telemetry defaults: %+v", cfg.Telemetry)
	}
}

func TestConfigParsesEmbeddings(t *testing.T) {
	yamlData := `
version: "1"
embeddings:
  enabled: true
  provider: "openai"
  model: "text-embedding-3-small"
  dimensions: 1536
  batch_size: 32
  providers:
    openai:
      type: "http"
      url: "https://api.openai.com/v1"
      api_key: "test-openai-key"
      model: "text-embedding-3-small"
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}

	if !cfg.Embeddings.Enabled {
		t.Fatal("expected embeddings.enabled to be true")
	}
	if cfg.Embeddings.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", cfg.Embeddings.Provider)
	}
	if cfg.Embeddings.BatchSize != 32 {
		t.Errorf("expected batch_size 32, got %d", cfg.Embeddings.BatchSize)
	}
	pCfg, ok := cfg.Embeddings.Providers["openai"]
	if !ok {
		t.Fatal("expected openai provider in config")
	}
	if pCfg.APIKey != "test-openai-key" || pCfg.Type != "http" {
		t.Errorf("unexpected openai provider config: %+v", pCfg)
	}
}

func TestActionEchoDefaultsOn(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.ActionEcho() {
		t.Fatal("ActionEcho() should default to true")
	}
	off := false
	cfg.Agents.ActionEcho = &off
	if cfg.ActionEcho() {
		t.Fatal("ActionEcho() should honour an explicit false")
	}
	on := true
	cfg.Agents.ActionEcho = &on
	if !cfg.ActionEcho() {
		t.Fatal("ActionEcho() should honour an explicit true")
	}
}

func TestOpusBitrateDefaultsAndClamps(t *testing.T) {
	cfg := DefaultConfig()
	if got := cfg.OpusBitrate(); got != 32000 {
		t.Fatalf("OpusBitrate() = %d, want the default 32000", got)
	}
	cfg.Media.TTS.OpusBitrate = 48000
	if got := cfg.OpusBitrate(); got != 48000 {
		t.Fatalf("OpusBitrate() = %d, want 48000", got)
	}
	cfg.Media.TTS.OpusBitrate = 1
	if got := cfg.OpusBitrate(); got != 6000 {
		t.Fatalf("OpusBitrate() = %d, want the clamp floor 6000", got)
	}
	cfg.Media.TTS.OpusBitrate = 999999
	if got := cfg.OpusBitrate(); got != 510000 {
		t.Fatalf("OpusBitrate() = %d, want the clamp ceiling 510000", got)
	}
}
