# Draining the Host Bridge's Directives Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A directive a hook injects reaches the turn that asked for it and no later turn.
**Architecture:** `HostAPI` carries `DrainDirectives`, which returns the injected directives and clears them; the orchestrator calls it where it called `GetDirectives`.
**Tech stack:** Go 1.27, standard-library tests.
**Spec:** `docs/superpowers/specs/2026-10-16-directive-drain-design.md`
**Bead:** LocalRPG-2j4
**Branch:** `fix/directive-drain`

## File Map

| File | Why |
|---|---|
| `pkg/rules/host_api.go` | the interface and the campaign bridge |
| `pkg/systemtest/bridge.go` | the scenario bridge |
| `pkg/engine/mechanics_engagement.go` | the orchestrator's drain |
| `pkg/engine/mechanics_engagement_test.go` | the regression test |
| `pkg/rules/js_engine_test.go` | reads through the drain |

## Task 1: The drain

- [ ] `HostAPI`: replace `GetDirectives` with `DrainDirectives`.
- [ ] `DefaultHostBridge.DrainDirectives` and `systemtest.Bridge.DrainDirectives` return the slice and
      reset it, each with the reason written beside it.
- [ ] `TurnOrchestrator.drainDirectives` calls it.

## Task 2: The regression test

- [ ] `TestAHookDirectiveIsDrainedOnce`: a world-tick hook's directive comes out of the first drain and
      not out of the second.
- [ ] `pkg/rules/js_engine_test.go` reads through the drain.

## Task 3: Gates

- [ ] `gofmt -l`, `go vet ./...`, `go test ./pkg/rules/... ./pkg/engine/... ./pkg/systemtest/...`.
- [ ] `mise run test`, `mise run lint`.
- [ ] Open the pull request, wait for CI, close the bead.

