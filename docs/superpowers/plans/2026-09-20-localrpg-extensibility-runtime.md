# LocalRPG Extensibility Runtime & Mechanics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the extensibility runtime for LocalRPG: integrate `github.com/darkliquid/roll` for dice evaluation, provide sandboxed JavaScript (Goja) and WebAssembly (Wazero) runtimes with a unified GameHostAPI, and implement action lifecycle hooks and world system overrides.

**Architecture:** The mechanics runtime is strictly schema-agnostic. A unified `GameHostAPI` interface bridges entity state, dice rolls, and GM directives to both Goja and Wazero. Scripts hook into the turn lifecycle (`onAction`, `onTurnEnd`, `onWorldTick`) to evaluate dice checks and mutate state deterministically.

**Tech Stack:** Go 1.27, `github.com/darkliquid/roll`, `github.com/dop251/goja`, `github.com/tetratelabs/wazero`.

---

### File Structure Map

```text
LocalRPG/
├── cmd/
│   └── localrpg/
│       ├── main.go               # Updated with "roll" subcommand
│       ├── roll.go               # CLI roll handler using darkliquid/roll
│       └── roll_test.go          # CLI roll command tests
├── pkg/
│   └── rules/
│       ├── dice.go               # darkliquid/roll integration & RollResult struct
│       ├── dice_test.go          # Dice notation evaluation tests
│       ├── host_api.go           # GameHostAPI interface & bridge
│       ├── js_engine.go          # Goja JavaScript sandbox & API injection
│       ├── js_engine_test.go     # JS script execution & state mutation tests
│       ├── wasm_engine.go        # Wazero Wasm runtime & Host ABI imports
│       ├── wasm_engine_test.go   # Wasm execution & host function tests
│       ├── lifecycle.go          # Action, turn-end, and world-tick dispatchers
│       ├── lifecycle_test.go     # Pre/post action hook lifecycle tests
│       ├── loader.go             # Loader for system mechanics & world overrides
│       └── loader_test.go        # Multi-tier script composition tests
```

---

### Task 1: Dice Rolling Integration (`darkliquid/roll`)

**Files:**
- Create: `pkg/rules/dice.go`
- Test: `pkg/rules/dice_test.go`

- [x] **Step 1: Write the failing test for Dice Roller**

```go
// pkg/rules/dice_test.go
package rules

import (
	"testing"
)

func TestEvaluateRoll(t *testing.T) {
	// Standard polyhedral roll
	res, err := EvaluateRoll("1d20+5")
	if err != nil {
		t.Fatalf("EvaluateRoll failed: %v", err)
	}
	if res.Total < 6 || res.Total > 25 {
		t.Errorf("expected total in [6, 25], got %d", res.Total)
	}
	if res.Notation != "1d20+5" {
		t.Errorf("expected notation 1d20+5, got %s", res.Notation)
	}

	// Exploding dice
	resExploding, err := EvaluateRoll("2d6!")
	if err != nil {
		t.Fatalf("EvaluateRoll with exploding dice failed: %v", err)
	}
	if resExploding.Total < 2 {
		t.Errorf("unexpected total: %d", resExploding.Total)
	}

	// Invalid notation
	_, err = EvaluateRoll("invalid_notation_xyz")
	if err == nil {
		t.Errorf("expected error on invalid dice notation")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/rules/... -v`  
Expected: FAIL (package/rules not defined)

- [x] **Step 3: Implement Dice Roller**

Write `pkg/rules/dice.go`:
```go
package rules

import (
	"fmt"

	"github.com/darkliquid/roll"
)

type RollResult struct {
	Notation  string `json:"notation"`
	Total     int    `json:"total"`
	Successes int    `json:"successes"`
	RollCount int    `json:"roll_count"`
}

func EvaluateRoll(notation string) (*RollResult, error) {
	program, err := roll.CompileString(notation)
	if err != nil {
		return nil, fmt.Errorf("compile roll %q: %w", notation, err)
	}

	result, err := roll.EvaluateProgram(program)
	if err != nil {
		return nil, fmt.Errorf("evaluate roll %q: %w", notation, err)
	}

	return &RollResult{
		Notation:  notation,
		Total:     result.Total,
		Successes: result.Successes,
		RollCount: len(result.Results),
	}, nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/rules/... -v`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/rules/dice.go pkg/rules/dice_test.go
