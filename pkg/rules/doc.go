// Package rules executes tabletop RPG mechanics and rules resolution using an
// embedded JavaScript runtime.
//
// The JSEngine (js_engine.go) embeds Goja to run sandboxed script hooks (such as
// onAction, onCheck, onTurn, onAdvance) defined by game systems. A host API
// (host_api.go) exposes deterministic dice rolling (dice.go), state inspection and
// mutation, table lookups, and modifier calculations. It also resolves opposed
// checks and validates mechanics state changes.
//
// What rules is NOT:
//   - It does NOT hardcode specific tabletop RPG rulesets (e.g. D&D 5e, PbtA, Fate);
//     all game systems are pluggable YAML and JS assets loaded at runtime.
//   - It does NOT coordinate turn generation or LLM interaction; turn progression
//     belongs to pkg/engine and prompt assembly belongs to pkg/harness.
//   - It does NOT handle persistent storage; in-memory data is held by pkg/state
//     and durable writes are executed by pkg/engine/timeline.go.
package rules
