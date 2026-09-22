# Settings TTS Options and Model Download Prompt Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Disambiguate TTS provider options in Settings Studio, add live Kokoro model status and preview interception with the download modal, and provide 25-voice Kokoro autofill with accent and gender tags.

**Architecture:** 
- Backend: Update `pkg/config/presets.go` with 25-voice `KokoroVoiceProfiles` and `pkg/gui/service.go:TestProvider` with canonical model path fallback and structured `model_missing` DTO response.
- Frontend: Update `frontend/src/templates/providerPresets.ts` with `KOKORO_VOICE_PROFILES` and `TTS_PRESETS["sherpa-onnx"]`. In `SettingsStudio.tsx`, flatten the TTS engine select into explicit top-level options, add live Kokoro model status badge/button, intercept synthesis tests and archetype previews to pop up `ModelDownloadModal`, and add Kokoro voice profiles autofill.

**Tech Stack:** Go (1.27.1), React 19, TypeScript, Tailwind CSS v4, Server-Sent Events (SSE).

---

### Task 1: Backend Presets & Diagnostics DTO

**Files:**
- Modify: `pkg/config/presets.go`
- Modify: `pkg/config/presets_test.go`
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/service_test.go`

- [ ] **Step 1: Write unit test for `KokoroVoiceProfiles` in `pkg/config/presets_test.go`**

Add test checking that `KokoroVoiceProfiles` has all 25 voices and tags include gender and accent:
```go
func TestKokoroVoiceProfilesPreset(t *testing.T) {
	if len(KokoroVoiceProfiles) != 25 {
		t.Fatalf("expected 25 Kokoro voice profiles, got %d", len(KokoroVoiceProfiles))
	}
	for _, p := range KokoroVoiceProfiles {
		if p.ID == "" || p.VoiceID == "" || len(p.Tags) == 0 {
			t.Errorf("invalid profile: %+v", p)
		}
	}

	preset, ok := TTSPresets["sherpa-onnx"]
	if !ok {
		t.Fatal("missing sherpa-onnx preset")
	}
	if len(preset.VoiceProfiles) != 25 {
		t.Errorf("expected sherpa-onnx preset to have 25 voice profiles, got %d", len(preset.VoiceProfiles))
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

Run: `go test -v -run TestKokoroVoiceProfilesPreset ./pkg/config/`
Expected: FAIL with undefined `KokoroVoiceProfiles`.

- [ ] **Step 3: Implement `KokoroVoiceProfiles` in `pkg/config/presets.go`**

Add all 25 Kokoro profiles with derived gender and accent tags and descriptions:
```go
var KokoroVoiceProfiles = []VoiceProfile{
	{ID: "af_alloy", Name: "Alloy (American Female)", VoiceID: "af_alloy", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "alloy", "clear", "neutral"}, Description: "American female voice, neutral, balanced, and articulate."},
	{ID: "af_aoede", Name: "Aoede (American Female)", VoiceID: "af_aoede", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "aoede", "melodic", "expressive"}, Description: "American female voice, musical, dramatic, and expressive."},
	{ID: "af_bella", Name: "Bella (American Female)", VoiceID: "af_bella", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "bella", "warm", "friendly"}, Description: "American female voice, warm, approachable, and pleasant."},
	{ID: "af_heart", Name: "Heart (American Female)", VoiceID: "af_heart", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "heart", "calm", "gentle"}, Description: "American female voice, soft-spoken, comforting, and calm."},
	{ID: "af_jessica", Name: "Jessica (American Female)", VoiceID: "af_jessica", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "jessica", "bright", "conversational"}, Description: "American female voice, energetic, clear, and conversational."},
	{ID: "af_kore", Name: "Kore (American Female)", VoiceID: "af_kore", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "kore", "mystical", "soft"}, Description: "American female voice, ethereal, gentle, and quiet."},
	{ID: "af_nicole", Name: "Nicole (American Female)", VoiceID: "af_nicole", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "nicole", "youthful", "energetic"}, Description: "American female voice, brisk, youthful, and direct."},
	{ID: "af_nova", Name: "Nova (American Female)", VoiceID: "af_nova", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "nova", "dynamic", "sharp"}, Description: "American female voice, focused, sharp, and confident."},
	{ID: "af_river", Name: "River (American Female)", VoiceID: "af_river", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "river", "smooth", "casual"}, Description: "American female voice, smooth, relaxed, and natural."},
	{ID: "af_sarah", Name: "Sarah (American Female)", VoiceID: "af_sarah", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "sarah", "poised", "narrative"}, Description: "American female voice, polished, measured, and story-oriented."},
	{ID: "af_sky", Name: "Sky (American Female)", VoiceID: "af_sky", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "female", "sky", "light", "airy"}, Description: "American female voice, light, gentle, and breathy."},
	{ID: "am_adam", Name: "Adam (American Male)", VoiceID: "am_adam", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "adam", "deep", "authoritative"}, Description: "American male voice, deep, steady, and commanding."},
	{ID: "am_echo", Name: "Echo (American Male)", VoiceID: "am_echo", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "echo", "resonant", "neutral"}, Description: "American male voice, resonant, clear, and balanced."},
	{ID: "am_eric", Name: "Eric (American Male)", VoiceID: "am_eric", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "eric", "grounded", "steady"}, Description: "American male voice, solid, plainspoken, and trustworthy."},
	{ID: "am_fenrir", Name: "Fenrir (American Male)", VoiceID: "am_fenrir", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "fenrir", "fierce", "husky"}, Description: "American male voice, rough, intense, and gravelly."},
	{ID: "am_liam", Name: "Liam (American Male)", VoiceID: "am_liam", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "liam", "warm", "relatable"}, Description: "American male voice, youthful, warm, and friendly."},
	{ID: "am_michael", Name: "Michael (American Male)", VoiceID: "am_michael", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "michael", "commanding", "formal"}, Description: "American male voice, disciplined, authoritative, and formal."},
	{ID: "am_onyx", Name: "Onyx (American Male)", VoiceID: "am_onyx", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "onyx", "dark", "gravelly"}, Description: "American male voice, deep, shadowy, and solemn."},
	{ID: "am_puck", Name: "Puck (American Male)", VoiceID: "am_puck", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"american", "male", "puck", "playful", "mischievous"}, Description: "American male voice, spirited, upbeat, and sly."},
	{ID: "bf_alice", Name: "Alice (British Female)", VoiceID: "bf_alice", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "alice", "articulate", "refined"}, Description: "British female voice, cultured, articulate, and poised."},
	{ID: "bf_emma", Name: "Emma (British Female)", VoiceID: "bf_emma", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "emma", "gentle", "poised"}, Description: "British female voice, elegant, gentle, and softly spoken."},
	{ID: "bf_isabella", Name: "Isabella (British Female)", VoiceID: "bf_isabella", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "isabella", "noble", "melodic"}, Description: "British female voice, aristocratic, melodic, and graceful."},
	{ID: "bf_lily", Name: "Lily (British Female)", VoiceID: "bf_lily", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "female", "lily", "sweet", "youthful"}, Description: "British female voice, sweet, youthful, and crisp."},
	{ID: "bm_daniel", Name: "Daniel (British Male)", VoiceID: "bm_daniel", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "daniel", "scholarly", "calm"}, Description: "British male voice, scholarly, calm, and deliberate."},
	{ID: "bm_fable", Name: "Fable (British Male)", VoiceID: "bm_fable", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "fable", "dramatic", "storyteller"}, Description: "British male voice, theatrical, expressive, and storied."},
	{ID: "bm_george", Name: "George (British Male)", VoiceID: "bm_george", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "george", "mature", "distinguished"}, Description: "British male voice, mature, distinguished, and resonant."},
	{ID: "bm_lewis", Name: "Lewis (British Male)", VoiceID: "bm_lewis", Pitch: 1.0, SpeechRate: 1.0, Tags: []string{"british", "male", "lewis", "thoughtful", "refined"}, Description: "British male voice, measured, polite, and reflective."},
}
```
Update `TTSPresets["sherpa-onnx"]`:
```go
	"sherpa-onnx": {
		Type:          "builtin",
		BuiltinName:   "sherpa-onnx",
		DefaultVoice:  "af_bella",
		Pitch:         1.0,
		SpeechRate:    1.0,
		MasterVolume:  1.0,
		VoiceProfiles: KokoroVoiceProfiles,
	},
```

- [ ] **Step 4: Update `TestProviderResponseDTO` in `pkg/gui/types.go`**

Add `ModelMissing` and `ModelID`:
```go
type TestProviderResponseDTO struct {
	Success      bool   `json:"success"`
	LatencyMS    int64  `json:"latency_ms"`
	Message      string `json:"message"`
	Preview      string `json:"preview,omitempty"`
	AudioDataURI string `json:"audio_data_uri,omitempty"`
	ModelMissing bool   `json:"model_missing,omitempty"`
	ModelID      string `json:"model_id,omitempty"`
}
```

- [ ] **Step 5: Write unit test in `pkg/gui/service_test.go` for missing model probe**

Add to `pkg/gui/service_test.go`:
```go
func TestTestProviderSherpaTTSMissingModelReturnsStructuredMissing(t *testing.T) {
	svc := NewService(t.TempDir())
	res, err := svc.TestProvider(context.Background(), TestProviderRequestDTO{
		Category: "tts",
		Provider: config.TTSConfig{
			Type:        "builtin",
			BuiltinName: "sherpa-onnx",
		},
		TestPrompt: "Testing speech",
	})
	if err != nil {
		t.Fatalf("TestProvider failed: %v", err)
	}
	if res.Success {
		t.Errorf("expected failure for uninstalled kokoro model")
	}
	if !res.ModelMissing {
		t.Errorf("expected ModelMissing to be true")
	}
	if res.ModelID != "kokoro-tts" {
		t.Errorf("expected ModelID 'kokoro-tts', got %q", res.ModelID)
	}
}
```

- [ ] **Step 6: Update `Service.TestProvider` in `pkg/gui/service.go`**

In `case "tts"`:
```go
		if ttsCfg.Type == "builtin" && (ttsCfg.BuiltinName == "sherpa-onnx" || ttsCfg.BuiltinName == "kokoro") {
			if ttsCfg.ModelPath == "" && s.modelsManager != nil {
				ttsCfg.ModelPath = s.modelsManager.ModelDir("kokoro-tts")
			}
			if s.modelsManager != nil {
				status := s.modelsManager.Status("kokoro-tts")
				if !status.Installed {
					return &TestProviderResponseDTO{
						Success:      false,
						ModelMissing: true,
						ModelID:      "kokoro-tts",
						Message:      "Kokoro voice pack is not installed; download required",
					}, nil
				}
			}
		}
```

- [ ] **Step 7: Run tests to verify `pkg/config` and `pkg/gui` pass**

Run: `go test -v -count=1 ./pkg/config/ ./pkg/gui/`
Expected: PASS.

- [ ] **Step 8: Commit backend changes**

```bash
git add pkg/config/ pkg/gui/
git commit -m "feat(config,gui): add KokoroVoiceProfiles and structured model_missing diagnostics"
```

---

### Task 2: Frontend Types & Provider Presets Catalog

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/templates/providerPresets.ts`

- [ ] **Step 1: Update `TestProviderResponse` in `frontend/src/types.ts`**

In `frontend/src/types.ts`:
```typescript
export interface TestProviderResponse {
  success: boolean;
  latency_ms: number;
  message: string;
  preview?: string;
  audio_data_uri?: string;
  model_missing?: boolean;
  model_id?: string;
}
```

- [ ] **Step 2: Add `KOKORO_VOICE_PROFILES` and `sherpa-onnx` preset to `frontend/src/templates/providerPresets.ts`**

In `frontend/src/templates/providerPresets.ts`:
1. Define `KOKORO_VOICE_PROFILES: VoiceProfile[]` matching the 25 profiles.
2. Add `sherpa-onnx` to `TTS_PRESETS`:
```typescript
  'sherpa-onnx': {
    label: 'Sherpa-ONNX Kokoro (Built-in Neural TTS)',
    description: 'High-quality Kokoro TTS running in-process via Sherpa-ONNX (downloads model on demand).',
    config: {
      type: 'builtin',
      builtin_name: 'sherpa-onnx',
      default_voice: 'af_bella',
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
      voice_profiles: [...KOKORO_VOICE_PROFILES],
    },
  },
```
3. Update `native-os` label:
```typescript
  'native-os': {
    label: 'Native OS Speech (Built-in Fallback)',
    description: 'Uses spd-say (Linux), say (macOS), or PowerShell (Windows) with procedural audio fallback.',
...
```

- [ ] **Step 3: Run frontend type checks**

Run: `mise run test:frontend`
Expected: PASS (`tsc --noEmit` exits with 0).

- [ ] **Step 4: Commit frontend types and presets**

```bash
git add frontend/src/types.ts frontend/src/templates/providerPresets.ts
git commit -m "feat(frontend): add KOKORO_VOICE_PROFILES and sherpa-onnx preset"
```

---

### Task 3: Settings Studio TTS Engine Selector & Kokoro Autofill

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [ ] **Step 1: Flatten TTS provider selector in `SettingsStudio.tsx`**

Replace the nested builtin select with a clean top-level value mapping:
```tsx
<select
  value={
    config.media.tts.type === 'builtin'
      ? `builtin:${config.media.tts.builtin_name || 'native-os'}`
      : config.media.tts.type
  }
  onChange={(e) => {
    const val = e.target.value;
    if (val.startsWith('builtin:')) {
      const builtinName = val.split(':')[1];
      setConfig({
        ...config,
        media: {
          ...config.media,
          tts: { ...config.media.tts, type: 'builtin', builtin_name: builtinName },
        },
      });
    } else {
      setConfig({
        ...config,
        media: {
          ...config.media,
          tts: { ...config.media.tts, type: val as any, builtin_name: undefined },
        },
      });
    }
  }}
  className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
>
  <option value="disabled">Disabled</option>
  <option value="builtin:sherpa-onnx">Built-in: Sherpa-ONNX (Kokoro Neural Voice)</option>
  <option value="builtin:native-os">Built-in: Native OS Speech (spd-say / SAPI / procedural)</option>
  <option value="http">HTTP Endpoint (Kokoro-FastAPI, AllTalk, OpenAI Speech)</option>
  <option value="cli">CLI Command (e.g. piper)</option>
</select>
```

- [ ] **Step 2: Add Kokoro Autofill button in Voice Profiles Library header**

In the Voice Profiles Library section header:
Import `KOKORO_VOICE_PROFILES` from `../templates/providerPresets`.
Render:
```tsx
{isKokoro && (
  <button
    onClick={() => {
      setConfig({
        ...config,
        media: {
          ...config.media,
          tts: {
            ...config.media.tts,
            voice_profiles: [...KOKORO_VOICE_PROFILES],
          },
        },
      });
    }}
    className="flex items-center gap-1 text-[11px] px-2 py-1 rounded bg-amber-600/20 border border-amber-500/40 text-amber-300 hover:bg-amber-600/30 transition cursor-pointer"
    title="Autofill all 25 Kokoro voice profiles with gender and accent tags"
  >
    <Sparkles className="w-3 h-3" />
    <span>Load Kokoro Voices (25 Profiles)</span>
  </button>
)}
```
Where `isKokoro = config.media.tts.type === 'builtin' && (config.media.tts.builtin_name === 'sherpa-onnx' || config.media.tts.builtin_name === 'kokoro');`.

- [ ] **Step 3: Run frontend type checks**

Run: `mise run test:frontend`
Expected: PASS.

- [ ] **Step 4: Commit selector and autofill changes**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): flatten TTS provider selector and add Kokoro voice profile autofill"
```

---

### Task 4: Settings Studio Model Status & Preview Interception

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [ ] **Step 1: Add model state and SSE subscription in `SettingsStudio.tsx`**

Import `ModelDownloadModal` from `./ModelDownloadModal` and `ModelStatus` from `../types`.
Add state hooks:
```tsx
const [models, setModels] = useState<ModelStatus[]>([]);
const [missingModelPrompt, setMissingModelPrompt] = useState<{
  id: string;
  name: string;
  sizeBytes: number;
} | null>(null);
```
In `useEffect`:
```tsx
APIClient.getModels().then(setModels).catch(console.error);
const unsubscribe = APIClient.subscribeModelEvents((status) => {
  setModels((prev) => {
    const next = [...prev];
    const idx = next.findIndex((m) => m.id === status.id);
    if (idx >= 0) {
      next[idx] = status;
    } else {
      next.push(status);
    }
    return next;
  });
});
return () => unsubscribe();
```

- [ ] **Step 2: Render live model status card for Kokoro**

When `isKokoro`:
Find `kokoroStatus = models.find((m) => m.id === 'kokoro-tts')`.
Render an inline status card beneath the TTS Provider Type:
- If installed:
  `<div className="flex items-center gap-2 text-xs text-emerald-400 bg-emerald-950/30 border border-emerald-800/40 p-2.5 rounded-lg"><CheckCircle className="w-4 h-4" /><span>Kokoro Voice Pack (Installed)</span></div>`
- If downloading:
  Progress bar showing `Math.round(kokoroStatus.progress * 100)%`.
- If not installed:
  Card with warning and `[Download Model (~86 MB)]` button that calls:
  `setMissingModelPrompt({ id: 'kokoro-tts', name: 'Kokoro Voice Pack', sizeBytes: 90177536 })`.

- [ ] **Step 3: Intercept `handleTestProvider` and archetype preview buttons**

In `handleTestProvider`:
```tsx
if (category === 'tts') {
  const isTargetKokoro =
    provider?.type === 'builtin' &&
    (provider?.builtin_name === 'sherpa-onnx' || provider?.builtin_name === 'kokoro');
  const kokoroInstalled = models.find((m) => m.id === 'kokoro-tts')?.installed;
  if (isTargetKokoro && !kokoroInstalled) {
    setMissingModelPrompt({
      id: 'kokoro-tts',
      name: 'Kokoro Voice Pack',
      sizeBytes: 90177536,
    });
    return;
  }
}
```
And in `catch` or after `res`:
```tsx
if (res.model_missing) {
  setMissingModelPrompt({
    id: res.model_id || 'kokoro-tts',
    name: 'Kokoro Voice Pack',
    sizeBytes: 90177536,
  });
}
```

- [ ] **Step 4: Mount `<ModelDownloadModal>` in `SettingsStudio.tsx`**

At the end of `SettingsStudio.tsx` return JSX:
```tsx
{missingModelPrompt && (
  <ModelDownloadModal
    modelId={missingModelPrompt.id}
    modelName={missingModelPrompt.name}
    sizeBytes={missingModelPrompt.sizeBytes}
    onClose={() => setMissingModelPrompt(null)}
  />
)}
```

- [ ] **Step 5: Run frontend type checks**

Run: `mise run test:frontend`
Expected: PASS.

- [ ] **Step 6: Commit model status and modal interception**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): add live Kokoro model status and preview download prompt in Settings"
```

---

### Task 5: End-to-End Verification & Lint Gates

**Files:**
- Verification only

- [ ] **Step 1: Run backend tests**

Run: `mise run test:backend`
Expected: All unit and integration tests pass.

- [ ] **Step 2: Run frontend type check**

Run: `mise run test:frontend`
Expected: `tsc --noEmit` passes with 0 errors.

- [ ] **Step 3: Run backend linter**

Run: `mise run lint`
Expected: `go vet ./...` clean with 0 warnings.

- [ ] **Step 4: Build entire binary (frontend + backend)**

Run: `mise run build`
Expected: Production build succeeds and outputs `bin/localrpg`.
Restore tracked placeholder: `git checkout pkg/gui/dist/.gitkeep`.
