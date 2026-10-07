# Provider Instance Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a provider configuration carry an optional instance id so two configs of one adapter at one endpoint stay distinct in usage, pricing, and caches.

**Architecture:** Each provider config struct gains `Instance`; one `provider.InstanceDiscriminator` helper prefers it over the endpoint/command derivation; the four resolvers thread it into `provider.InstanceOrSelf`; `Config.Validate` rejects a malformed or duplicate id.

**Tech Stack:** Go standard library; `gopkg.in/yaml.v3` for config.

**Spec:** `docs/superpowers/specs/2026-10-05-provider-instance-identity-design.md`

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- A config with no `instance` must produce exactly the key it produced before.
- The discriminator grammar is `^[a-z0-9.:-]+$` (`pkg/provider/key.go:18`).
- Conventional Commits, subject under 72 chars.

---

### Task 1: The `InstanceDiscriminator` helper

**Files:**
- Modify: `pkg/provider/key.go`
- Test: `pkg/provider/key_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces: `func InstanceDiscriminator(instance, derived string) string`.

- [ ] **Step 1: Write the failing test**

```go
func TestInstanceDiscriminator(t *testing.T) {
	if got := InstanceDiscriminator("", "localhost:8880"); got != "localhost:8880" {
		t.Fatalf("derived = %q", got)
	}
	if got := InstanceDiscriminator("narrator", "localhost:8880"); got != "narrator" {
		t.Fatalf("instance = %q", got)
	}
	if got := InstanceDiscriminator("  ", "x"); got != "x" {
		t.Fatalf("blank instance should fall back, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/provider/ -run TestInstanceDiscriminator -v`
Expected: FAIL, `undefined: InstanceDiscriminator`.

- [ ] **Step 3: Write minimal implementation**

Append to `pkg/provider/key.go`:

```go
// InstanceDiscriminator returns the user-chosen instance id when set, else the
// derived discriminator for the configuration's transport.
func InstanceDiscriminator(instance, derived string) string {
	if s := strings.TrimSpace(instance); s != "" {
		return s
	}
	return derived
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/provider/ -run TestInstanceDiscriminator -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/key.go pkg/provider/key_test.go
git commit -m "feat(provider): prefer a user-chosen instance discriminator"
```

---

### Task 2: The config field

**Files:**
- Modify: `pkg/config/types.go` (`AgentRoleConfig`, `TTSConfig`, `STTConfig`, `ImageConfig`, `EmbeddingProviderConfig`)
- Test: `pkg/config/types_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces: `Instance string` on each struct, YAML `instance`.

- [ ] **Step 1: Write the failing test**

```go
func TestInstanceRoundTrips(t *testing.T) {
	in := []byte("agents:\n  roles:\n    gm:\n      type: http\n      endpoint: http://x\n      instance: good\n")
	var cfg Config
	if err := yaml.Unmarshal(in, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Agents.Roles["gm"].Instance != "good" {
		t.Fatalf("instance = %q", cfg.Agents.Roles["gm"].Instance)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -run TestInstanceRoundTrips -v`
Expected: FAIL, `Instance` unknown.

- [ ] **Step 3: Write minimal implementation**

Add the field (with the spec's doc comment) to `AgentRoleConfig`, `TTSConfig`, `STTConfig`,
`ImageConfig`, and `EmbeddingProviderConfig`:

```go
	// Instance is an optional user-chosen discriminator for this provider
	// configuration. It becomes the key's "@<instance>" segment. Empty means the
	// discriminator is derived from the endpoint or command.
	Instance string `yaml:"instance,omitempty" json:"instance,omitempty"`
```

Add `Instance?: string;` to the matching TS types in `frontend/src/types.ts`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -run TestInstanceRoundTrips -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/types_test.go frontend/src/types.ts
git commit -m "feat(config): add an optional provider instance id"
```

---

### Task 3: The LLM resolver honours it

**Files:**
- Modify: `pkg/harness/exports.go:9-35` (`KeyFor`)
- Test: `pkg/harness/exports_test.go` (append)

**Interfaces:**
- Consumes: `provider.InstanceDiscriminator` (Task 1), `AgentRoleConfig.Instance` (Task 2).
- Produces: `KeyFor` returns the instance key when set.

- [ ] **Step 1: Write the failing test**

```go
func TestKeyForPrefersInstance(t *testing.T) {
	base := ProviderConfig{Type: "http", Endpoint: "https://api.openai.com"}
	a, _ := KeyFor(base)
	base.Instance = "good"
	b, _ := KeyFor(base)
	if a == b {
		t.Fatalf("instance did not change the key: %s", a)
	}
	if b != "llm:openaichat@good" {
		t.Fatalf("key = %s", b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/harness/ -run TestKeyForPrefersInstance -v`
Expected: FAIL (the instance is ignored).

- [ ] **Step 3: Write minimal implementation**

`KeyFor` receives a `ProviderConfig` (`pkg/harness/types.go:126-140`); add `Instance` to that struct
too if it does not already carry it, and change each `InstanceOrSelf(parent, derived)` call to:

```go
	provider.InstanceOrSelf(parent, provider.InstanceDiscriminator(cfg.Instance, provider.HostDiscriminator(cfg.Endpoint)))
```

with the command case using `CommandDiscriminator(cfg.Command)`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/harness/ -v`
Expected: PASS, including the existing key tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/harness/exports.go pkg/harness/types.go pkg/harness/exports_test.go
git commit -m "feat(harness): honour a provider instance id in the key"
```

---

### Task 4: The media resolvers honour it

**Files:**
- Modify: `pkg/media/exports.go:22-186` (`TTSKeyFor`, `STTKeyFor`, `ImageKeyFor`)
- Test: `pkg/media/exports_test.go` (append)

**Interfaces:**
- Consumes: `provider.InstanceDiscriminator`, the config fields (Task 2).
- Produces: the three resolvers return the instance key when set.

- [ ] **Step 1: Write the failing test**

```go
func TestTTSKeyForPrefersInstance(t *testing.T) {
	cfg := config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs", Instance: "narrator"}
	k, ok := TTSKeyFor(cfg)
	if !ok || k != "tts:elevenlabs@narrator" {
		t.Fatalf("key = %s ok = %v", k, ok)
	}
	plain := config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"}
	k2, _ := TTSKeyFor(plain)
	if k2 != "tts:elevenlabs" {
		t.Fatalf("plain key = %s", k2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/media/ -run TestTTSKeyForPrefersInstance -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In each of `TTSKeyFor`, `STTKeyFor`, `ImageKeyFor`, wrap the derived discriminator with
`provider.InstanceDiscriminator(cfg.Instance, derived)` before `provider.InstanceOrSelf`. For a
`builtin` adapter with no derived discriminator, the instance is used directly.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/exports.go pkg/media/exports_test.go
git commit -m "feat(media): honour a provider instance id in the key"
```

---

### Task 5: The embedding resolver honours it

**Files:**
- Modify: `pkg/embeddings/factory.go:85-110` (`KeyFor`)
- Test: `pkg/embeddings/factory_test.go` (append)

**Interfaces:**
- Consumes: `provider.InstanceDiscriminator`, `EmbeddingProviderConfig.Instance` (Task 2).
- Produces: `KeyFor` returns the instance key when set.

- [ ] **Step 1: Write the failing test**

```go
func TestEmbeddingKeyForPrefersInstance(t *testing.T) {
	cfg := config.EmbeddingProviderConfig{Type: "http", URL: "http://localhost:11434", Instance: "local-a"}
	k := KeyFor(cfg)
	if k != "embedding:openai@local-a" {
		t.Fatalf("key = %s", k)
	}
}
```

Adjust the expected adapter to whatever `KeyFor` maps `http` to today.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/embeddings/ -run TestEmbeddingKeyForPrefersInstance -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Thread `provider.InstanceDiscriminator(cfg.Instance, derived)` into the `InstanceOrSelf` call.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/embeddings/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/embeddings/factory.go pkg/embeddings/factory_test.go
git commit -m "feat(embeddings): honour a provider instance id in the key"
```

---

### Task 6: Validation

**Files:**
- Modify: `pkg/config/types.go` (`Config.Validate`)
- Test: `pkg/config/types_test.go` (append)

**Interfaces:**
- Consumes: `provider.ParseKey` (to reuse the discriminator grammar), the config fields (Task 2).
- Produces: validation errors for a malformed or duplicate instance.

- [ ] **Step 1: Write the failing test**

```go
func TestValidateRejectsBadInstances(t *testing.T) {
	bad := DefaultConfig()
	bad.Agents.Roles["gm"] = AgentRoleConfig{Type: "http", Endpoint: "http://x", Instance: "Bad Id"}
	if len(bad.Validate()) == 0 {
		t.Fatal("a malformed instance should be rejected")
	}
	dup := DefaultConfig()
	dup.Agents.Roles["gm"] = AgentRoleConfig{Type: "http", Endpoint: "http://x", Instance: "same"}
	dup.Agents.Roles["narrator"] = AgentRoleConfig{Type: "http", Endpoint: "http://y", Instance: "same"}
	if len(dup.Validate()) == 0 {
		t.Fatal("a duplicate instance should be rejected")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/config/ -run TestValidateRejectsBadInstances -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In `Config.Validate`, add a check that each set `Instance` matches the discriminator grammar and
that no two configurations of the same family share one. Build the key with
`provider.ParseKey("llm:x@"+instance)` (or a small local regexp mirroring
`discriminatorPattern`) to validate the grammar, and collect instances per family into a map,
reporting the two config paths on a duplicate. The `config` package already imports
`pkg/provider`? If not, a local regexp avoids a new dependency; prefer the local regexp to keep
`pkg/config` a leaf.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/config/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/types_test.go
git commit -m "feat(config): validate provider instance ids"
```

---

### Task 7: Verification

- [ ] **Step 1: Regression guard**

Add a test asserting that a config with no `instance` produces exactly the key it produced before
this change (compare against a literal, e.g. `tts:http@localhost:8880` and
`llm:openaichat@api.openai.com`).

- [ ] **Step 2: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 3: Confirm the acceptance criteria**

- Two same-endpoint configs with different instances get different keys.
- An unset instance is byte-identical to today.
- A malformed or duplicate instance is a validation error.
- The usage ledger and pricing now separate the two instances.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: guard provider keys against the unset-instance case"
```
