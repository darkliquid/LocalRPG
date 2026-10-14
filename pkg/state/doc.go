// Package state provides a thread-safe, in-memory store for hierarchical,
// schema-agnostic game mechanics data.
//
// State (state.go) wraps a nested map[string]any protected by a read-write mutex.
// It supports dot-separated keypath access (e.g. "stats.hp", "inventory.gold"),
// automatically creates intermediate maps on write, and provides shallow snapshots
// for serialization or rules evaluation.
//
// What state is NOT:
//   - It does NOT define game system schemas, character attributes, or class rules;
//     the engine is schema-agnostic, and schemas are defined by YAML system packages
//     and dynamic scripts.
//   - It does NOT evaluate game rules, dice modifiers, or check outcomes; rule
//     execution belongs to pkg/rules.
//   - It does NOT persist state directly to disk; durable mechanics state is stored
//     in entity frontmatter via pkg/engine/timeline.go.
package state
