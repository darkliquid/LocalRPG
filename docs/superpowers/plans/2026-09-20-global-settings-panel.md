# Global Settings Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Provide a comprehensive global settings system and UI for configuring storage paths, AI agent role routing, media engines (TTS, STT, Image Gen) across builtin/http/cli/disabled types, with live provider diagnostics and dual-access frontend UI.

**Architecture:** A dedicated `pkg/config` package manages hierarchical config resolution (`./localrpg.yaml` vs `~/.config/localrpg/config.yaml`). The backend `gui.Service` hot-swaps active storage paths and model/media pipelines on save without restarts. REST endpoints serve config and execute live provider diagnostic tests. A frontend `SettingsStudio` component integrates into `LauncherHub` and an in-game slide-over drawer.

**Tech Stack:** Go 1.24, `gopkg.in/yaml.v3`, React 18, Tailwind CSS, TypeScript, Lucide React icons.

---

### File Map

- **Backend Configuration & Types:**
  - Create: `pkg/config/types.go` (Config schemas for paths, agents, media, preferences)
  - Create: `pkg/config/manager.go` (Hierarchical loader, saver, defaults, thread-safe manager)
  - Test: `pkg/config/manager_test.go`
- **Core PathResolver Dynamic Paths:**
  - Modify: `pkg/core/types.go` (Support custom configured paths)
  - Test: `pkg/core/types_test.go`
- **Media Engine Providers (HTTP, CLI, Builtin, Disabled):**
  - Create: `pkg/media/providers.go` (HTTP/CLI/Builtin/Disabled clients for TTS, STT, Image)
  - Test: `pkg/media/providers_test.go`
- **Model Harness Provider Factory:**
  - Modify: `pkg/harness/types.go` (Add BuiltinName, validate provider types)
  - Create: `pkg/harness/factory.go` (Construct ModelProvider from ProviderConfig)
  - Test: `pkg/harness/factory_test.go`
- **GUI Service & REST Endpoints:**
  - Modify: `pkg/gui/types.go` (Settings DTOs, diagnostics request/response)
  - Modify: `pkg/gui/service.go` (GetSettings, SaveSettings with dynamic re-binding, TestProvider)
  - Modify: `pkg/gui/server.go` (Route registration for `/api/settings` and `/api/settings/test-provider`)
  - Test: `pkg/gui/server_test.go`
- **CLI Commands Integration:**
  - Modify: `cmd/localrpg/gui.go`, `cmd/localrpg/play.go`, `cmd/localrpg/media.go` (Use config.Load)
- **Frontend Types & API Client:**
  - Modify: `frontend/src/types.ts`
  - Modify: `frontend/src/api/client.ts`
- **Frontend Settings Studio & In-Game Drawer:**
  - Create: `frontend/src/components/SettingsStudio.tsx`
  - Modify: `frontend/src/components/LauncherHub.tsx` (Add Settings tab)
  - Modify: `frontend/src/App.tsx` (Add Settings gear icon and drawer)

---

### Task 1: Configuration Data Model & Hierarchical Manager (`pkg/config`)

**Files:**
- Create: `pkg/config/types.go`
- Create: `pkg/config/manager.go`
- Test: `pkg/config/manager_test.go`

- [x] **Step 1: Write failing tests for ConfigManager**

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestConfigManager_Defaults(t *testing.T) {
	mgr := config.NewConfigManagerWithPaths("/non/existent/user/config.yaml", "/non/existent/local/localrpg.yaml")
	cfg, err := mgr.Load()
	if err != nil {
		t.Fatalf("unexpected error loading defaults: %v", err)
	}

	if cfg.Paths.Systems != "./systems" {
		t.Errorf("expected default systems path ./systems, got %q", cfg.Paths.Systems)
	}
	if cfg.Agents.DefaultRole != "gm" {
		t.Errorf("expected default role gm, got %q", cfg.Agents.DefaultRole)
	}
	if cfg.Media.TTS.Type != "disabled" {
		t.Errorf("expected default tts type disabled, got %q", cfg.Media.TTS.Type)
	}
}

