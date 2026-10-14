// Package config manages application and campaign configuration, presets, and
// hierarchical setting cascades.
//
// It defines core configuration schemas (Config, ModelConfig, ProviderConfig in
// types.go), offline presets (presets.go), and provider validation rules. The Manager
// (manager.go) loads and cascades settings across system defaults, user global config
// files, project-local overrides, and runtime command-line flags.
//
// What config is NOT:
//   - It does NOT resolve operating-system XDG filesystem paths; base path resolution
//     belongs to pkg/paths.
//   - It does NOT instantiate active API clients or model runners; client construction
//     belongs to pkg/harness, pkg/media, and pkg/provider.
//   - It does NOT store campaign runtime state; game state belongs to pkg/engine.
package config