git commit -m "feat(rules): integrate darkliquid/roll for dice evaluation"
```

---

### Task 2: Unified GameHostAPI & Bridge

**Files:**
- Create: `pkg/rules/host_api.go`

- [x] **Step 1: Write host_api.go interface and default implementation**

Write `pkg/rules/host_api.go`:
```go
package rules

import (
	"fmt"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type ActionResult struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Roll    *RollResult            `json:"roll,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

type GameHostAPI interface {
	Roll(notation string) (*RollResult, error)
	GetStat(entityID string, path string) (interface{}, error)
	SetStat(entityID string, path string, value interface{}) error
	GetEntity(entityID string) (*entity.Entity, error)
	InjectGMDirection(directive string)
	GetDirectives() []string
	Log(message string)
	GetLogs() []string
}

type DefaultHostBridge struct {
	store      *storage.Store
	directives []string
	logs       []string
}

func NewHostBridge(store *storage.Store) *DefaultHostBridge {
	return &DefaultHostBridge{
		store:      store,
		directives: make([]string, 0),
		logs:       make([]string, 0),
	}
}

func (h *DefaultHostBridge) Roll(notation string) (*RollResult, error) {
	return EvaluateRoll(notation)
}

func (h *DefaultHostBridge) GetStat(entityID string, path string) (interface{}, error) {
	ent, err := h.store.GetEntity(entityID)
	if err != nil {
		return nil, fmt.Errorf("entity %q not found: %w", entityID, err)
	}
	if ent.State == nil {
		return nil, nil
	}
	val, ok := ent.State.Get(path)
	if !ok {
		return nil, nil
	}
	return val, nil
}

func (h *DefaultHostBridge) SetStat(entityID string, path string, value interface{}) error {
	ent, err := h.store.GetEntity(entityID)
	if err != nil {
		return fmt.Errorf("entity %q not found: %w", entityID, err)
	}
	if ent.State == nil {
		ent.InitState(make(map[string]interface{}))
	}
	if err := ent.State.Set(path, value); err != nil {
		return fmt.Errorf("set stat %q: %w", path, err)
	}
	return h.store.SaveEntity(ent)
}

func (h *DefaultHostBridge) GetEntity(entityID string) (*entity.Entity, error) {
	return h.store.GetEntity(entityID)
}

func (h *DefaultHostBridge) InjectGMDirection(directive string) {
	h.directives = append(h.directives, directive)
}

func (h *DefaultHostBridge) GetDirectives() []string {
	return h.directives
}

func (h *DefaultHostBridge) Log(message string) {
	h.logs = append(h.logs, message)
}

func (h *DefaultHostBridge) GetLogs() []string {
	return h.logs
}
```

- [x] **Step 2: Run build to verify compilation**

Run: `go test ./pkg/rules/... -v`  
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add pkg/rules/host_api.go
git commit -m "feat(rules): define unified GameHostAPI and default storage bridge"
```

---

### Task 3: Sandboxed JavaScript Runtime (Goja)

**Files:**
- Create: `pkg/rules/js_engine.go`
- Test: `pkg/rules/js_engine_test.go`

- [x] **Step 1: Write failing test for JavaScript Engine**

```go
// pkg/rules/js_engine_test.go
package rules

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestJSEngineExecution(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed player
	player := &entity.Entity{
		ID:   "player",
		Name: "Sean",
		Type: "character",
	}
	player.InitState(map[string]interface{}{"hp": 20})
	store.SaveEntity(player)

	bridge := NewHostBridge(store)
	engine := NewJSEngine(bridge)

	script := `
onAction("attack", function(ctx) {
    var rollRes = roll("1d20+2");
    var currentHP = getStat("player", "hp");
    setStat("player", "hp", currentHP - 5);
    injectGMDirection("The player attacked with roll " + rollRes.total);
    return {
        success: rollRes.total >= 10,
        message: "Attack resolved",
        roll: rollRes
    };
});
`
	if err := engine.LoadScript(script); err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	result, err := engine.ExecuteAction("attack", map[string]interface{}{"target": "goblin"})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	if result == nil || result.Message != "Attack resolved" {
		t.Errorf("unexpected action result: %+v", result)
	}

	// Verify state mutation
	hp, err := bridge.GetStat("player", "hp")
	if err != nil || hp != int64(15) && hp != 15 {
		t.Errorf("expected player hp=15, got %v", hp)
	}

	// Verify GM directive injection
	directives := bridge.GetDirectives()
	if len(directives) != 1 {
		t.Errorf("expected 1 directive, got: %v", directives)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/rules/... -v -run TestJSEngineExecution`  
Expected: FAIL (JSEngine not defined)

- [x] **Step 3: Implement Sandboxed JavaScript Runtime**

Write `pkg/rules/js_engine.go`:
```go
package rules

import (
	"fmt"
	"sync"

	"github.com/dop251/goja"
)

type JSEngine struct {
	mu             sync.Mutex
	vm             *goja.Runtime
	bridge         GameHostAPI
	actionHandlers map[string]goja.Callable
	turnEndHooks   []goja.Callable
	worldTickHooks []goja.Callable
}

func NewJSEngine(bridge GameHostAPI) *JSEngine {
	vm := goja.New()
	engine := &JSEngine{
		vm:             vm,
		bridge:         bridge,
		actionHandlers: make(map[string]goja.Callable),
		turnEndHooks:   make([]goja.Callable, 0),
		worldTickHooks: make([]goja.Callable, 0),
	}

	engine.bindHostAPI()
	return engine
}

func (j *JSEngine) bindHostAPI() {
	j.vm.Set("roll", func(call goja.FunctionCall) goja.Value {
		notation := call.Argument(0).String()
		res, err := j.bridge.Roll(notation)
		if err != nil {
			panic(j.vm.ToValue(fmt.Sprintf("roll error: %v", err)))
		}
		return j.vm.ToValue(res)
	})

	j.vm.Set("getStat", func(call goja.FunctionCall) goja.Value {
		entityID := call.Argument(0).String()
		path := call.Argument(1).String()
		val, err := j.bridge.GetStat(entityID, path)
		if err != nil {
			panic(j.vm.ToValue(fmt.Sprintf("getStat error: %v", err)))
		}
		return j.vm.ToValue(val)
	})

	j.vm.Set("setStat", func(call goja.FunctionCall) goja.Value {
		entityID := call.Argument(0).String()
		path := call.Argument(1).String()
		val := call.Argument(2).Export()
		if err := j.bridge.SetStat(entityID, path, val); err != nil {
			panic(j.vm.ToValue(fmt.Sprintf("setStat error: %v", err)))
		}
		return goja.Undefined()
	})

	j.vm.Set("injectGMDirection", func(call goja.FunctionCall) goja.Value {
		dir := call.Argument(0).String()
		j.bridge.InjectGMDirection(dir)
		return goja.Undefined()
	})

	j.vm.Set("log", func(call goja.FunctionCall) goja.Value {
		msg := call.Argument(0).String()
		j.bridge.Log(msg)
		return goja.Undefined()
	})

	j.vm.Set("onAction", func(call goja.FunctionCall) goja.Value {
		actionType := call.Argument(0).String()
		fn, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			panic(j.vm.ToValue("onAction handler must be a function"))
		}
		j.actionHandlers[actionType] = fn
		return goja.Undefined()
	})

	j.vm.Set("onTurnEnd", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(j.vm.ToValue("onTurnEnd handler must be a function"))
		}
		j.turnEndHooks = append(j.turnEndHooks, fn)
		return goja.Undefined()
	})

	j.vm.Set("onWorldTick", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(j.vm.ToValue("onWorldTick handler must be a function"))
		}
		j.worldTickHooks = append(j.worldTickHooks, fn)
		return goja.Undefined()
	})
}

