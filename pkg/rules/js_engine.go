package rules

import (
	"encoding/json"
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

	m, ok := exported.(map[string]interface{})
	if !ok {
		return &ActionResult{Success: true, Message: fmt.Sprint(exported)}, nil
	}

	res := &ActionResult{
		Data: make(map[string]interface{}),
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
	} else if rMap, ok := m["roll"].(map[string]interface{}); ok {
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
