# Kokoro-FastAPI Provider Enhancements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enhance LocalRPG's HTTP TTS provider for Kokoro-FastAPI to accept base URLs (`http://localhost:8880`), dynamically fetch the voice catalog from `/v1/audio/voices`, and leverage advanced features like `allow_voice_tags` and `response_format`.

**Architecture:** Extend `httpTTSClient` in `pkg/media/providers.go` to normalize HTTP endpoints into speech and voices URLs, implement the `media.VoiceCatalog` interface to query `/v1/audio/voices` and parse Kokoro voice metadata, and update presets/frontend to use `http://localhost:8880`.

**Tech Stack:** Go (std `net/http`, `encoding/json`), TypeScript, React, Vite.

---

### File Map
- Modify: `pkg/media/providers.go` (HTTP endpoint resolution, `ListVoices` implementation, Kokoro voice parser, `allow_voice_tags`)
- Modify: `pkg/media/providers_test.go` (Unit tests for endpoint resolution, `Synthesize`, and `ListVoices`)
- Modify: `pkg/config/presets.go` (`kokoro-fastapi` default endpoint)
- Modify: `frontend/src/templates/providerPresets.ts` (`kokoro-fastapi` preset endpoint)
- Modify: `frontend/src/components/SettingsStudio.tsx` (Update placeholder and endpoint hints)

---

### Task 1: HTTP Endpoint Resolution and Speech Synthesis Payload

**Files:**
- Modify: `pkg/media/providers.go:110-165`
- Test: `pkg/media/providers_test.go`

- [ ] **Step 1: Write failing tests for HTTP endpoint resolution and Kokoro synthesis payload**

Add `TestResolveHTTPEndpoints` and `TestHTTPTTSClientSynthesizeKokoro` in `pkg/media/providers_test.go`:

```go
func TestResolveHTTPEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSpeech string
		wantVoices string
	}{
		{
			name:       "bare base url",
			input:      "http://localhost:8880",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "base url with trailing slash",
			input:      "http://localhost:8880/",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "full speech endpoint",
			input:      "http://localhost:8880/v1/audio/speech",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "v1 endpoint",
			input:      "http://localhost:8880/v1",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "alltalk endpoint preserved",
			input:      "http://localhost:7851/api/tts-generate",
			wantSpeech: "http://localhost:7851/api/tts-generate",
			wantVoices: "",
		},
		{
			name:       "empty endpoint",
			input:      "",
			wantSpeech: "",
			wantVoices: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpeech, gotVoices := resolveHTTPEndpoints(tt.input)
			if gotSpeech != tt.wantSpeech {
				t.Errorf("speechURL = %q, want %q", gotSpeech, tt.wantSpeech)
			}
			if gotVoices != tt.wantVoices {
				t.Errorf("voicesURL = %q, want %q", gotVoices, tt.wantVoices)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestResolveHTTPEndpoints ./pkg/media/`
Expected: FAIL with undefined `resolveHTTPEndpoints`.

- [ ] **Step 3: Implement `resolveHTTPEndpoints` and update `httpTTSClient.Synthesize`**

In `pkg/media/providers.go`:
1. Implement `resolveHTTPEndpoints(endpoint string) (speechURL, voicesURL string)`.
2. Update `httpTTSClient.Synthesize` to resolve `speechURL`.
3. If model is `kokoro` or endpoint is Kokoro, include `"allow_voice_tags": true` and `"response_format": "mp3"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestResolveHTTPEndpoints ./pkg/media/`
Expected: PASS.

- [ ] **Step 5: Commit changes**

```bash
git add pkg/media/providers.go pkg/media/providers_test.go
git commit -m "feat(media): add HTTP TTS endpoint resolution and Kokoro speech parameters"
```

---

### Task 2: Dynamic Voice Catalog (`ListVoices`) for HTTP TTS

**Files:**
- Modify: `pkg/media/providers.go`
- Test: `pkg/media/providers_test.go`

- [ ] **Step 1: Write failing tests for `httpTTSClient.ListVoices`**

In `pkg/media/providers_test.go`:
Add `TestHTTPTTSClientListVoices` testing:
- Valid Kokoro-FastAPI `/v1/audio/voices` response parsing
- Enrichment of language, gender, accent, tags, and readable name
- Handling server errors and empty voices

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestHTTPTTSClientListVoices ./pkg/media/`
Expected: FAIL (`ListVoices` not implemented or test fails).

- [ ] **Step 3: Implement `ListVoices` on `httpTTSClient`**

In `pkg/media/providers.go`:
- Implement `ListVoices(ctx context.Context) ([]ProviderVoice, error)`
- If `voicesURL` is empty, return `nil, nil` or error.
- Send GET request with optional `apiKey` Bearer header.
- Decode response handling `{ "voices": [...] }` and raw arrays.
- Helper `parseKokoroVoice(raw voiceItem) ProviderVoice` extracting `id`, `name`, `gender`, `language`, `accent`, `tags`, etc.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestHTTPTTSClientListVoices ./pkg/media/`
Expected: PASS.

- [ ] **Step 5: Commit changes**

```bash
git add pkg/media/providers.go pkg/media/providers_test.go
git commit -m "feat(media): implement VoiceCatalog for Kokoro-FastAPI HTTP TTS"
```

---

### Task 3: Update Presets and Frontend Templates

**Files:**
- Modify: `pkg/config/presets.go`
- Modify: `frontend/src/templates/providerPresets.ts`
- Modify: `frontend/src/components/SettingsStudio.tsx`
- Modify: `~/.config/localrpg/config.yaml` (if present)

- [ ] **Step 1: Update backend and frontend presets**

In `pkg/config/presets.go`:
Change `kokoro-fastapi` endpoint from `"http://localhost:8880/v1/audio/speech"` to `"http://localhost:8880"`.

In `frontend/src/templates/providerPresets.ts`:
Change `kokoro-fastapi` endpoint from `'http://localhost:8880/v1/audio/speech'` to `'http://localhost:8880'`.

In `frontend/src/components/SettingsStudio.tsx`:
Update placeholder for speech endpoint from `"e.g. http://localhost:8880/v1/audio/speech"` to `"e.g. http://localhost:8880"`.

- [ ] **Step 2: Run backend and frontend tests and lint**

Run:
`mise run test:backend`
`mise run test:frontend`
`mise run lint`

- [ ] **Step 3: Commit changes**

```bash
git add pkg/config/presets.go frontend/src/templates/providerPresets.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(config): update kokoro-fastapi presets to base url"
```

---

### Task 4: End-to-End Verification with Live Local Kokoro-FastAPI

**Files:**
- Test against live `http://localhost:8880`

- [ ] **Step 1: Verify InspectTTS against live Kokoro-FastAPI**

Run automated test or test script verifying `Service.InspectTTS` with `http://localhost:8880`:
- Returns `Catalog.Available == true`
- Returns > 50 voices from live instance
- Returns proper `ProviderKey == "http:localhost:8880"`

- [ ] **Step 2: Full project build & test verification**

Run:
`mise run test`
`mise run build`
Verify `pkg/gui/dist/.gitkeep` is intact.

- [ ] **Step 3: Final branch review & merge**