func (j *JSEngine) LoadScript(script string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	_, err := j.vm.RunString(script)
	if err != nil {
		return fmt.Errorf("run script: %w", err)
	}
	return nil
}

func (j *JSEngine) ExecuteAction(actionType string, ctx map[string]interface{}) (*ActionResult, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	handler, exists := j.actionHandlers[actionType]
	if !exists {
		return nil, nil // No handler registered for this action
	}

	val, err := handler(goja.Undefined(), j.vm.ToValue(ctx))
	if err != nil {
		return nil, fmt.Errorf("execute action %q: %w", actionType, err)
	}

	var res ActionResult
	if err := j.vm.ExportTo(val, &res); err != nil {
		return nil, fmt.Errorf("export action result: %w", err)
	}
	return &res, nil
}

func (j *JSEngine) ExecuteTurnEnd(ctx map[string]interface{}) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	for _, hook := range j.turnEndHooks {
		if _, err := hook(goja.Undefined(), j.vm.ToValue(ctx)); err != nil {
			return fmt.Errorf("execute turnEnd hook: %w", err)
		}
	}
	return nil
}

func (j *JSEngine) ExecuteWorldTick(ctx map[string]interface{}) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	for _, hook := range j.worldTickHooks {
		if _, err := hook(goja.Undefined(), j.vm.ToValue(ctx)); err != nil {
			return fmt.Errorf("execute worldTick hook: %w", err)
		}
	}
	return nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/rules/... -v -run TestJSEngineExecution`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/rules/js_engine.go pkg/rules/js_engine_test.go
git commit -m "feat(rules): implement sandboxed JavaScript runtime via Goja"
```