func TestConfigManager_HierarchicalSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	userConfigPath := filepath.Join(tmpDir, "user", "config.yaml")
	localConfigPath := filepath.Join(tmpDir, "local", "localrpg.yaml")

	mgr := config.NewConfigManagerWithPaths(userConfigPath, localConfigPath)

	// Save when neither exists -> should write to user config
	cfg, _ := mgr.Load()
	cfg.Paths.Systems = "/custom/systems"
	if err := mgr.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	if _, err := os.Stat(userConfigPath); os.IsNotExist(err) {
		t.Fatalf("expected user config to be created at %s", userConfigPath)
	}

	// Now create a local override
	_ = os.MkdirAll(filepath.Dir(localConfigPath), 0755)
	_ = os.WriteFile(localConfigPath, []byte("paths:\n  systems: /local/override\n"), 0644)

	reloaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if reloaded.Paths.Systems != "/local/override" {
		t.Errorf("expected local override /local/override, got %q", reloaded.Paths.Systems)
	}
	if !mgr.IsLocalOverride() {
		t.Errorf("expected IsLocalOverride to be true")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/config`
Expected: FAIL (package does not exist)

- [x] **Step 3: Implement `pkg/config/types.go` and `pkg/config/manager.go`**

`pkg/config/types.go`:
```go
package config

type PathsConfig struct {
	Systems string `yaml:"systems" json:"systems"`
	Worlds  string `yaml:"worlds" json:"worlds"`
	Games   string `yaml:"games" json:"games"`
	Cache   string `yaml:"cache" json:"cache"`
}

type AgentRoleConfig struct {
	Type        string   `yaml:"type" json:"type"` // "builtin", "http", "cli", "disabled"
	BuiltinName string   `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command     string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args        []string `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint    string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model       string   `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey      string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Temperature float64  `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens   int      `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
}

type AgentsConfig struct {
	DefaultRole string                     `yaml:"default_role" json:"default_role"`
	Roles       map[string]AgentRoleConfig `yaml:"roles" json:"roles"`
	Fallbacks   map[string]string          `yaml:"fallbacks,omitempty" json:"fallbacks,omitempty"`
}

type TTSConfig struct {
	Type         string   `yaml:"type" json:"type"` // "builtin", "http", "cli", "disabled"
	BuiltinName  string   `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command      string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args         []string `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint     string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model        string   `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey       string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	DefaultVoice string   `yaml:"default_voice,omitempty" json:"default_voice,omitempty"`
	Pitch        float64  `yaml:"pitch,omitempty" json:"pitch,omitempty"`
	SpeechRate   float64  `yaml:"speech_rate,omitempty" json:"speech_rate,omitempty"`
	AutoPlay     bool     `yaml:"auto_play" json:"auto_play"`
	MasterVolume float64  `yaml:"master_volume" json:"master_volume"`
}

type STTConfig struct {
	Type        string   `yaml:"type" json:"type"`
	BuiltinName string   `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command     string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args        []string `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint    string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model       string   `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey      string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}

type ImageConfig struct {
	Type         string   `yaml:"type" json:"type"`
	BuiltinName  string   `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command      string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args         []string `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint     string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model        string   `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey       string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	AutoGenerate bool     `yaml:"auto_generate" json:"auto_generate"`
}

type MediaConfig struct {
	TTS   TTSConfig   `yaml:"tts" json:"tts"`
	STT   STTConfig   `yaml:"stt" json:"stt"`
	Image ImageConfig `yaml:"image" json:"image"`
}

type PreferencesConfig struct {
	Streaming        bool   `yaml:"streaming" json:"streaming"`
	TypingSpeedMS    int    `yaml:"typing_speed_ms" json:"typing_speed_ms"`
	CinematicEffects bool   `yaml:"cinematic_effects" json:"cinematic_effects"`
	FontScale        string `yaml:"font_scale" json:"font_scale"`
}

type Config struct {
	Version     string            `yaml:"version" json:"version"`
	Paths       PathsConfig       `yaml:"paths" json:"paths"`
	Agents      AgentsConfig      `yaml:"agents" json:"agents"`
	Media       MediaConfig       `yaml:"media" json:"media"`
	Preferences PreferencesConfig `yaml:"preferences" json:"preferences"`
}

func DefaultConfig() *Config {
	return &Config{
		Version: "1",
		Paths: PathsConfig{
			Systems: "./systems",
			Worlds:  "./worlds",
			Games:   "./games",
			Cache:   "./cache",
		},
		Agents: AgentsConfig{
			DefaultRole: "gm",
			Roles: map[string]AgentRoleConfig{
				"gm": {
					Type:        "cli",
					Command:     "echo",
					Args:        []string{},
					Temperature: 0.7,
					MaxTokens:   1024,
				},
				"narrator": {
					Type: "disabled",
				},
			},
			Fallbacks: make(map[string]string),
		},
		Media: MediaConfig{
			TTS: TTSConfig{
				Type:         "disabled",
				DefaultVoice: "default",
				Pitch:        1.0,
				SpeechRate:   1.0,
				AutoPlay:     false,
				MasterVolume: 1.0,
			},
			STT: STTConfig{
				Type: "disabled",
			},
			Image: ImageConfig{
				Type:         "disabled",
				AutoGenerate: false,
			},
		},
		Preferences: PreferencesConfig{
			Streaming:        true,
			TypingSpeedMS:    15,
			CinematicEffects: true,
			FontScale:        "medium",
		},
	}
}
```

`pkg/config/manager.go`:
```go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

type ConfigManager struct {
	mu              sync.RWMutex
	userConfigPath  string
	localConfigPath string
	activeConfig    *Config
	isOverride      bool
}

func NewConfigManager() *ConfigManager {
	homeDir, _ := os.UserHomeDir()
	userPath := filepath.Join(homeDir, ".config", "localrpg", "config.yaml")
	localPath := "./localrpg.yaml"
	return NewConfigManagerWithPaths(userPath, localPath)
}

func NewConfigManagerWithPaths(userPath, localPath string) *ConfigManager {
	return &ConfigManager{
		userConfigPath:  userPath,
		localConfigPath: localPath,
		activeConfig:    DefaultConfig(),
	}
}

func (m *ConfigManager) ActiveFilePath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.isOverride {
		return m.localConfigPath
	}
	return m.userConfigPath
}

