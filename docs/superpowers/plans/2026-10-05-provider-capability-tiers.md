# Provider Capability Tiers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Label every provider with an honest tier and caveat, shown in the catalogue, the preset picker, and the docs, with a drift guard so the label cannot lie.

**Architecture:** `provider.Descriptor` gains `Tier` and `Caveat`; a `TierCaveat` helper supplies defaults; `provider.Validate` rejects a tier inconsistent with the declared features; every adapter declares its tier; the frontend renders a `TierBadge`.

**Tech Stack:** Go standard library; React 19 + Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-10-05-provider-capability-tiers-design.md`

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- Every adapter must declare a tier in the same change; the drift guard rejects an empty tier.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The tier vocabulary and default caveats

**Files:**
- Create: `pkg/provider/tier.go`
- Test: `pkg/provider/tier_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `type Tier string`, `TierOfflineBasic`, `TierOfflineNeural`, `TierLocalServer`, `TierCloud`, `func TierCaveat(Tier) string`, `func (Tier) Valid() bool`.

- [ ] **Step 1: Write the failing test**

```go
package provider

import "testing"

func TestTierCaveatIsNonEmptyForEveryTier(t *testing.T) {
	for _, tier := range []Tier{TierOfflineBasic, TierOfflineNeural, TierLocalServer, TierCloud} {
		if !tier.Valid() {
			t.Errorf("%q is not valid", tier)
		}
		if TierCaveat(tier) == "" {
			t.Errorf("%q has no caveat", tier)
		}
	}
	if Tier("nonsense").Valid() {
		t.Error("an unknown tier should be invalid")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/provider/ -run TestTierCaveat -v`
Expected: FAIL, `undefined: TierOfflineBasic`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/provider/tier.go`:

```go
package provider

// Tier is a coarse, user-facing classification of how a provider runs.
type Tier string

const (
	TierOfflineBasic  Tier = "offline-basic"
	TierOfflineNeural Tier = "offline-neural"
	TierLocalServer   Tier = "local-server"
	TierCloud         Tier = "cloud"
)

// Valid reports whether t is one of the known tiers.
func (t Tier) Valid() bool {
	switch t {
	case TierOfflineBasic, TierOfflineNeural, TierLocalServer, TierCloud:
		return true
	default:
		return false
	}
}

