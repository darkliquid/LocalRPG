package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"
)

type ConfigManager struct {
	mu              sync.RWMutex
	userConfigPath  string
	localConfigPath string
	activeConfig    *Config
	isOverride      bool
	// warnings holds the problems Validate found in the last loaded config, so
	// the caller can surface them without the load failing.
	warnings []string
	// revision advances on every successful Load and Save, so callers can key
	// caches on the configuration without diffing it.
	revision atomic.Uint64
}

// Revision is a monotonic counter that changes whenever the configuration is
// loaded or saved.
func (m *ConfigManager) Revision() uint64 { return m.revision.Load() }

// Warnings returns the problems found during the last Load, newest first
// replaced wholesale each time.
func (m *ConfigManager) Warnings() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.warnings...)
}

// DetectConfigFile returns the config file to read and where to write one. An
// explicit LOCALRPG_CONFIG_DIR wins; otherwise the XDG config search path is
// used (XDG_CONFIG_HOME then XDG_CONFIG_DIRS), falling back to the application's
// config directory for the first save.
func DetectConfigFile() (read, write string) {
	if dir := os.Getenv("LOCALRPG_CONFIG_DIR"); dir != "" {
		path := filepath.Join(dir, "config.yaml")
		return path, path
	}
	rel := filepath.Join("localrpg", "config.yaml")
	if found, err := xdg.SearchConfigFile(rel); err == nil && found != "" {
		return found, found
	}
	fallback := filepath.Join(xdg.ConfigHome, "localrpg", "config.yaml")
	return fallback, fallback
}

func NewConfigManager() *ConfigManager {
	userPath, _ := DetectConfigFile()
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

// StylesDir is where style packs live: a folder beside the active configuration
// file, so a project override carries its own packs.
func (m *ConfigManager) StylesDir() string {
	dir := filepath.Dir(m.ActiveFilePath())
	if dir == "" || dir == "." {
		return "styles"
	}
	return filepath.Join(dir, "styles")
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

	m.warnings = merged.Validate()
	m.activeConfig = merged
	m.revision.Add(1)
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
	m.warnings = cfg.Validate()
	m.revision.Add(1)
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