func (m *ConfigManager) IsLocalOverride() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isOverride
}

func (m *ConfigManager) Load() (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	merged := DefaultConfig()

	// 1. Try User Global Config
	if data, err := os.ReadFile(m.userConfigPath); err == nil {
		_ = yaml.Unmarshal(data, merged)
	}

	// 2. Try Local Override
	if data, err := os.ReadFile(m.localConfigPath); err == nil {
		if err := yaml.Unmarshal(data, merged); err == nil {
			m.isOverride = true
		}
	} else {
		m.isOverride = false
	}

	m.activeConfig = merged
	return m.activeConfig, nil
}

func (m *ConfigManager) Save(cfg *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	targetPath := m.userConfigPath
	if _, err := os.Stat(m.localConfigPath); err == nil || m.isOverride {
		targetPath = m.localConfigPath
		m.isOverride = true
	} else {
		m.isOverride = false
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	m.activeConfig = cfg
	return nil
}

func (m *ConfigManager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activeConfig == nil {
		return DefaultConfig()
	}
	return m.activeConfig
}
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/config`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/config/
git commit -m "feat(config): add configuration schema and hierarchical manager"
```

---

### Task 2: Core PathResolver Dynamic Paths Integration

**Files:**
- Modify: `pkg/core/types.go`
- Test: `pkg/core/types_test.go`

- [x] **Step 1: Write failing test for dynamic PathResolver**

Add to `pkg/core/types_test.go`:
```go
func TestPathResolver_CustomPaths(t *testing.T) {
	resolver := core.NewCustomPathResolver("/custom/sys", "/custom/worlds", "/custom/games")
	if resolver.SystemsDir() != "/custom/sys" {
		t.Errorf("expected /custom/sys, got %q", resolver.SystemsDir())
	}
	if resolver.WorldsDir() != "/custom/worlds" {
		t.Errorf("expected /custom/worlds, got %q", resolver.WorldsDir())
	}
	if resolver.GamesDir() != "/custom/games" {
		t.Errorf("expected /custom/games, got %q", resolver.GamesDir())
	}
	if resolver.SystemDir("d20") != "/custom/sys/d20" {
		t.Errorf("expected /custom/sys/d20, got %q", resolver.SystemDir("d20"))
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/core -run TestPathResolver_CustomPaths`
Expected: FAIL (`NewCustomPathResolver` undefined)

- [x] **Step 3: Update `pkg/core/types.go`**

Add fields and methods to `PathResolver`:
```go
type PathResolver struct {
	BaseDir    string
	systemsDir string
	worldsDir  string
	gamesDir   string
}

func NewPathResolver(baseDir string) *PathResolver {
	return &PathResolver{
		BaseDir:    baseDir,
		systemsDir: filepath.Join(baseDir, "systems"),
		worldsDir:  filepath.Join(baseDir, "worlds"),
		gamesDir:   filepath.Join(baseDir, "games"),
	}
}

func NewCustomPathResolver(systemsDir, worldsDir, gamesDir string) *PathResolver {
	return &PathResolver{
		BaseDir:    "",
		systemsDir: systemsDir,
		worldsDir:  worldsDir,
		gamesDir:   gamesDir,
	}
}

func (p *PathResolver) SetPaths(systemsDir, worldsDir, gamesDir string) {
	p.systemsDir = systemsDir
	p.worldsDir = worldsDir
	p.gamesDir = gamesDir
}

func (p *PathResolver) SystemsDir() string {
	if p.systemsDir != "" {
		return p.systemsDir
	}
	return filepath.Join(p.BaseDir, "systems")
}

func (p *PathResolver) WorldsDir() string {
	if p.worldsDir != "" {
		return p.worldsDir
	}
	return filepath.Join(p.BaseDir, "worlds")
}

func (p *PathResolver) GamesDir() string {
	if p.gamesDir != "" {
		return p.gamesDir
	}
	return filepath.Join(p.BaseDir, "games")
}
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/core`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/core/
git commit -m "feat(core): support custom dynamic paths in PathResolver"
```

---

### Task 3: Unified Media Engine Clients (`builtin`, `http`, `cli`, `disabled`)

**Files:**
- Create: `pkg/media/providers.go`
- Test: `pkg/media/providers_test.go`

- [x] **Step 1: Write failing tests for media engine providers**

`pkg/media/providers_test.go`:
```go
package media_test

import (
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestMediaProviders_Disabled(t *testing.T) {
	ttsClient, err := media.NewTTSClient(config.TTSConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = ttsClient.Synthesize(context.Background(), "Hello", nil)
	if !errors.Is(err, media.ErrProviderDisabled) {
		t.Errorf("expected ErrProviderDisabled, got %v", err)
	}

	sttClient, err := media.NewSTTClient(config.STTConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = sttClient.Transcribe(context.Background(), []byte("audio"))
	if !errors.Is(err, media.ErrProviderDisabled) {
		t.Errorf("expected ErrProviderDisabled, got %v", err)
	}

	imgClient, err := media.NewImageClient(config.ImageConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = imgClient.GenerateImage(context.Background(), "A dark tower")
	if !errors.Is(err, media.ErrProviderDisabled) {
		t.Errorf("expected ErrProviderDisabled, got %v", err)
	}
}

func TestMediaProviders_BuiltinEcho(t *testing.T) {
	ttsClient, err := media.NewTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bytes, err := ttsClient.Synthesize(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("synthesize failed: %v", err)
	}
	if len(bytes) == 0 {
		t.Errorf("expected non-empty audio bytes from builtin echo")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/media -run TestMediaProviders_Disabled`
Expected: FAIL (`NewTTSClient` undefined)

- [x] **Step 3: Implement `pkg/media/providers.go`**

```go
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
)

var ErrProviderDisabled = errors.New("provider is disabled")

// Disabled implementations
type disabledTTSClient struct{}
func (d *disabledTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return nil, ErrProviderDisabled
}

type disabledSTTClient struct{}
func (d *disabledSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	return "", ErrProviderDisabled
}

type disabledImageClient struct{}
func (d *disabledImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	return nil, ErrProviderDisabled
}

// Builtin / Echo implementations
type echoTTSClient struct{}
func (e *echoTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return []byte("RIFF....WAVEfmt ....data" + text), nil
}

type echoSTTClient struct{}
func (e *echoSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	return "Transcribed audio sample", nil
}

type echoImageClient struct{}
func (e *echoImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	return []byte("fake-image-bytes-for-" + prompt), nil
}

// CLI implementations
type cliTTSClient struct {
	command string
	args    []string
}
func (c *cliTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.command, c.args...)
	cmd.Stdin = bytes.NewBufferString(text)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cli tts error: %w", err)
	}
	return out.Bytes(), nil
}

type cliSTTClient struct {
	command string
	args    []string
}
func (c *cliSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	cmd := exec.CommandContext(ctx, c.command, c.args...)
	cmd.Stdin = bytes.NewReader(audioData)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cli stt error: %w", err)
	}
	return out.String(), nil
}

type cliImageClient struct {
	command string
	args    []string
}
func (c *cliImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.command, append(c.args, prompt)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cli image error: %w", err)
	}
	return out.Bytes(), nil
}

// HTTP implementations (OpenAI compatible)
type httpTTSClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}
func (h *httpTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	voiceID := "alloy"
	if voice != nil && voice.VoiceID != "" {
		voiceID = voice.VoiceID
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"model": h.model,
		"input": text,
		"voice": voiceID,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", h.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http tts failed (%d): %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

type httpImageClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}
func (h *httpImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	payload, _ := json.Marshal(map[string]interface{}{
		"model":  h.model,
		"prompt": prompt,
		"n":      1,
		"size":   "512x512",
	})
	req, err := http.NewRequestWithContext(ctx, "POST", h.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http image failed (%d): %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

// Factory Constructors
func NewTTSClient(cfg config.TTSConfig) (TTSClient, error) {
	switch cfg.Type {
	case "disabled", "":
		return &disabledTTSClient{}, nil
	case "builtin":
		return &echoTTSClient{}, nil
	case "cli":
		return &cliTTSClient{command: cfg.Command, args: cfg.Args}, nil
	case "http":
		return &httpTTSClient{endpoint: cfg.Endpoint, model: cfg.Model, apiKey: cfg.APIKey, client: &http.Client{Timeout: 30 * time.Second}}, nil
	default:
		return nil, fmt.Errorf("unsupported tts provider type: %s", cfg.Type)
	}
}

func NewSTTClient(cfg config.STTConfig) (STTClient, error) {
	switch cfg.Type {
	case "disabled", "":
		return &disabledSTTClient{}, nil
	case "builtin":
		return &echoSTTClient{}, nil
	case "cli":
		return &cliSTTClient{command: cfg.Command, args: cfg.Args}, nil
	default:
		return nil, fmt.Errorf("unsupported stt provider type: %s", cfg.Type)
	}
}

func NewImageClient(cfg config.ImageConfig) (ImageClient, error) {
	switch cfg.Type {
	case "disabled", "":
		return &disabledImageClient{}, nil
	case "builtin":
		return &echoImageClient{}, nil
	case "cli":
		return &cliImageClient{command: cfg.Command, args: cfg.Args}, nil
	case "http":
		return &httpImageClient{endpoint: cfg.Endpoint, model: cfg.Model, apiKey: cfg.APIKey, client: &http.Client{Timeout: 60 * time.Second}}, nil
	default:
		return nil, fmt.Errorf("unsupported image provider type: %s", cfg.Type)
	}
}
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/media`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/media/
git commit -m "feat(media): add unified TTS, STT, and Image clients for builtin, http, cli, and disabled"
```

---

### Task 4: Harness Factory & Builtin/Disabled Provider Support

**Files:**
- Modify: `pkg/harness/types.go`
- Create: `pkg/harness/factory.go`
- Test: `pkg/harness/factory_test.go`

- [x] **Step 1: Write failing test for Harness ModelProvider factory**

`pkg/harness/factory_test.go`:
```go
package harness_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestNewModelProvider(t *testing.T) {
	// Disabled
	disabled, err := harness.NewModelProvider("p1", harness.ProviderConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disabled.ID() != "p1" {
		t.Errorf("expected id p1, got %q", disabled.ID())
	}

	// CLI
	cli, err := harness.NewModelProvider("p2", harness.ProviderConfig{
		Type:    "cli",
		Command: "echo",
		Args:    []string{"hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, err := cli.Generate(context.Background(), harness.GenerateRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if resp.Text == "" {
		t.Errorf("expected output from echo cli provider")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/harness -run TestNewModelProvider`
Expected: FAIL (`NewModelProvider` undefined)

- [x] **Step 3: Update `pkg/harness/types.go` and implement `pkg/harness/factory.go`**

In `pkg/harness/types.go`, add `BuiltinName`:
```go
type ProviderConfig struct {
	Type        string   `yaml:"type"` // "builtin", "cli", "http", "mock", "disabled"
	BuiltinName string   `yaml:"builtin_name,omitempty"`
	Command     string   `yaml:"command,omitempty"`
	Args        []string `yaml:"args,omitempty"`
	Endpoint    string   `yaml:"endpoint,omitempty"`
	Model       string   `yaml:"model,omitempty"`
	APIKey      string   `yaml:"api_key,omitempty"`
	Temperature float64  `yaml:"temperature,omitempty"`
}
```

`pkg/harness/factory.go`:
```go
package harness

import (
	"context"
	"fmt"
)

type disabledModelProvider struct {
	id string
}
func (d *disabledModelProvider) ID() string { return d.id }
func (d *disabledModelProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("provider %q is disabled", d.id)
}
func (d *disabledModelProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)
	return fmt.Errorf("provider %q is disabled", d.id)
}

func NewModelProvider(id string, cfg ProviderConfig) (ModelProvider, error) {
	switch cfg.Type {
	case "disabled":
		return &disabledModelProvider{id: id}, nil
	case "cli":
		return NewCLIProvider(id, cfg.Command, cfg.Args), nil
	case "http":
		return NewHTTPProvider(id, cfg.Endpoint, cfg.Model, cfg.APIKey), nil
	case "builtin", "mock", "":
		return NewCLIProvider(id, "echo", []string{}), nil
	default:
		return nil, fmt.Errorf("unknown model provider type: %s", cfg.Type)
	}
}
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/harness`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/harness/
git commit -m "feat(harness): add model provider factory with disabled and builtin support"
```

---

### Task 5: GUI Service Dynamic Re-binding & Diagnostics Endpoints

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/server.go`
- Test: `pkg/gui/server_test.go`

- [x] **Step 1: Write failing tests for Settings and Diagnostics API**

In `pkg/gui/server_test.go`:
```go
func TestSettingsEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := storage.NewStore(filepath.Join(tmpDir, "test.db"))
	defer store.Close()

	paths := core.NewPathResolver(tmpDir)
	svc := gui.NewService(store, paths, nil)
	server := gui.NewServer(svc, nil)

	// 1. GET /api/settings
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from GET /api/settings, got %d: %s", w.Code, w.Body.String())
	}

	var res gui.SettingsResponseDTO
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode settings: %v", err)
	}
	if res.Config.Paths.Systems == "" {
		t.Errorf("expected non-empty systems path")
	}

	// 2. PUT /api/settings
	newSysPath := filepath.Join(tmpDir, "new_systems")
	res.Config.Paths.Systems = newSysPath
	putBody, _ := json.Marshal(res)
	req2 := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(putBody))
	w2 := httptest.NewRecorder()
	server.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from PUT /api/settings, got %d: %s", w2.Code, w2.Body.String())
	}

	// Verify path resolver dynamically updated
	if svc.GetPaths().SystemsDir() != newSysPath {
		t.Errorf("expected service to dynamically update systems dir to %s, got %s", newSysPath, svc.GetPaths().SystemsDir())
	}

	// 3. POST /api/settings/test-provider
	testReqBody := `{"category":"llm","provider":{"type":"cli","command":"echo","args":["pong"]},"test_prompt":"ping"}`
	req3 := httptest.NewRequest(http.MethodPost, "/api/settings/test-provider", strings.NewReader(testReqBody))
	w3 := httptest.NewRecorder()
	server.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from POST /api/settings/test-provider, got %d: %s", w3.Code, w3.Body.String())
	}
	var testRes gui.TestProviderResponseDTO
	_ = json.Unmarshal(w3.Body.Bytes(), &testRes)
	if !testRes.Success {
		t.Errorf("expected test provider success: %s", testRes.Message)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/gui -run TestSettingsEndpoints`
Expected: FAIL (404 / undefined)

- [x] **Step 3: Update `pkg/gui/types.go`**

Add DTOs to `pkg/gui/types.go`:
```go
import (
	"github.com/darkliquid/localrpg/pkg/config"
)

type SettingsResponseDTO struct {
	Config          config.Config `json:"config"`
	ConfigFilePath  string        `json:"config_file_path"`
	IsLocalOverride bool          `json:"is_local_override"`
}

type TestProviderRequestDTO struct {
	Category   string                   `json:"category"` // "llm", "tts", "stt", "image"
	Provider   config.AgentRoleConfig   `json:"provider"`
	TestPrompt string                   `json:"test_prompt,omitempty"`
}

type TestProviderResponseDTO struct {
	Success   bool   `json:"success"`
	LatencyMS int64  `json:"latency_ms"`
	Message   string `json:"message"`
	Preview   string `json:"preview,omitempty"`
}
```

- [x] **Step 4: Update `pkg/gui/service.go` and `pkg/gui/server.go`**

Update `Service` in `pkg/gui/service.go`:
- Store `configMgr *config.ConfigManager`.
- Expose `GetSettings()`, `SaveSettings(cfg config.Config)`, `TestProvider(req TestProviderRequestDTO)`, `GetPaths() *core.PathResolver`.
- On `SaveSettings`, update paths and make sure directories exist.
Update `Server` in `pkg/gui/server.go`:
- Register `/api/settings` and `/api/settings/test-provider`.

- [x] **Step 5: Run tests to verify they pass**

Run: `go test -v ./pkg/gui`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add pkg/gui/
git commit -m "feat(gui): implement settings endpoints, dynamic path re-binding, and live diagnostics"
```

---

### Task 6: CLI Commands Global Config Wiring

**Files:**
- Modify: `cmd/localrpg/play.go`
- Modify: `cmd/localrpg/media.go`
- Modify: `cmd/localrpg/gui.go`
- Test: `cmd/localrpg/gui_test.go`, `cmd/localrpg/play_test.go`

- [x] **Step 1: Write test verifying CLI commands initialize from config**

Run: `go test -v ./cmd/localrpg`
Ensure baseline passes.

- [x] **Step 2: Update `cmd/localrpg/play.go`, `media.go`, `gui.go` to use `config.NewConfigManager().Load()`**

Replace hardcoded `baseDir := "."` and default `echo` router with resolved config paths and configured agent roles.

- [x] **Step 3: Run tests to verify they pass**

Run: `go test -v ./cmd/localrpg`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add cmd/localrpg/
git commit -m "feat(cli): wire global config into play, media, and gui commands"
```

---

### Task 7: Frontend Types & API Client Integration

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/api/client.ts`

- [x] **Step 1: Add configuration and diagnostics types to `frontend/src/types.ts`**

```typescript
export interface PathsConfig {
  systems: string;
  worlds: string;
  games: string;
  cache: string;
}

export interface AgentRoleConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  temperature?: number;
  max_tokens?: number;
}

export interface AgentsConfig {
  default_role: string;
  roles: Record<string, AgentRoleConfig>;
  fallbacks?: Record<string, string>;
}

export interface TTSConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  default_voice?: string;
  pitch?: number;
  speech_rate?: number;
  auto_play: boolean;
  master_volume: number;
}

export interface STTConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
}

export interface ImageConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  auto_generate: boolean;
}

