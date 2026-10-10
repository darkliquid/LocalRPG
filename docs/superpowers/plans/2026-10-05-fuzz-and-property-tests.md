# Fuzz and Property Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fuzz the parser and the repair, and property-test the group planner and the caption timing, so the epic's edge cases fail CI.

**Architecture:** Standard-library fuzz targets with committed seeds, property tests over structured random inputs, and a bounded CI step plus a longer local task.

**Tech Stack:** Go standard library (`testing`, `testing/fuzz`).

**Spec:** `docs/superpowers/specs/2026-10-05-fuzz-and-property-tests-design.md`
**Depends on:** RB-1, RB-2.

## Global Constraints

- Go tests use the standard library only; no testify.
- Use `any`, not `interface{}`, in Go.
- Invariants are the real contracts; loosen one only with a reason.
- A fuzz finding is a bug to fix, with its seed committed.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The parser fuzz

**Files:**
- Create: `pkg/turnstream/fuzz_test.go`

**Interfaces:**
- Consumes: `NewParser`, `Feed`, `Flush`, `Events`.
- Produces: `FuzzParser`.

- [ ] **Step 1: Write the target**

```go
package turnstream

import "testing"

func FuzzParser(f *testing.F) {
	f.Add("A quiet hall.\n")
	f.Add("> Garrick: \"Keep walking.\"\n")
	f.Add("@roll {\"actor\":\"x\"\n}\n")
	f.Add("@roll {bad json\n")
	f.Add("")
	f.Fuzz(func(t *testing.T, data string) {
		p := NewParser(testRoster{})
		p.Feed(data)
		_ = p.Flush()
		events := p.Events()
		second := NewParser(testRoster{})
		second.Feed(data)
		_ = second.Flush()
		if len(events) != len(second.Events()) {
			t.Fatal("parsing is not deterministic")
		}
		for _, ev := range events {
			switch ev.Kind {
			case KindNarration, KindSpeech:
			case KindRecord:
				if ev.Record == nil {
					t.Fatal("a record event has no record")
				}
			default:
				t.Fatalf("unknown event kind %q", ev.Kind)
			}
			if ev.Kind == KindSpeech && ev.Speaker == "" && ev.SpeakerID == "" {
				t.Fatal("a speech event has no speaker")
			}
		}
	})
}
```

- [ ] **Step 2: Run it briefly**

Run: `go test ./pkg/turnstream/ -run FuzzParser -fuzz FuzzParser -fuzztime 15s`
Expected: no failures; fix any finding and commit its seed.

- [ ] **Step 3: Commit**

```bash
git add pkg/turnstream/fuzz_test.go pkg/turnstream/testdata
git commit -m "test(turnstream): fuzz the parser"
```

---

### Task 2: The repair fuzz extension

**Files:**
- Modify: `pkg/jsonrepair/fuzz_test.go` (RB-1's target)

**Interfaces:**
- Consumes: `Repair`, `BraceDepth`.
- Produces: extended invariants.

- [ ] **Step 1: Extend the invariants**

```go
func FuzzRepair(f *testing.F) {
	f.Add([]byte(`{"a":1}`))
	f.Add([]byte("```json\n{}\n```"))
	f.Add([]byte(`{"a":1`))
	f.Fuzz(func(t *testing.T, in []byte) {
		res := Repair(in)
		if res.OK && !json.Valid(res.Payload) {
			t.Fatalf("OK but invalid: %q", res.Payload)
		}
		if res.OK {
			again := Repair(res.Payload)
			if !again.OK || !bytes.Equal(again.Payload, res.Payload) {
				t.Fatalf("repair is not idempotent: %q -> %q", res.Payload, again.Payload)
			}
		}
		_ = BraceDepth(in)
	})
}
```

- [ ] **Step 2: Run it briefly**

Run: `go test ./pkg/jsonrepair/ -run FuzzRepair -fuzz FuzzRepair -fuzztime 15s`
Expected: no failures.

- [ ] **Step 3: Commit**

```bash
git add pkg/jsonrepair
git commit -m "test(jsonrepair): extend the repair fuzz invariants"
```

---

### Task 3: The group planner property test

**Files:**
- Modify: `pkg/media/groupstream_test.go`
- Test: `pkg/media/groupstream_test.go` (append)

**Interfaces:**
- Consumes: `GroupFolder`, `GroupPlan`, RB-2's segment-index sets.
- Produces: a property test over random segment sequences.

- [ ] **Step 1: Write the property test**

```go
func TestGroupFoldEqualsPlanProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 500; i++ {
		segs := randomSegments(rng) // 0-30 segments, random speakers and lengths
		plan := planGroups(segs, narrator, voiceFor, caps)
		fold := foldSegments(segs, caps) // GroupFolder with budget 0
		if !sameGroups(plan, fold) {
			t.Fatalf("fold != plan for %+v", segs)
		}
		if !sameSegmentSets(plan, fold) {
			t.Fatalf("segment sets differ for %+v", segs)
		}
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./pkg/media/ -run TestGroupFoldEqualsPlanProperty -v`
Expected: PASS, or a bug to fix.

- [ ] **Step 3: Commit**

```bash
git add pkg/media/groupstream_test.go
git commit -m "test(media): property-test the group fold against the plan"
```

---

### Task 4: The caption property test

**Deferred:** the caption/WebVTT surface is TH-4, which is not implemented yet, so there is nothing to
property-test. The spec's caption property is "where cheap"; revisit this task when TH-4 lands.

**Files:**
- Modify: `pkg/scene/captions_test.go`
- Test: `pkg/scene/captions_test.go` (append)

**Interfaces:**
- Consumes: `Captions`, the theatre's caption rule (TH-4).
- Produces: a property test over random beat sequences.

- [ ] **Step 1: Write the property test**

```go
func TestCaptionCueMatchesTheBeatProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 200; i++ {
		beats := randomBeats(rng)
		vtt := Captions(beats)
		for _, b := range beats {
			if b.Kind == SegmentSpeech {
				if !strings.Contains(vtt, b.Text) {
					t.Fatalf("a spoken beat has no cue: %+v", b)
				}
			}
		}
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./pkg/scene/ -run TestCaptionCueMatches -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/scene/captions_test.go
git commit -m "test(scene): property-test the caption cues"
```

---

### Task 5: CI and the local task

**Files:**
- Modify: `mise.toml`, `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: the targets (Tasks 1-4).
- Produces: a bounded CI step and a longer local task.

- [ ] **Step 1: Add the mise task**

```toml
[tasks."test:fuzz"]
description = "Run the fuzz targets for a longer budget"
run = """
go test ./pkg/turnstream/ -run FuzzParser -fuzz FuzzParser -fuzztime 60s
go test ./pkg/jsonrepair/ -run FuzzRepair -fuzz FuzzRepair -fuzztime 60s
"""
```

- [ ] **Step 2: Add the CI step**

In the Go job, run each fuzz target with `-fuzztime 10s`, so the committed seeds and a short fuzz pass
gate the build.

- [ ] **Step 3: Verify**

Run: `mise run test:fuzz`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add mise.toml .github/workflows/ci.yml
git commit -m "ci: run the fuzz targets with a bounded budget"
```

---

### Task 6: Verification

- [ ] **Step 1: Full suite**

Run: `mise run test`
Expected: PASS.

- [ ] **Step 2: Confirm the acceptance criteria**

- The parser and the repair are fuzzed with committed seeds.
- The group planner and the captions are property-tested.
- CI runs the targets with a bounded budget.
- A finding is fixed and its seed committed.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "test: finalise the fuzz and property coverage"
```
