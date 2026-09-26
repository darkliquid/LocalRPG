package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/dop251/goja"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

type JSEngine struct {
	mu              sync.Mutex
	vm              *goja.Runtime
	bridge          GameHostAPI
	actionHandlers  map[string]goja.Callable
	turnEndHooks    []goja.Callable
	turnBeginHooks  []goja.Callable
	worldTickHooks  []goja.Callable
	checkResolvers  map[string]goja.Callable
	healthZeroHooks []goja.Callable
}

func NewJSEngine(bridge GameHostAPI) *JSEngine {
	vm := goja.New()
	engine := &JSEngine{
		vm:              vm,
		bridge:          bridge,
		actionHandlers:  make(map[string]goja.Callable),
		turnEndHooks:    make([]goja.Callable, 0),
		turnBeginHooks:  make([]goja.Callable, 0),
		worldTickHooks:  make([]goja.Callable, 0),
		checkResolvers:  make(map[string]goja.Callable),
		healthZeroHooks: make([]goja.Callable, 0),
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

	j.vm.Set("getLocation", func(call goja.FunctionCall) goja.Value {
		locationID, err := j.bridge.GetLocation()
		if err != nil {
			panic(j.vm.ToValue(fmt.Sprintf("getLocation error: %v", err)))
		}
		return j.vm.ToValue(locationID)
	})

	j.vm.Set("setLocation", func(call goja.FunctionCall) goja.Value {
		locationID := call.Argument(0).String()
		if err := j.bridge.SetLocation(locationID); err != nil {
			panic(j.vm.ToValue(fmt.Sprintf("setLocation error: %v", err)))
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

	j.vm.Set("onTurnBegin", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(j.vm.ToValue("onTurnBegin handler must be a function"))
		}
		j.turnBeginHooks = append(j.turnBeginHooks, fn)
		return goja.Undefined()
	})

	j.vm.Set("onCheck", func(call goja.FunctionCall) goja.Value {
		kind := call.Argument(0).String()
		fn, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			panic(j.vm.ToValue("onCheck handler must be a function"))
		}
		j.checkResolvers[kind] = fn
		return goja.Undefined()
	})

	j.vm.Set("onHealthZero", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(j.vm.ToValue("onHealthZero handler must be a function"))
		}
		j.healthZeroHooks = append(j.healthZeroHooks, fn)
		return goja.Undefined()
	})
}

// SetManifest hands the declarative schema to the host bridge if it accepts one.
func (j *JSEngine) SetManifest(manifest *core.SystemManifest) {
	if setter, ok := j.bridge.(interface {
		SetManifest(*core.SystemManifest)
	}); ok {
		setter.SetManifest(manifest)
	}
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

func (j *JSEngine) ExecuteAction(actionType string, ctx map[string]any) (*ActionResult, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	handler, exists := j.actionHandlers[actionType]
	if !exists {
		return nil, nil
	}

	val, err := handler(goja.Undefined(), j.vm.ToValue(ctx))
	if err != nil {
		return nil, fmt.Errorf("execute action %q: %w", actionType, err)
	}

	exported := val.Export()
	if exported == nil {
		return nil, nil
	}

	m, ok := exported.(map[string]any)
	if !ok {
		return &ActionResult{Success: true, Message: fmt.Sprint(exported)}, nil
	}

	res := &ActionResult{
		Data: make(map[string]any),
	}
	if s, ok := m["success"].(bool); ok {
		res.Success = s
	}
	if outcome, ok := m["outcome"].(string); ok {
		// The vocabulary belongs to the system and is stored verbatim; a label alone
		// does not imply success, because the engine never invents semantics.
		res.Outcome = outcome
	}
	if msg, ok := m["message"].(string); ok {
		res.Message = msg
	}
	if r, ok := m["roll"].(*RollResult); ok {
		res.Roll = r
	} else if rMap, ok := m["roll"].(map[string]any); ok {
		data, _ := json.Marshal(rMap)
		var rr RollResult
		json.Unmarshal(data, &rr)
		res.Roll = &rr
	}

	for k, v := range m {
		if k == "success" || k == "outcome" || k == "message" || k == "roll" {
			continue
		}
		res.Data[k] = v
	}

	return res, nil
}

func (j *JSEngine) ExecuteTurnEnd(ctx map[string]any) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	for _, hook := range j.turnEndHooks {
		if _, err := hook(goja.Undefined(), j.vm.ToValue(ctx)); err != nil {
			return fmt.Errorf("execute turnEnd hook: %w", err)
		}
	}
	return nil
}

func (j *JSEngine) ExecuteWorldTick(ctx map[string]any) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	for _, hook := range j.worldTickHooks {
		if _, err := hook(goja.Undefined(), j.vm.ToValue(ctx)); err != nil {
			return fmt.Errorf("execute worldTick hook: %w", err)
		}
	}
	return nil
}

// Resolve implements harness.CheckResolver: a script resolver registered for the
// check kind wins, otherwise the system's declared conventions resolve it.
func (j *JSEngine) Resolve(ctx context.Context, req harness.CheckRequest, actor *entity.Entity) (*harness.CheckResult, error) {
	j.mu.Lock()
	fn, hasResolver := j.checkResolvers[req.CheckKind]
	if hasResolver {
		arg := j.vm.ToValue(map[string]any{
			"actor": req.Actor, "target": req.Target, "check_kind": req.CheckKind,
			"stat": req.Stat, "difficulty": req.Difficulty, "stakes": req.Stakes,
		})
		value, err := fn(goja.Undefined(), arg)
		j.mu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("check resolver %q: %w", req.CheckKind, err)
		}
		result := &harness.CheckResult{CheckID: newCheckID(), Actor: req.Actor, Target: req.Target, Outcome: "fail"}
		if mapped, ok := value.Export().(map[string]any); ok {
			if outcome, ok := mapped["outcome"].(string); ok && outcome != "" {
				result.Outcome = outcome
			}
		}
		return result, nil
	}

	conventions := core.CheckConventions{}
	if schema, ok := j.bridge.(interface {
		CheckConventions() core.CheckConventions
	}); ok {
		conventions = schema.CheckConventions()
	}
	bridge := j.bridge
	j.mu.Unlock()
	return SchemaResolver{bridge: bridge, conventions: conventions}.Resolve(ctx, req, actor)
}

// ExecuteTurnBegin runs every onTurnBegin hook.
func (j *JSEngine) ExecuteTurnBegin(ctx map[string]any) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, hook := range j.turnBeginHooks {
		if _, err := hook(goja.Undefined(), j.vm.ToValue(ctx)); err != nil {
			return fmt.Errorf("turn begin hook: %w", err)
		}
	}
	return nil
}

// HostAPI exposes the host bridge a script was given, so the engine can apply
// state changes through the same path scripts use.
func (j *JSEngine) HostAPI() GameHostAPI { return j.bridge }

// EvaluateHealthZero resolves the declared health-zero effect: the first
// onHealthZero hook that returns text wins, otherwise the free-text effect is
// returned unchanged.
func (j *JSEngine) EvaluateHealthZero(effect string) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, hook := range j.healthZeroHooks {
		value, err := hook(goja.Undefined(), j.vm.ToValue(effect))
		if err != nil {
			return "", fmt.Errorf("health zero hook: %w", err)
		}
		if mapped, ok := value.Export().(string); ok && mapped != "" {
			return mapped, nil
		}
	}
	return effect, nil
}
