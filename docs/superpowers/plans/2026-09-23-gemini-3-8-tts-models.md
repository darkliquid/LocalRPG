# Gemini 3.8 Flash & Flash-Lite TTS Models Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate Google's newly released `gemini-3.8-flash-tts` and `gemini-3.8-flash-lite-tts` models into LocalRPG's Gemini TTS provider, presets, and Settings Studio UI.

**Architecture:** Add `gemini-3.8-flash-tts` and `gemini-3.8-flash-lite-tts` to `TTSPresets` in `pkg/config/presets.go`, update the default model fallback in `pkg/media/gemini_tts.go` to `gemini-3.8-flash-tts`, add frontend presets in `frontend/src/templates/providerPresets.ts`, and update model pills in `frontend/src/components/SettingsStudio.tsx`.

**Tech Stack:** Go (1.27.1), TypeScript, React 19.

---

### File Map

| Action | File | Responsibility |
|---|---|---|
| Modify | `pkg/config/presets.go` | Add `gemini-3.8-flash-tts` and `gemini-3.8-flash-lite-tts` to `TTSPresets` |
| Modify | `pkg/config/presets_test.go` | Verify both 3.8 presets exist with correct models |
| Modify | `pkg/media/gemini_tts.go` | Update default model fallback to `gemini-3.8-flash-tts` |
| Modify | `frontend/src/templates/providerPresets.ts` | Add 3.8 Flash and Flash-Lite to `TTS_PRESETS` |
| Modify | `frontend/src/components/SettingsStudio.tsx` | Update default fallback and add 3.8 model pills |

---

### Task 1: Backend Presets & Default Model Update

**Files:**
- Modify: `pkg/config/presets.go:150-185`
- Modify: `pkg/config/presets_test.go:120-155`
- Modify: `pkg/media/gemini_tts.go:100-140`

- [x] **Step 1: Write failing test in `pkg/config/presets_test.go`**

Update `TestGetGeminiTTSPresets` in `pkg/config/presets_test.go` to include the 3.8 models:

```go
func TestGetGeminiTTSPresets(t *testing.T) {
	expected := []struct {
		id    string
		model string
	}{
		{"gemini-3.8-flash-tts", "gemini-3.8-flash-tts"},
		{"gemini-3.8-flash-lite-tts", "gemini-3.8-flash-lite-tts"},
		{"gemini-3.1-flash-tts", "gemini-3.1-flash-tts-preview"},
		{"gemini-2.5-flash-tts", "gemini-2.5-flash-preview-tts"},
		{"gemini-2.5-pro-tts", "gemini-2.5-pro-preview-tts"},
	}

	for _, tc := range expected {
		p, ok := config.GetTTSPreset(tc.id)
		if !ok {
			t.Fatalf("expected preset %q to exist in TTSPresets", tc.id)
		}
		if p.Type != "gemini" {
			t.Errorf("preset %q: expected Type 'gemini', got %q", tc.id, p.Type)
		}
		if p.Model != tc.model {
			t.Errorf("preset %q: expected Model %q, got %q", tc.id, tc.model, p.Model)
		}
		if p.DefaultVoice != "Aoede" {
			t.Errorf("preset %q: expected DefaultVoice 'Aoede', got %q", tc.id, p.DefaultVoice)
		}
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestGetGeminiTTSPresets ./pkg/config/`
Expected: FAIL (`expected preset "gemini-3.8-flash-tts" to exist in TTSPresets`)

- [x] **Step 3: Update `pkg/config/presets.go` and `pkg/media/gemini_tts.go`**

In `pkg/config/presets.go`, add to `TTSPresets`:
```go
	"gemini-3.8-flash-tts": {
		Type:         "gemini",
		Model:        "gemini-3.8-flash-tts",
		DefaultVoice: "Aoede",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"gemini-3.8-flash-lite-tts": {
		Type:         "gemini",
		Model:        "gemini-3.8-flash-lite-tts",
		DefaultVoice: "Aoede",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
```

In `pkg/media/gemini_tts.go`:
Update `NewGeminiTTSClientWithClient` and `NewGeminiTTSClientOffline`:
```go
	if model == "" {
		model = "gemini-3.8-flash-tts"
	}
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v -run TestGetGeminiTTSPresets ./pkg/config/`
Run: `go test -v ./pkg/media/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/config/presets.go pkg/config/presets_test.go pkg/media/gemini_tts.go
git commit -m "feat(media): add Gemini 3.8 Flash and Flash-Lite TTS models"
```

---

### Task 2: Frontend Presets & Settings Studio UI

**Files:**
- Modify: `frontend/src/templates/providerPresets.ts`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Add presets to `frontend/src/templates/providerPresets.ts`**

In `frontend/src/templates/providerPresets.ts`, add to `TTS_PRESETS`:
```typescript
  'gemini-3.8-flash-tts': {
    label: 'Google Gemini 3.8 Flash TTS',
    description: 'Most expressive audio model with deep creative direction and character design.',
    config: {
      type: 'gemini',
      model: 'gemini-3.8-flash-tts',
      default_voice: 'Aoede',
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
    },
  },
  'gemini-3.8-flash-lite-tts': {
    label: 'Google Gemini 3.8 Flash-Lite TTS',
    description: 'High-volume, cost-efficient expressive voice generation with low latency.',
    config: {
      type: 'gemini',
      model: 'gemini-3.8-flash-lite-tts',
      default_voice: 'Aoede',
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
    },
  },
```

- [x] **Step 2: Update `frontend/src/components/SettingsStudio.tsx`**

1. Update default model fallback when switching to Gemini TTS:
   `model: config.media.tts.model || 'gemini-3.8-flash-tts'`
2. Update the Gemini TTS model pills to list 3.8 models first:
   ```typescript
   [
     { id: 'gemini-3.8-flash-tts', label: '3.8 Flash TTS' },
     { id: 'gemini-3.8-flash-lite-tts', label: '3.8 Flash-Lite TTS' },
     { id: 'gemini-3.1-flash-tts-preview', label: '3.1 Flash TTS' },
     { id: 'gemini-2.5-flash-preview-tts', label: '2.5 Flash TTS' },
     { id: 'gemini-2.5-pro-preview-tts', label: '2.5 Pro TTS' },
   ]
   ```

- [x] **Step 3: Run frontend typecheck**

Run: `mise run test:frontend`
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/templates/providerPresets.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): add Gemini 3.8 Flash & Flash-Lite TTS presets and pills"
```

---

### Task 3: Full Verification and Build

- [x] **Step 1: Run all tests**

Run: `mise run test`
Expected: PASS

- [x] **Step 2: Run build**

Run: `mise run build`
Expected: SUCCESS

- [x] **Step 3: Verify clean git status (restore dist/.gitkeep if needed)**

Run: `git status`
Expected: Clean working tree

- [x] **Step 4: Mark plan complete and commit**

```bash
git add docs/superpowers/plans/2026-09-23-gemini-3-8-tts-models.md
git commit -m "docs: mark all gemini 3.8 tts models plan tasks complete"
```