export interface MediaConfig {
  tts: TTSConfig;
  stt: STTConfig;
  image: ImageConfig;
}

export interface PreferencesConfig {
  streaming: boolean;
  typing_speed_ms: number;
  cinematic_effects: boolean;
  font_scale: 'small' | 'medium' | 'large';
}

export interface AppConfig {
  version: string;
  paths: PathsConfig;
  agents: AgentsConfig;
  media: MediaConfig;
  preferences: PreferencesConfig;
}

export interface SettingsResponse {
  config: AppConfig;
  config_file_path: string;
  is_local_override: boolean;
}

export interface TestProviderRequest {
  category: 'llm' | 'tts' | 'stt' | 'image';
  provider: AgentRoleConfig;
  test_prompt?: string;
}

export interface TestProviderResponse {
  success: boolean;
  latency_ms: number;
  message: string;
  preview?: string;
}
```

- [x] **Step 2: Add `getSettings`, `saveSettings`, `testProvider` to `frontend/src/api/client.ts`**

```typescript
  static async getSettings(): Promise<SettingsResponse> {
    const res = await fetch(`${API_BASE}/api/settings`);
    if (!res.ok) throw new Error(`Failed to load settings: ${res.statusText}`);
    return res.json();
  }

  static async saveSettings(cfg: AppConfig): Promise<SettingsResponse> {
    const res = await fetch(`${API_BASE}/api/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ config: cfg }),
    });
    if (!res.ok) throw new Error(`Failed to save settings: ${res.statusText}`);
    return res.json();
  }

  static async testProvider(req: TestProviderRequest): Promise<TestProviderResponse> {
    const res = await fetch(`${API_BASE}/api/settings/test-provider`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new Error(`Test provider failed: ${res.statusText}`);
    return res.json();
  }
```

- [x] **Step 3: Run typescript verification**

Run: `npx --prefix frontend tsc --noEmit`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): add settings and provider diagnostics API types and client methods"
```

---

### Task 8: Frontend SettingsStudio Component & Diagnostics UI

**Files:**
- Create: `frontend/src/components/SettingsStudio.tsx`
- Test: `npx --prefix frontend tsc --noEmit`

- [x] **Step 1: Implement `frontend/src/components/SettingsStudio.tsx`**

Build tabbed panel covering:
1. **Paths & Storage**: editable inputs for systems, worlds, games, cache; file location badge; save button.
2. **AI Agents & Roles**: role selection (`gm`, `narrator`, `evaluator`), type dropdown (`builtin`, `http`, `cli`, `disabled`), contextual fields, temperature, and "Test Connection" button with live spinner & results card.
3. **Media Engines**: sub-sections for TTS, STT, Image generation. TTS volume & speech rate controls, auto-play toggle, auto-generate scene art toggle. "Test Engine" button for each.
4. **Preferences & Appearance**: CRT/noise toggle, font scaling, token streaming toggle, typing speed.

- [x] **Step 2: Run typescript check**

Run: `npx --prefix frontend tsc --noEmit`
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): add comprehensive SettingsStudio component with live provider diagnostics"
```

---

### Task 9: Dual-Access Navigation Integration (LauncherHub & App In-Game Header)

**Files:**
- Modify: `frontend/src/components/LauncherHub.tsx`
- Modify: `frontend/src/App.tsx`
- Test: `mise run test`

- [x] **Step 1: Add Settings tab to `LauncherHub.tsx`**

Add `Settings` to activeTab options (`'campaigns' | 'systems' | 'worlds' | 'settings'`), add tab button with `<Settings className="w-3.5 h-3.5" />` in header navigation, and render `<SettingsStudio />` when active.

- [x] **Step 2: Add Settings gear button & drawer in `App.tsx`**

Add gear button in header pill triggers:
```tsx
<button
  onClick={() => setActiveDrawer('settings')}
  className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
    activeDrawer === 'settings' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
  }`}
  title="Global Settings"
