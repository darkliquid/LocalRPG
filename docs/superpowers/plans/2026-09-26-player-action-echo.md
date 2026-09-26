# Player Action Echo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the narrator open each action turn by restating the player's action in the third person before resolving it.

**Architecture:** Add the player's name and an echo toggle to `ContextRequest`, render a non-droppable `PLAYER ACTION ECHO` instruction section in `buildSections`, and gate it in the orchestrator (off for `Say`, opening, `/gm`, `Roll`, and `System`). Configure it with an `agents.action_echo` pointer bool, default on.

**Tech Stack:** Go 1.27 (stdlib tests), React 19 + TypeScript.

**Spec:** `docs/superpowers/specs/2026-09-26-player-action-echo-design.md`

## Global Constraints

- Use `interface{}`, not `any`; `go vet` clean; `mise run test:backend`, `mise run lint`, `mise run test:frontend`.
- Default on for existing configs: a nil pointer means enabled.
- Do not echo for `Say`, opening, `/gm`, `Roll`, or `System`.
- Do not commit unless the user asks.

---

## File Map

- Modify: `pkg/config/types.go` (field + accessor), `pkg/config/types_test.go` or `manager_test.go`
- Modify: `pkg/harness/context.go` (ContextRequest + sections), `pkg/harness/context_test.go`
- Modify: `pkg/harness/turn_tools.go` (submit_turn description)
- Modify: `pkg/engine/orchestrator.go` (field, setter, gating, ContextRequest)
- Modify: `pkg/gui/service.go`, `cmd/localrpg/play.go` (wire the toggle)
- Modify: `frontend/src/types.ts`, `frontend/src/components/SettingsStudio.tsx`

---

### Task 1: Config toggle

**Files:**
- Modify: `pkg/config/types.go`
- Modify: `pkg/config/types_test.go` (create if absent)

**Interfaces:**
- Produces: `AgentsConfig.ActionEcho *bool`, `Config.ActionEcho() bool`.

- [x] **Step 1: Add the field**

In `pkg/config/types.go`, after `ContinuityChecks`:

```go
	// ActionEcho prepends the narrator's third-person restatement of the player's
	// action to each turn. A pointer distinguishes "not configured" (on) from
	// "switched off".
	ActionEcho *bool `yaml:"action_echo" json:"action_echo,omitempty"`
```

- [x] **Step 2: Add the accessor**

After `ContinuityChecks()`:

```go
// ActionEcho reports whether the narrator should restate the player's action.
// The default is on, so a configuration that never mentions it keeps echoing.
func (c *Config) ActionEcho() bool {
	return c.Agents.ActionEcho == nil || *c.Agents.ActionEcho
}
```

- [x] **Step 3: Test**

Add to `pkg/config/types_test.go` (create with `package config` if it does not exist):

```go
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
```

- [x] **Step 4: Run**

Run: `go test ./pkg/config/ -run TestActionEchoDefaultsOn -v`
Expected: PASS.

---

### Task 2: Context section

**Files:**
- Modify: `pkg/harness/context.go`, `pkg/harness/context_test.go`

**Interfaces:**
- Produces: `ContextRequest.PlayerName string`, `ContextRequest.ActionEcho bool`, `actionEchoSection() string`.

- [x] **Step 1: Fields**

In `ContextRequest`, after `Action`:

```go
	// PlayerName is the protagonist's display name, so the narrator can name
	// them when restating the action.
	PlayerName string
	// ActionEcho asks for a leading third-person restatement of the action.
	ActionEcho bool
```

- [x] **Step 2: The instruction text**

Add near the other section helpers in `pkg/harness/context.go`:

```go
// actionEchoInstruction tells the narrator to re-anchor the scene on the player's
// action before resolving it.
const actionEchoInstruction = `## PLAYER ACTION ECHO
Open every turn with one short narration segment that restates the player's action
in the third person, using the character's name or a pronoun, before anything else
happens. Do not change what they did, invent intent they did not state, or resolve
it in that sentence; re-anchor the scene on their action, then continue.

Example:
  Player (Stretch Layabout): I jump into my ship, blasting my pursuers as the hatch closes.
  Opening narration: Stretch jumps into their ship, firing blaster shots at their pursuers until the canopy seals shut.
`

func actionEchoSection(enabled bool, action string) string {
	if !enabled || strings.TrimSpace(action) == "" {
		return ""
	}
	return actionEchoInstruction + "\n"
}
```

- [x] **Step 3: Render it and name the actor**

In `buildSections`, compute the action text and add the section:

```go
	actionText := "\n## PLAYER ACTION\n"
	if name := strings.TrimSpace(req.PlayerName); name != "" {
		actionText += name + ": "
	}
	actionText += req.Action + "\n"
```

Add to the returned slice, immediately after `instructions`:

```go
		{name: "action_echo", source: "action_echo", text: actionEchoSection(req.ActionEcho, req.Action)},
```

and change the action section to `{name: "action", source: "player_action", text: actionText, refs: actionRefs}`.

- [x] **Step 4: Test**

Add to `pkg/harness/context_test.go`:

