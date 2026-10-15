package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"
)

// ConfigManager holds the configuration in force, the file it was read from, and a
// revision counter a caller can key a cache on. It is safe for concurrent use.
// ConfigManager holds the configuration in force, the file it was read from, and a
// revision counter a caller can key a cache on. It is safe for concurrent use.
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

// NewConfigManager resolves the standard locations: the user's configuration file,
// and a localrpg.yaml in the working directory as an override.
// NewConfigManager resolves the standard locations: the user's configuration file,
// and a localrpg.yaml in the working directory as an override.
func NewConfigManager() *ConfigManager {
	userPath, _ := DetectConfigFile()
	localPath := "./localrpg.yaml"
	return NewConfigManagerWithPaths(userPath, localPath)
}

// NewConfigManagerWithPaths builds a manager around explicit paths, which is what
// a test or a --dir run needs.
// NewConfigManagerWithPaths builds a manager around explicit paths, which is what
// a test or a --dir run needs.
func NewConfigManagerWithPaths(userPath, localPath string) *ConfigManager {
	return &ConfigManager{
		userConfigPath:  userPath,
		localConfigPath: localPath,
		activeConfig:    DefaultConfig(),
	}
}

// ActiveFilePath is the file a save would write: the local override when one is in
// force, and the user configuration otherwise.
// ActiveFilePath is the file a save would write: the local override when one is in
// force, and the user configuration otherwise.
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

// IsLocalOverride reports whether the local file is the one in force.
// IsLocalOverride reports whether the local file is the one in force.
func (m *ConfigManager) IsLocalOverride() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isOverride
}

// Load reads the user configuration, then a local override when there is one, and
// returns the layered result. A file that does not parse is reported: the error
// names it, the same failure is recorded in Warnings for a caller that has no
// error channel, and the file is skipped rather than half applied. The
// configuration returned is always usable and holds everything that did parse, so
// a caller which sees the error must report it rather than quietly use the
// defaults it fell back to.
func (m *ConfigManager) Load() (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	merged := DefaultConfig()
	var problems []error

	// 1. Try User Global Config
	if data, err := os.ReadFile(m.userConfigPath); err == nil {
		if err := mergeConfigFile(m.userConfigPath, data, merged); err != nil {
			problems = append(problems, err)
		}
	}

	// 2. Try Local Override
	if data, err := os.ReadFile(m.localConfigPath); err == nil {
		if err := mergeConfigFile(m.localConfigPath, data, merged); err == nil {
			m.isOverride = true
		} else {
			problems = append(problems, err)
		}
	} else {
		m.isOverride = false
	}

	m.warnings = merged.Validate()
	m.activeConfig = merged
	m.revision.Add(1)

	if err := errors.Join(problems...); err != nil {
		// A caller with no error channel still has the warnings list, which the GUI
		// logs and the Settings payload returns.
		m.warnings = append(m.warnings, err.Error())
		return merged, err
	}
	return m.activeConfig, nil
}

// mergeConfigFile layers one configuration document over cfg. It parses the
// document into a throwaway Config first, so a file that does not parse leaves cfg
// exactly as it was: the alternative is a configuration built from the prefix of a
// file that yaml managed to read before it gave up. The error names the file.
func mergeConfigFile(path string, data []byte, cfg *Config) error {
	var probe Config
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// Save writes the configuration to the active file. A local override that exists
// stays the active file, so a project keeps being edited in place.
// Save writes the configuration to the active file. A local override that exists
// stays the active file, so a project keeps being edited in place.
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

// Get returns the configuration in force without re-reading it. A manager that has
// never loaded answers with defaults.
// Get returns the configuration in force without re-reading it. A manager that has
// never loaded answers with defaults.
func (m *ConfigManager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activeConfig == nil {
		return DefaultConfig()
	}
	return m.activeConfig
}
