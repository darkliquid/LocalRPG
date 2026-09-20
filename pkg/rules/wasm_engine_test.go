package rules

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/storage"
)

// Minimal valid Wasm module binary that exports a function "check(i32, i32) -> i32"
// checks if param1 >= param0 (1 if true, 0 if false)
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