---

### Task 4: Sandboxed WebAssembly Runtime (Wazero)

**Files:**
- Create: `pkg/rules/wasm_engine.go`
- Test: `pkg/rules/wasm_engine_test.go`

- [x] **Step 1: Write failing test for Wazero Wasm Engine**

```go
// pkg/rules/wasm_engine_test.go
package rules

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/storage"
)

// Minimal valid Wasm module binary that exports a function "calculate_roll(dc, roll) -> bool"
// WAT: (module (func (export "check") (param i32 i32) (result i32) local.get 1 local.get 0 i32.ge_s))
var sampleWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // \0asm v1
	0x01, 0x07, 0x01, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f, // type: (i32, i32) -> i32
	0x03, 0x02, 0x01, 0x00, // func: type 0
	0x07, 0x09, 0x01, 0x05, 0x63, 0x68, 0x65, 0x63, 0x6b, 0x00, 0x00, // export "check"
	0x0a, 0x09, 0x01, 0x07, 0x00, 0x20, 0x01, 0x20, 0x00, 0x4e, 0x0b, // code: local.get 1; local.get 0; i32.ge_s
}

func TestWasmEngineExecution(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	bridge := NewHostBridge(store)
	wasmEngine, err := NewWasmEngine(ctx, bridge)
	if err != nil {
		t.Fatalf("NewWasmEngine failed: %v", err)
	}
	defer wasmEngine.Close(ctx)

	if err := wasmEngine.LoadModule(ctx, "mechanics", sampleWasm); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	res, err := wasmEngine.CallFunction(ctx, "mechanics", "check", 15, 18)
	if err != nil {
		t.Fatalf("CallFunction failed: %v", err)
	}
	if len(res) == 0 || res[0] != 1 {
		t.Errorf("expected 1 (true) since 18 >= 15, got %v", res)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/rules/... -v -run TestWasmEngineExecution`  
Expected: FAIL (WasmEngine not defined)