>
  <Settings className="w-3.5 h-3.5 text-amber-400" />
  <span className="hidden sm:inline">Settings</span>
</button>
```
Render `<SettingsStudio isCompact={true} />` inside `<Drawers>` when `activeDrawer === 'settings'`.

- [x] **Step 3: Run full project test suite**

Run: `mise run test`
Expected: All 12 Go packages pass and `tsc --noEmit` passes.

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/LauncherHub.tsx frontend/src/App.tsx
git commit -m "feat(frontend): integrate SettingsStudio into LauncherHub and in-game header drawer"
```

---

### Task 10: End-to-End Verification & Documentation

**Files:**
- Test: `mise run test`
- Test: CLI commands `localrpg gui --help`, `localrpg play --help`
- Modify: `README.md` (Document global settings hierarchy and configuration)

- [x] **Step 1: Run comprehensive tests**

Run: `mise run test`
Expected: All Go unit tests pass, TypeScript builds with 0 errors.

- [x] **Step 2: Update README.md with configuration documentation**

Add section:
```markdown
## Configuration & Global Settings

LocalRPG supports global configuration via `~/.config/localrpg/config.yaml` or a project-level override `./localrpg.yaml`.
Configure storage paths, AI agent role routing (Ollama, vLLM, CLI binaries), and media engines (TTS, STT, Image generation) via the web GUI **Settings** tab or directly in YAML.
```

- [x] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add global settings and configuration documentation"
```
