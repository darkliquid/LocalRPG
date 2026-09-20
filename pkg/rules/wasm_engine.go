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