- [x] **Step 3: Implement Wazero Wasm Runtime**

Write `pkg/rules/wasm_engine.go`:
```go
package rules

import (
	"context"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type WasmEngine struct {
	mu      sync.RWMutex
	runtime wazero.Runtime
	bridge  GameHostAPI
	modules map[string]api.Module
}

func NewWasmEngine(ctx context.Context, bridge GameHostAPI) (*WasmEngine, error) {
	r := wazero.NewRuntime(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	engine := &WasmEngine{
		runtime: r,
		bridge:  bridge,
		modules: make(map[string]api.Module),
	}

	if err := engine.exportHostAPI(ctx); err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("export host api: %w", err)
	}

	return engine, nil
}

func (w *WasmEngine) exportHostAPI(ctx context.Context) error {
	_, err := w.runtime.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, ptr uint32, len uint32) uint64 {
			bytes, ok := mod.Memory().Read(ptr, len)
			if !ok {
				return 0
			}
			res, err := w.bridge.Roll(string(bytes))
			if err != nil {
				return 0
			}
			return uint64(res.Total)
		}).
		Export("host_roll").
		Instantiate(ctx)

	return err
}

func (w *WasmEngine) LoadModule(ctx context.Context, moduleName string, wasmBinary []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	compiled, err := w.runtime.CompileModule(ctx, wasmBinary)
	if err != nil {
		return fmt.Errorf("compile wasm module %q: %w", moduleName, err)
	}

	mod, err := w.runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(moduleName))
	if err != nil {
		return fmt.Errorf("instantiate wasm module %q: %w", moduleName, err)
	}

	w.modules[moduleName] = mod
	return nil
}

func (w *WasmEngine) CallFunction(ctx context.Context, moduleName, funcName string, params ...uint64) ([]uint64, error) {
	w.mu.RLock()
	mod, exists := w.modules[moduleName]
	w.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("module %q not loaded", moduleName)
	}

	fn := mod.ExportedFunction(funcName)
	if fn == nil {
		return nil, fmt.Errorf("function %q not exported in module %q", funcName, moduleName)
	}

	return fn.Call(ctx, params...)
}

func (w *WasmEngine) Close(ctx context.Context) error {
	return w.runtime.Close(ctx)
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/rules/... -v -run TestWasmEngineExecution`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/rules/wasm_engine.go pkg/rules/wasm_engine_test.go
git commit -m "feat(rules): implement sandboxed WebAssembly runtime via Wazero"
```

---

### Task 5: Multi-Tier Script Loader & Composition

**Files:**
- Create: `pkg/rules/loader.go`
- Test: `pkg/rules/loader_test.go`

- [x] **Step 1: Write failing test for Script Loader**

```go
// pkg/rules/loader_test.go
package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestLoaderComposition(t *testing.T) {
	tempDir := t.TempDir()
	paths := core.NewPathResolver(tempDir)

	// Setup system rules
	sysDir := paths.SystemDir("d20")
	os.MkdirAll(sysDir, 0755)
	baseScript := `
onAction("inspect", function(ctx) {
    return { success: true, message: "Inspected from base system" };
});
`
	os.WriteFile(filepath.Join(sysDir, "mechanics.js"), []byte(baseScript), 0644)

	// Setup world overrides
	worldOverrideDir := filepath.Join(paths.WorldDir("fantasy"), "system_overrides", "d20")
	os.MkdirAll(worldOverrideDir, 0755)
	overrideScript := `
onAction("inspect", function(ctx) {
    return { success: true, message: "Inspected from world override" };
});
`
	os.WriteFile(filepath.Join(worldOverrideDir, "hooks.js"), []byte(overrideScript), 0644)

	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	bridge := NewHostBridge(store)
	engine := NewJSEngine(bridge)

	loader := NewRuleLoader(paths, engine)
	if err := loader.LoadRules("d20", "fantasy"); err != nil {
		t.Fatalf("LoadRules failed: %v", err)
	}

	res, err := engine.ExecuteAction("inspect", nil)
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}
	if res.Message != "Inspected from world override" {
		t.Errorf("expected override message, got %q", res.Message)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/rules/... -v -run TestLoaderComposition`  
Expected: FAIL (RuleLoader not defined)

- [x] **Step 3: Implement RuleLoader**

Write `pkg/rules/loader.go`:
```go
package rules

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
)