```go
func TestActionEchoSection(t *testing.T) {
	assembler := &ContextAssembler{}
	result, err := assembler.Assemble(ContextRequest{
		PlayerID:   "stretch",
		PlayerName: "Stretch Layabout",
		Action:     "I jump into my ship.",
		ActionEcho: true,
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if !strings.Contains(result.Prompt, "PLAYER ACTION ECHO") {
		t.Fatal("expected the echo instruction when enabled")
	}
	if !strings.Contains(result.Prompt, "Stretch Layabout: I jump into my ship.") {
		t.Fatal("expected the named action block")
	}

	off, err := assembler.Assemble(ContextRequest{
		PlayerID: "stretch", PlayerName: "Stretch Layabout", Action: "I jump.", ActionEcho: false,
	})
	if err != nil {
		t.Fatalf("Assemble off: %v", err)
	}
	if strings.Contains(off.Prompt, "PLAYER ACTION ECHO") {
		t.Fatal("echo instruction must be absent when disabled")
	}
}
```

(Use the repository's existing `ContextAssembler` construction in that file if it needs a store; match the neighbouring tests.)

- [x] **Step 5: Run**

Run: `go test ./pkg/harness/ -run TestActionEchoSection -v`
Expected: PASS. Fix any section-list assertions the new section breaks.

---

### Task 3: Orchestrator gating and wiring

**Files:**
- Modify: `pkg/engine/orchestrator.go`, `pkg/harness/turn_tools.go`
- Modify: `pkg/gui/service.go`, `cmd/localrpg/play.go`

**Interfaces:**
- Produces: `(*TurnOrchestrator).SetActionEcho(bool)`.

- [x] **Step 1: Field and setter**

In `pkg/engine/orchestrator.go`, add the field near `openingPrompt` and the setter near `SetContinuityChecks`:

```go
// SetActionEcho asks the narrator to open each action turn with a third-person
// restatement of the player's action.
func (o *TurnOrchestrator) SetActionEcho(enabled bool) {
	o.actionEcho = enabled
}
```

- [x] **Step 2: Gate it**

Where the assembly request is built (`orchestrator.go:620`), compute:

```go
	echoAction := o.actionEcho &&
		!isOpening && !isCorrection &&
		!strings.EqualFold(mode, "Roll") &&
		!strings.EqualFold(mode, "Say") &&
		strings.TrimSpace(actionInput) != ""
```

and pass:

```go
		PlayerName:       o.playerDisplayName(),
		ActionEcho:       echoAction,
```

- [x] **Step 3: Tool description**

In `pkg/harness/turn_tools.go`, append to the `submit_turn` description:

> Begin with a short third-person restatement of the player's action before resolving it.

- [x] **Step 4: Wire the setter**

In `pkg/gui/service.go` after `orchestrator.SetContinuityChecks(cfg.ContinuityChecks())`:

```go
	orchestrator.SetActionEcho(cfg.ActionEcho())
```

In `cmd/localrpg/play.go`, near the other orchestrator setters:

```go
	orchestrator.SetActionEcho(cfg.ActionEcho())
```

- [x] **Step 5: Run**

Run: `go test ./pkg/engine/ ./pkg/harness/ && go vet ./pkg/engine/ ./pkg/harness/`
Expected: PASS.

---

### Task 4: Settings toggle

**Files:**
- Modify: `frontend/src/types.ts`, `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Type**

In `frontend/src/types.ts`, beside `continuity_checks?: boolean;`:

```ts
  action_echo?: boolean;
```

- [x] **Step 2: Toggle**

In `SettingsStudio.tsx`, inside the `activeSubTab === 'agents'` block's scroll container, before the role-routing card:

```tsx
          <label className="flex items-center justify-between gap-3 p-4 rounded-xl bg-glass-card border border-stone-800 cursor-pointer">
            <span className="text-sm font-sans text-stone-200">
              Restate the player's action
              <span className="block text-xs text-stone-400 font-normal">
                The narrator opens each turn by restating what the player did in the third person.
              </span>
            </span>
            <input
              type="checkbox"
              checked={config.agents.action_echo !== false}
              onChange={(e) => setConfig({ ...config, agents: { ...config.agents, action_echo: e.target.checked } })}
              className="w-4 h-4 accent-purple-600 cursor-pointer"
            />
          </label>
```

- [x] **Step 3: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 5: Verification

- [x] **Step 1: Backend**

Run: `mise run test:backend && mise run lint`
Expected: PASS, clean.

- [x] **Step 2: Frontend**

Run: `mise run test:frontend && mise run build:frontend`
Expected: exit 0.

- [x] **Step 3: Manual**

Play an action turn and confirm the narrator opens by restating it in third person, then resolves it; confirm `Say` and the opening turn are unchanged; toggle the setting off and confirm the echo stops.

---

## Self-Review

**Spec coverage:** name in the action block (Task 2), instruction section (Task 2), config toggle + default on (Task 1), orchestrator gating and wiring (Task 3), Settings toggle (Task 4), tool description (Task 3).

**Placeholder scan:** none.

**Type consistency:** `ActionEcho` is the field name on both `AgentsConfig` and `ContextRequest`; `SetActionEcho` is used by both call sites; `action_echo` is the JSON/YAML key and the frontend field.