// TierCaveat returns the default honest description of a tier.
func TierCaveat(t Tier) string {
	switch t {
	case TierOfflineBasic:
		return "Runs with no model and no network. Deterministic and simple; its output is limited and repetitive next to a model."
	case TierOfflineNeural:
		return "Runs a small model on your CPU with no network. Quality is well below a large local or cloud model."
	case TierLocalServer:
		return "Needs a server you run yourself. Local, but only offline while that server is."
	case TierCloud:
		return "Sends your text to a remote provider and needs an API key. Usually metered."
	}
	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/provider/ -run TestTierCaveat -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/tier.go pkg/provider/tier_test.go
git commit -m "feat(provider): add a capability tier vocabulary"
```

---

### Task 2: The descriptor fields and the drift guard

**Files:**
- Modify: `pkg/provider/descriptor.go:65-74`
- Modify: `pkg/provider/provider.go:78-97` (`Validate`)
- Test: `pkg/provider/provider_test.go` (append)

**Interfaces:**
- Consumes: `Tier` (Task 1).
- Produces: `Descriptor.Tier`, `Descriptor.Caveat`; `Validate` rejects a tier/feature mismatch.

- [ ] **Step 1: Write the failing test**

```go
func TestValidateRejectsTierFeatureMismatch(t *testing.T) {
	cases := []struct {
		name string
		d    Descriptor
	}{
		{"cloud without key", Descriptor{ID: "llm:x", Family: FamilyLLM, Tier: TierCloud}},
		{"offline without offline", Descriptor{ID: "tts:x", Family: FamilyTTS, Tier: TierOfflineBasic}},
		{"empty tier", Descriptor{ID: "stt:x", Family: FamilySTT}},
	}
	for _, c := range cases {
		if err := validateOne(c.d); err == nil {
			t.Errorf("%s: expected an error", c.name)
		}
	}
	if err := validateOne(Descriptor{ID: "tts:x", Family: FamilyTTS, Tier: TierOfflineBasic, Features: []Feature{FeatureOffline}}); err != nil {
		t.Errorf("a consistent descriptor should pass: %v", err)
	}
}
```

Add a `validateOne(Descriptor) error` helper in the test, or refactor `Validate` to expose a
single-descriptor check the test can call.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/provider/ -run TestValidateRejectsTier -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

In `pkg/provider/descriptor.go`, add to `Descriptor`:

```go
	Tier   Tier   `json:"tier"`
	Caveat string `json:"caveat,omitempty"`
```

In `pkg/provider/provider.go`, extend `Validate` (and the per-descriptor helper) so that for each
registered descriptor:

```go
	if !d.Tier.Valid() {
		return fmt.Errorf("provider %q: missing or unknown tier", d.ID)
	}
	has := func(f Feature) bool { /* membership test */ }
	switch d.Tier {
	case TierCloud:
		if !has(FeatureKeyRequired) {
			return fmt.Errorf("provider %q: cloud tier requires key_required", d.ID)
		}
	case TierOfflineBasic, TierOfflineNeural:
		if !has(FeatureOffline) {
			return fmt.Errorf("provider %q: offline tier requires offline", d.ID)
		}
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/provider/ -v`
Expected: PASS (the new tests; existing tests may need their fixtures to gain a tier).

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/descriptor.go pkg/provider/provider.go pkg/provider/provider_test.go
git commit -m "feat(provider): require a consistent tier on every descriptor"
```

---

### Task 3: Declare a tier on every adapter

**Files:**
- Modify: every `pkg/provider/*/…go` that calls `provider.Register` (the adapter packages listed in
  the spec's table).

**Interfaces:**
- Consumes: `Tier` constants (Task 1).
- Produces: no new symbols; each `Descriptor` gains `Tier:`.

- [ ] **Step 1: Add the tier to each descriptor**

For each adapter, add `Tier: provider.TierX` to its `Descriptor` literal, per the spec's table. For
example, `pkg/provider/oracle/oracle.go`:

```go
	Descriptor: provider.Descriptor{
		ID: string(provider.KeyLLMOracle), Family: provider.FamilyLLM,
		Label: "Narrative Oracle", Source: "builtin",
		Tier:   provider.TierOfflineBasic,
		Features: []provider.Feature{provider.FeatureStreaming, provider.FeatureOffline},
	},
```

Set `Caveat` only where the default is wrong (for example `tts:native-os`, whose fallback is a
tone: "Uses an operating-system voice, or a plain tone when none is installed.").

- [ ] **Step 2: Run the drift guard**

Run: `go test ./pkg/provider/all/ -v`
Expected: PASS. The guard fails any adapter whose tier is missing or inconsistent.

- [ ] **Step 3: Run the whole provider tree**

Run: `go test ./pkg/provider/... -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/provider
git commit -m "feat(provider): declare a capability tier for every adapter"
```

---

### Task 4: The catalogue badge

**Files:**
- Create: `frontend/src/components/providers/TierBadge.tsx`
- Modify: the provider catalogue component and `frontend/src/types.ts`
- Test: `frontend/src/components/providers/TierBadge.test.tsx`

**Interfaces:**
- Consumes: `Descriptor.Tier`, `Descriptor.Caveat` (Task 2).
- Produces: `<TierBadge tier caveat />`.

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from "@testing-library/react";
import { TierBadge } from "./TierBadge";

test("labels an offline tier", () => {
  render(<TierBadge tier="offline-basic" caveat="No model." />);
  expect(screen.getByText(/Offline · basic/i)).toBeInTheDocument();
  expect(screen.getByTitle("No model.")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- TierBadge` (in `frontend/`)
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```tsx
const LABELS: Record<string, string> = {
  "offline-basic": "Offline · basic",
  "offline-neural": "Offline · small model",
  "local-server": "Local server",
  "cloud": "Cloud",
};

export function TierBadge({ tier, caveat }: { tier?: string; caveat?: string }) {
  if (!tier) return null;
  const tone =
    tier === "cloud" ? "border-sky-400/40 bg-sky-400/10 text-sky-200"
    : tier === "local-server" ? "border-violet-400/40 bg-violet-400/10 text-violet-200"
    : "border-emerald-400/40 bg-emerald-400/10 text-emerald-200";
  return (
    <span title={caveat}
      className={`rounded-full border px-2 py-0.5 text-[11px] font-medium ${tone}`}>
      {LABELS[tier] ?? tier}
    </span>
  );
}
```

Add `tier?: string; caveat?: string;` to the `ProviderDescriptor` TS type and render `<TierBadge>`
in the catalogue rows.

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test -- TierBadge` and `npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/providers/TierBadge.tsx frontend/src/components/providers/TierBadge.test.tsx frontend/src/types.ts
git commit -m "feat(frontend): badge providers by capability tier"
```

---

### Task 5: The preset picker badge

**Files:**
- Modify: the preset picker component (the one that renders `ProviderPreset`s).

**Interfaces:**
- Consumes: `TierBadge` (Task 4).
- Produces: a badge per preset row.

- [ ] **Step 1: Render the badge**

Where presets are listed, look up the adapter's descriptor from the catalogue (the picker already
loads `/api/providers`) and render `<TierBadge tier caveat />` next to each preset. Replace any
bare "Zero-GPU" label with the tier badge, since the tier is the honest version of that claim.

- [ ] **Step 2: Typecheck**

Run: `npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): show the tier in the preset picker"
```

---

### Task 6: Document the tiers

**Files:**
- Modify: `pkg/gui/docs/05-providers.md`

**Interfaces:**
- Consumes: `provider.TierCaveat` text (Task 1).
- Produces: a "How providers run" section.

- [ ] **Step 1: Add the section**

Add a short section listing the four tiers, their labels, and their caveats, matching the strings in
`tier.go`. Keep the article's tone; the embedded docs are Vale-linted.

- [ ] **Step 2: Lint the docs**

Run: `mise run lint:docs`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/gui/docs/05-providers.md
git commit -m "docs: explain provider capability tiers"
```

---

### Task 7: Verification

- [ ] **Step 1: Full suite and lint**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 2: Confirm the acceptance criteria**

- Every registered provider has a tier.
- The drift guard rejects a cloud descriptor without `key_required` and an offline one without
  `offline`.
- The catalogue and the preset picker show the badge and caveat.
- The docs list the tiers.

- [ ] **Step 3: Commit any residual fixes**

```bash
git add -A
git commit -m "chore: finish capability-tier labelling"
```