type RuleLoader struct {
	paths    *core.PathResolver
	jsEngine *JSEngine
}

func NewRuleLoader(paths *core.PathResolver, jsEngine *JSEngine) *RuleLoader {
	return &RuleLoader{
		paths:    paths,
		jsEngine: jsEngine,
	}
}

func (r *RuleLoader) LoadRules(systemID, worldID string) error {
	// 1. Load base system JS if present
	sysScriptPath := filepath.Join(r.paths.SystemDir(systemID), "mechanics.js")
	if data, err := os.ReadFile(sysScriptPath); err == nil {
		if err := r.jsEngine.LoadScript(string(data)); err != nil {
			return fmt.Errorf("load base system script %q: %w", sysScriptPath, err)
		}
	}

	// 2. Load world overrides JS if present
	if worldID != "" {
		overridePath := filepath.Join(r.paths.WorldDir(worldID), "system_overrides", systemID, "hooks.js")
		if data, err := os.ReadFile(overridePath); err == nil {
			if err := r.jsEngine.LoadScript(string(data)); err != nil {
				return fmt.Errorf("load world override script %q: %w", overridePath, err)
			}
		}
	}

	return nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/rules/... -v -run TestLoaderComposition`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/rules/loader.go pkg/rules/loader_test.go
git commit -m "feat(rules): implement multi-tier script loader and world overrides"
```

---

### Task 6: CLI Roll Subcommand (`localrpg roll`)

**Files:**
- Create: `cmd/localrpg/roll.go`
- Modify: `cmd/localrpg/main.go`
- Test: `cmd/localrpg/roll_test.go`

- [x] **Step 1: Write CLI roll test**

```go
// cmd/localrpg/roll_test.go
package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIRollCommand(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "roll", "3d6+4")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v, output: %s", err, string(out))
	}

	outputStr := string(out)
	if !strings.Contains(outputStr, "Roll: 3d6+4") || !strings.Contains(outputStr, "Total:") {
		t.Errorf("unexpected output: %s", outputStr)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/localrpg/... -v -run TestCLIRollCommand`  
Expected: FAIL (subcommand roll not handled)

- [x] **Step 3: Implement CLI Roll Subcommand**

Write `cmd/localrpg/roll.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/darkliquid/localrpg/pkg/rules"
)

func handleRollCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: localrpg roll <notation> (e.g. 1d20+5, 4d6kh3, 3dF)")
		os.Exit(1)
	}

	notation := args[0]
	res, err := rules.EvaluateRoll(notation)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error evaluating roll %q: %v\n", notation, err)
		os.Exit(1)
	}

	fmt.Printf("Roll: %s\n", res.Notation)
	fmt.Printf("Total: %d\n", res.Total)
	if res.Successes > 0 {
		fmt.Printf("Successes: %d\n", res.Successes)
	}
	fmt.Printf("Dice rolled: %d\n", res.RollCount)
}
```

Modify `cmd/localrpg/main.go` to dispatch to `handleRollCommand`:
```go
	switch args[0] {
	case "roll":
		handleRollCommand(args[1:])
	case "version":
		fmt.Printf("LocalRPG v%s\n", Version)
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		printUsage()
		os.Exit(1)
	}
```

- [x] **Step 4: Run all project tests**

Run: `go test ./... -v`  
Expected: All package tests PASS

- [x] **Step 5: Commit**

```bash
git add cmd/localrpg/roll.go cmd/localrpg/main.go cmd/localrpg/roll_test.go
git commit -m "feat(cli): add 'roll' subcommand powered by darkliquid/roll"
```
