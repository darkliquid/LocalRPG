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
	configDir := os.Getenv("LOCALRPG_CONFIG_DIR")
	if configDir == "" {
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			configDir = filepath.Join(xdg, "localrpg")
		} else {
			homeDir, _ := os.UserHomeDir()
			configDir = filepath.Join(homeDir, ".config", "localrpg")
		}
	}
	userPath := filepath.Join(configDir, "config.yaml")
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
