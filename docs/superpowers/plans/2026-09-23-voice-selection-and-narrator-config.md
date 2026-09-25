# Voice Selection, Provider Catalogs, and Campaign Narrator Voice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide an interactive voice catalog picker and searchable combobox for TTS voice profiles, implement static voice catalogs for built-in providers like Sherpa-ONNX, and support campaign-level narrator voice customization.

**Architecture:**
1. Backend: Implement `media.VoiceCatalog` on `SherpaTTSClient` mapping the 11 Kokoro speakers into `ProviderVoice` objects. Support `NarratorVoice` in `CreateGameRequestDTO` and `manifest.Settings["narrator_voice"]` for per-campaign narrator voice overrides.
2. Frontend: Build `VoiceCombobox` with search, preview audio, and fallback to free-text when no catalog is available. Build `VoiceCatalogModal` for browsing and one-click profile creation.
3. UI Integration: Connect `default_voice` and `voice_profiles` in `SettingsStudio`, and add a narrator voice picker to the campaign creation wizard in `LauncherHub` and in-game settings.

**Tech Stack:** Go 1.27.1, React 19, Tailwind CSS v4, Lucide React icons, Vite, TypeScript.

---

## File Map

| Action | File | Responsibility |
| --- | --- | --- |
| Create | `pkg/media/sherpa_tts_catalog_test.go` | Unit tests for `SherpaTTSClient.ListVoices` |
| Modify | `pkg/media/sherpa_tts.go` | Implement `ListVoices(ctx context.Context) ([]ProviderVoice, error)` |
| Modify | `pkg/gui/types.go` | Add `NarratorVoice` to `CreateGameRequestDTO` |
| Modify | `pkg/gui/service.go` | Store `narrator_voice` in `manifest.Settings` and resolve it during turn TTS synthesis |
| Modify | `pkg/gui/service_test.go` | Test `CreateGame` with `NarratorVoice` |
| Modify | `frontend/src/types.ts` | Add `narrator_voice` to `CreateGameRequest` |
| Create | `frontend/src/components/VoiceCombobox.tsx` | Searchable voice selector with preview button and free-text fallback |
| Create | `frontend/src/components/VoiceCatalogModal.tsx` | Catalog browser modal for auditioning and creating voice profiles |
| Modify | `frontend/src/components/SettingsStudio.tsx` | Add Default Voice combobox, replace profile Voice ID input, add Import from Catalog button |
| Modify | `frontend/src/components/LauncherHub.tsx` | Add Narrator Voice selector to campaign creation wizard |

---

### Task 1: Sherpa-ONNX VoiceCatalog Implementation

**Files:**
- Create: `pkg/media/sherpa_tts_catalog_test.go`
- Modify: `pkg/media/sherpa_tts.go`

- [x] **Step 1: Write failing test for `SherpaTTSClient.ListVoices`**

```go
package media

import (
	"context"
	"testing"
)

func TestSherpaTTSClientListVoices(t *testing.T) {
	client := NewSherpaTTSClient(t.TempDir())
	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices: %v", err)
	}
	if len(voices) != 11 {
		t.Fatalf("expected 11 voices, got %d", len(voices))
	}

	foundBella := false
	for _, v := range voices {
		if v.ID == "af_bella" {
			foundBella = true
			if v.Name != "Bella (American Female)" {
				t.Errorf("expected name 'Bella (American Female)', got %q", v.Name)
			}
			if v.Gender != "female" {
				t.Errorf("expected gender 'female', got %q", v.Gender)
			}
		}
	}
	if !foundBella {
		t.Errorf("did not find 'af_bella' in listed voices")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v -count=1 ./pkg/media/ -run TestSherpaTTSClientListVoices`  
Expected: FAIL with `client.ListVoices undefined`

- [x] **Step 3: Implement `ListVoices` on `SherpaTTSClient`**

In `pkg/media/sherpa_tts.go`:

```go
// ListVoices enumerates the 11 known Kokoro speakers for the pinned model.
// It requires neither network access nor loaded model weights.
func (s *SherpaTTSClient) ListVoices(ctx context.Context) ([]ProviderVoice, error) {
	speakers := KokoroSpeakersForModel(s.modelID)
	voices := make([]ProviderVoice, 0, len(speakers))

	// Friendly metadata table indexed by speaker ID
	profiles := make(map[string]struct {
		name        string
		gender      string
		accent      string
		tags        []string
		description string
	}, len(speakers))

	profiles["af"] = struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{"Default (American Female)", "female", "american", []string{"american", "female", "default", "neutral"}, "The model's stock American female voice."}
	profiles["af_bella"] = struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{"Bella (American Female)", "female", "american", []string{"american", "female", "warm", "friendly"}, "American female voice, warm, approachable, and pleasant."}
	profiles["af_nicole"] = struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{"Nicole (American Female)", "female", "american", []string{"american", "female", "youthful", "energetic"}, "American female voice, brisk, youthful, and direct."}
	profiles["af_sarah"] = struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{"Sarah (American Female)", "female", "american", []string{"american", "female", "poised", "narrative"}, "American female voice, polished, measured, and story-oriented."}
	profiles["af_sky"] = struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{"Sky (American Female)", "female", "american", []string{"american", "female", "light", "airy"}, "American female voice, light, gentle, and breathy."}
	profiles["am_adam"] = struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{"Adam (American Male)", "male", "american", []string{"american", "male", "deep", "authoritative"}, "American male voice, deep, steady, and commanding."}
	profiles["am_michael"] = struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{"Michael (American Male)", "male", "american", []string{"american", "male", "commanding", "formal"}, "American male voice, disciplined, authoritative, and formal."}
	profiles["bf_emma"] = struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{"Emma (British Female)", "female", "british", []string{"british", "female", "gentle", "poised"}, "British female voice, elegant, gentle, and softly spoken."}
	profiles["bf_isabella"] = struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{"Isabella (British Female)", "female", "british", []string{"british", "female", "noble", "melodic"}, "British female voice, aristocratic, melodic, and graceful."}
	profiles["bm_george"] = struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{"George (British Male)", "male", "british", []string{"british", "male", "mature", "distinguished"}, "British male voice, mature, distinguished, and resonant."}
	profiles["bm_lewis"] = struct {
		name, gender, accent string
		tags                 []string
		description          string
	}{"Lewis (British Male)", "male", "british", []string{"british", "male", "thoughtful", "refined"}, "British male voice, measured, polite, and reflective."}

	for _, s := range speakers {
		meta, ok := profiles[s.Name]
		if !ok {
			meta.name = s.Name
		}
		voices = append(voices, ProviderVoice{
			ID:          s.Name,
			Name:        meta.name,
			Gender:      meta.gender,
			Accent:      meta.accent,
			Categories:  []string{"built-in"},
			Tags:        meta.tags,
			Description: meta.description,
		})
	}
	return voices, nil
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -v -count=1 ./pkg/media/ -run TestSherpaTTSClientListVoices`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/media/sherpa_tts.go pkg/media/sherpa_tts_catalog_test.go
git commit -m "feat(media): implement VoiceCatalog for SherpaTTSClient"
```

---

### Task 2: Backend Campaign Narrator Voice Configuration

**Files:**
- Modify: `pkg/gui/types.go:148-157`
- Modify: `pkg/gui/service.go:1410-1425,1720-1760`
- Modify: `pkg/gui/service_test.go`

- [x] **Step 1: Write failing test in `pkg/gui/service_test.go`**

```go
func TestCreateGamePersistsNarratorVoice(t *testing.T) {
	svc := newTestService(t)
	game, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:          "Narrator Campaign",
		SystemID:      "test-system",
		WorldID:       "test-world",
		PlayerName:    "Hero",
		NarratorVoice: "custom_narrator_voice",
		Player: PlayerCharacterDTO{
			Appearance: "Tall and dark",
		},
	})
	if err != nil {
		t.Fatalf("CreateGame: %v", err)
	}

	manifest, err := svc.loadGameManifest(game.ID)
	if err != nil {
		t.Fatalf("loadGameManifest: %v", err)
	}
	if manifest.Settings["narrator_voice"] != "custom_narrator_voice" {
		t.Errorf("expected narrator_voice 'custom_narrator_voice', got %v", manifest.Settings["narrator_voice"])
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v -count=1 ./pkg/gui/ -run TestCreateGamePersistsNarratorVoice`  
Expected: FAIL (field `NarratorVoice` unknown in `CreateGameRequestDTO`)

- [x] **Step 3: Update `CreateGameRequestDTO` and `service.go`**

In `pkg/gui/types.go`:
```go
type CreateGameRequestDTO struct {
	ID            string             `json:"id,omitempty"`
	Name          string             `json:"name"`
	SystemID      string             `json:"system_id"`
	WorldID       string             `json:"world_id"`
	PlayerName    string             `json:"player_name"`
	Player        PlayerCharacterDTO `json:"player,omitempty"`
	OpeningPrompt string             `json:"opening_prompt,omitempty"`
	NarratorVoice string             `json:"narrator_voice,omitempty"`
}
```

In `pkg/gui/service.go` in `CreateGame`:
```go
	if req.NarratorVoice != "" {
		if manifest.Settings == nil {
			manifest.Settings = map[string]interface{}{}
		}
		manifest.Settings["narrator_voice"] = req.NarratorVoice
	}
```

In `pkg/gui/service.go` in `narratorVoiceFor(gameID string, cfg config.Config) *entity.VoiceConfig`:
```go
func (s *Service) narratorVoiceFor(gameID string, cfg config.Config) *entity.VoiceConfig {
	voiceID := cfg.Media.TTS.DefaultVoice
	if manifest, err := s.loadGameManifest(gameID); err == nil && manifest != nil && manifest.Settings != nil {
		if nv, ok := manifest.Settings["narrator_voice"].(string); ok && strings.TrimSpace(nv) != "" {
			voiceID = strings.TrimSpace(nv)
		}
	}
	return &entity.VoiceConfig{
		VoiceID:    voiceID,
		Pitch:      cfg.Media.TTS.Pitch,
		SpeechRate: cfg.Media.TTS.SpeechRate,
		Options:    cfg.Media.TTS.Options,
	}
}
```
Update `SynthesizeUtterance` (around line 1416) and `probeTTSWithConfig` (around line 1478) to call `s.narratorVoiceFor(gameID, cfg)` instead of bare `cfg.Media.TTS.DefaultVoice`.

- [x] **Step 4: Run tests to verify it passes**

Run: `go test -v -count=1 ./pkg/gui/ -run TestCreateGamePersistsNarratorVoice`  
Expected: PASS

- [x] **Step 5: Run all backend tests**

Run: `go test -count=1 ./pkg/...`  
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/service_test.go
git commit -m "feat(gui): support campaign-level narrator voice setting"
```

---

### Task 3: Reusable `VoiceCombobox` Frontend Component

**Files:**
- Create: `frontend/src/components/VoiceCombobox.tsx`
- Modify: `frontend/src/types.ts`

- [x] **Step 1: Check `frontend/src/types.ts` and add `narrator_voice` to `CreateGameRequest`**

In `frontend/src/types.ts`:
```typescript
export interface CreateGameRequest {
  id?: string;
  name: string;
  system_id: string;
  world_id: string;
  player_name: string;
  player?: PlayerCharacter;
  opening_prompt?: string;
  narrator_voice?: string;
}
```

- [x] **Step 2: Create `frontend/src/components/VoiceCombobox.tsx`**

Implement `VoiceCombobox`:
```typescript
import React, { useState, useRef, useEffect, useMemo } from 'react';
import { ChevronDown, Play, X } from 'lucide-react';
import { ProviderVoice } from '../types';
import { playVoicePreview } from '../lib/audioPreview';

export interface VoiceComboboxProps {
  value: string;
  onChange: (value: string) => void;
  voices?: ProviderVoice[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

export const VoiceCombobox: React.FC<VoiceComboboxProps> = ({
  value,
  onChange,
  voices = [],
  placeholder = 'Select or enter voice ID...',
  disabled = false,
  className = '',
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQuery] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);

  const selectedVoice = useMemo(() => {
    return voices.find((v) => v.id === value);
  }, [voices, value]);

  // Keep search query synced with selection when closed
  useEffect(() => {
    if (!isOpen) {
      setQuery(selectedVoice ? selectedVoice.name : value);
    }
  }, [value, selectedVoice, isOpen]);

  // Handle outside clicks to close dropdown
  useEffect(() => {
    const handleOutsideClick = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleOutsideClick);
    return () => document.removeEventListener('mousedown', handleOutsideClick);
  }, []);

  const filteredVoices = useMemo(() => {
    if (!voices.length) return [];
    const needle = query.trim().toLowerCase();
    if (!needle) return voices;
    return voices.filter((v) => {
      return (
        v.name.toLowerCase().includes(needle) ||
        v.id.toLowerCase().includes(needle) ||
        v.tags?.some((t) => t.toLowerCase().includes(needle)) ||
        v.categories?.some((c) => c.toLowerCase().includes(needle))
      );
    });
  }, [voices, query]);

  // If no catalog is available, render clean text input
  if (!voices || voices.length === 0) {
    return (
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        disabled={disabled}
        className={`bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs font-mono text-stone-200 focus:outline-none focus:border-amber-500/60 ${className}`}
      />
    );
  }

  return (
    <div ref={containerRef} className={`relative flex items-center gap-1.5 ${className}`}>
      <div className="relative flex-1">
        <input
          type="text"
          value={isOpen ? query : (selectedVoice ? `${selectedVoice.name} (${selectedVoice.id})` : value)}
          onChange={(e) => {
            setQuery(e.target.value);
            onChange(e.target.value);
            if (!isOpen) setIsOpen(true);
          }}
          onFocus={() => {
            setQuery(selectedVoice ? selectedVoice.name : value);
            setIsOpen(true);
          }}
          placeholder={placeholder}
          disabled={disabled}
          className="w-full bg-stone-900 border border-stone-800 rounded px-2 py-1 pr-6 text-xs text-stone-200 focus:outline-none focus:border-amber-500/60 truncate"
        />
        <button
          type="button"
          onClick={() => setIsOpen((prev) => !prev)}
          tabIndex={-1}
          className="absolute right-1.5 top-1/2 -translate-y-1/2 text-stone-500 hover:text-stone-300"
        >
          <ChevronDown className="w-3.5 h-3.5" />
        </button>
      </div>

      {selectedVoice?.preview_url && (
        <button
          type="button"
          onClick={() => playVoicePreview(selectedVoice.preview_url as string)}
          className="p-1 rounded bg-stone-900 border border-stone-800 text-amber-400 hover:text-amber-300 cursor-pointer shrink-0"
          title="Audition Voice"
        >
          <Play className="w-3 h-3" />
        </button>
      )}

      {isOpen && (
        <div className="absolute left-0 top-full mt-1 w-full min-w-[260px] max-h-60 overflow-y-auto bg-stone-950 border border-stone-800 rounded-lg shadow-2xl z-50 p-1 space-y-0.5">
          {filteredVoices.length === 0 ? (
            <div className="px-2 py-1.5 text-[11px] text-stone-500 italic">No matching voices</div>
          ) : (
            filteredVoices.map((voice) => {
              const isSelected = voice.id === value;
              return (
                <div
                  key={voice.id}
                  onClick={() => {
                    onChange(voice.id);
                    setIsOpen(false);
                  }}
                  className={`flex items-center justify-between gap-2 px-2 py-1.5 rounded text-xs cursor-pointer ${
                    isSelected ? 'bg-amber-500/20 text-amber-300' : 'text-stone-300 hover:bg-stone-900'
                  }`}
                >
                  <div className="min-w-0">
                    <div className="font-medium truncate">{voice.name}</div>
                    <div className="text-[10px] font-mono text-stone-500 truncate">{voice.id}</div>
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    {voice.categories?.[0] && (
                      <span className="text-[9px] px-1 py-0.5 rounded bg-stone-900 text-stone-400 border border-stone-800">
                        {voice.categories[0]}
                      </span>
                    )}
                    {voice.preview_url && (
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          playVoicePreview(voice.preview_url as string);
                        }}
                        className="p-1 rounded hover:bg-stone-800 text-amber-400"
                        title="Audition"
                      >
                        <Play className="w-3 h-3" />
                      </button>
                    )}
                  </div>
                </div>
              );
            })
          )}
        </div>
      )}
    </div>
  );
};
```

- [x] **Step 3: Test TypeScript build**

Run: `mise run test:frontend`  
Expected: PASS

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/VoiceCombobox.tsx frontend/src/types.ts
git commit -m "feat(frontend): add reusable VoiceCombobox component"
```

---

### Task 4: `VoiceCatalogModal` Component

**Files:**
- Create: `frontend/src/components/VoiceCatalogModal.tsx`

- [x] **Step 1: Create `VoiceCatalogModal.tsx`**

```typescript
import React, { useState, useMemo } from 'react';
import { X, Play, Plus, Search } from 'lucide-react';
import { ProviderVoice, VoiceProfile } from '../types';
import { playVoicePreview } from '../lib/audioPreview';

export interface VoiceCatalogModalProps {
  isOpen: boolean;
  onClose: () => void;
  voices: ProviderVoice[];
  providerKey: string;
  onAddProfile: (profile: VoiceProfile) => void;
}

export const VoiceCatalogModal: React.FC<VoiceCatalogModalProps> = ({
  isOpen,
  onClose,
  voices,
  providerKey,
  onAddProfile,
}) => {
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('');

  const categories = useMemo(() => {
    const seen = new Set<string>();
    voices.forEach((v) => v.categories?.forEach((cat) => seen.add(cat)));
    return Array.from(seen).sort();
  }, [voices]);

  const filteredVoices = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return voices.filter((v) => {
      if (category && !v.categories?.includes(category)) return false;
      if (!needle) return true;
      return (
        v.name.toLowerCase().includes(needle) ||
        v.id.toLowerCase().includes(needle) ||
        v.tags?.some((t) => t.toLowerCase().includes(needle))
      );
    });
  }, [voices, query, category]);

  if (!isOpen) return null;

  const handleAdd = (voice: ProviderVoice) => {
    const slug = voice.name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '_')
      .replace(/^_+|_+$/g, '');

    onAddProfile({
      id: slug || voice.id,
      name: voice.name,
      voice_id: voice.id,
      provider: providerKey,
      pitch: 1.0,
      speech_rate: 1.0,
      tags: voice.tags || [],
      description: voice.description || `${voice.name} voice profile.`,
      options: voice.defaults,
    });
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-xs flex items-center justify-center p-4">
      <div className="bg-stone-950 border border-stone-800 rounded-2xl w-full max-w-2xl max-h-[85vh] flex flex-col shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-stone-800">
          <div>
            <h2 className="font-cinzel text-base font-bold text-amber-400">Voice Catalog Browser</h2>
            <p className="text-xs text-stone-400">Audition provider voices and add them directly into your archetypes.</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded-lg text-stone-400 hover:text-stone-200 hover:bg-stone-900 cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Filter bar */}
        <div className="p-4 border-b border-stone-800/80 bg-stone-900/30 flex flex-wrap gap-2.5">
          <div className="relative flex-1 min-w-[200px]">
            <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-stone-400" />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search by voice name, ID, or tag..."
              className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-8 pr-3 py-1.5 text-xs text-stone-100 focus:outline-none focus:border-amber-500/60"
            />
          </div>
          {categories.length > 0 && (
            <select
              value={category}
              onChange={(e) => setCategory(e.target.value)}
              className="bg-stone-950 border border-stone-800 rounded-xl px-3 py-1.5 text-xs text-stone-200 focus:outline-none cursor-pointer"
            >
              <option value="">All categories</option>
              {categories.map((cat) => (
                <option key={cat} value={cat}>
                  {cat}
                </option>
              ))}
            </select>
          )}
        </div>

        {/* Voice list */}
        <div className="flex-1 overflow-y-auto p-4 space-y-2">
          {filteredVoices.length === 0 ? (
            <div className="text-center py-12 text-xs text-stone-500 italic">No voices match your search.</div>
          ) : (
            filteredVoices.map((voice) => (
              <div
                key={voice.id}
                className="flex items-center justify-between gap-3 p-3 rounded-xl bg-stone-900/40 border border-stone-800/80 hover:border-stone-700 transition"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-semibold text-stone-200">{voice.name}</span>
                    <span className="text-[10px] font-mono text-stone-500">{voice.id}</span>
                    {voice.accent && (
                      <span className="text-[9px] px-1.5 py-0.5 rounded bg-stone-900 border border-stone-800 text-stone-400 capitalize">
                        {voice.accent}
                      </span>
                    )}
                  </div>
                  {voice.description && (
                    <p className="text-[11px] text-stone-400 mt-0.5 truncate">{voice.description}</p>
                  )}
                  {voice.tags && voice.tags.length > 0 && (
                    <div className="flex flex-wrap gap-1 mt-1.5">
                      {voice.tags.slice(0, 4).map((tag) => (
                        <span key={tag} className="text-[9px] px-1 py-0.2 rounded bg-stone-950 text-stone-400 border border-stone-800">
                          {tag}
                        </span>
                      ))}
                    </div>
                  )}
                </div>

                <div className="flex items-center gap-1.5 shrink-0">
                  {voice.preview_url && (
                    <button
                      type="button"
                      onClick={() => playVoicePreview(voice.preview_url as string)}
                      className="p-2 rounded-lg bg-stone-900 border border-stone-800 text-amber-400 hover:text-amber-300 hover:bg-stone-800 cursor-pointer"
                      title="Audition"
                    >
                      <Play className="w-3.5 h-3.5" />
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => handleAdd(voice)}
                    className="flex items-center gap-1 text-xs px-2.5 py-1.5 rounded-lg bg-amber-600/20 border border-amber-500/40 text-amber-300 hover:bg-amber-600/30 cursor-pointer font-cinzel font-medium"
                    title="Add as profile"
                  >
                    <Plus className="w-3.5 h-3.5" />
                    <span>Add</span>
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
};
```

- [x] **Step 2: Test TypeScript compilation**

Run: `mise run test:frontend`  
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/VoiceCatalogModal.tsx
git commit -m "feat(frontend): add VoiceCatalogModal component"
```

---

### Task 5: Settings Studio Integration

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Import `VoiceCombobox` and `VoiceCatalogModal` in `SettingsStudio.tsx`**

In `frontend/src/components/SettingsStudio.tsx`:
Add imports:
```typescript
import { VoiceCombobox } from './VoiceCombobox';
import { VoiceCatalogModal } from './VoiceCatalogModal';
```
Add state for the catalog modal:
```typescript
const [isCatalogModalOpen, setIsCatalogModalOpen] = useState(false);
```

- [x] **Step 2: Add Default Voice setting to TTS Provider section**

Under the API Key / Model / Tuning section (around line 1290):
```typescript
            {config.media.tts.type !== 'disabled' && (
              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300">Default Voice</label>
                <VoiceCombobox
                  value={config.media.tts.default_voice || ''}
                  onChange={(voiceID) =>
                    setConfig({
                      ...config,
                      media: {
                        ...config.media,
                        tts: { ...config.media.tts, default_voice: voiceID },
                      },
                    })
                  }
                  voices={inspect?.catalog.voices ?? []}
                  placeholder="Select default provider voice..."
                />
                <p className="text-[11px] text-stone-500">
                  Fallback voice used for turn narration and unvoiced characters.
                </p>
              </div>
            )}
```

- [x] **Step 3: Replace profile `voice_id` input with `VoiceCombobox`**

In the Voice Profiles map function (around line 1486):
Replace `<input placeholder="Voice ID (e.g. af_bella)">` with:
```typescript
                        <VoiceCombobox
                          value={profile.voice_id}
                          onChange={(voiceID) => {
                            const updated = [...(config.media.tts.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], voice_id: voiceID };
                            setConfig({
                              ...config,
                              media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                            });
                          }}
                          voices={inspect?.catalog.voices ?? []}
                          placeholder="Voice ID"
                        />
```

- [x] **Step 4: Add "Import from Catalog" button in Voice Profiles header**

Next to `Load Fantasy Defaults` and `Add Profile`:
```typescript
                  {Boolean(inspect?.catalog.voices?.length) && (
                    <button
                      type="button"
                      onClick={() => setIsCatalogModalOpen(true)}
                      className="flex items-center gap-1 text-[11px] px-2 py-1 rounded bg-stone-900 border border-amber-500/40 text-amber-300 hover:bg-stone-800 transition cursor-pointer"
                    >
                      <Plus className="w-3 h-3" />
                      <span>Import from Catalog</span>
                    </button>
                  )}
```
And render `<VoiceCatalogModal>` before the closing container:
```typescript
      <VoiceCatalogModal
        isOpen={isCatalogModalOpen}
        onClose={() => setIsCatalogModalOpen(false)}
        voices={inspect?.catalog.voices ?? []}
        providerKey={inspect?.provider_key || 'tts'}
        onAddProfile={(newProfile) => {
          const current = config?.media.tts.voice_profiles || [];
          if (config) {
            setConfig({
              ...config,
              media: {
                ...config.media,
                tts: {
                  ...config.media.tts,
                  voice_profiles: [...current, newProfile],
                },
              },
            });
          }
        }}
      />
```

- [x] **Step 5: Test TypeScript compilation**

Run: `mise run test:frontend`  
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): integrate VoiceCombobox and VoiceCatalogModal into SettingsStudio"
```

---

### Task 6: Campaign Creation & In-Game Narrator Voice Selector

**Files:**
- Modify: `frontend/src/components/LauncherHub.tsx`

- [x] **Step 1: Add `narratorVoiceID` state in `LauncherHub.tsx`**

```typescript
const [narratorVoiceID, setNarratorVoiceID] = useState<string>('');
```
Reset `narratorVoiceID` in `openNewGameWizard` to empty string.

- [x] **Step 2: Add Narrator Voice field to Campaign Wizard in `LauncherHub.tsx`**

In `LauncherHub.tsx` in the character / story setup step (around line 690):
```typescript
                      <div className="space-y-1.5">
                        <label className="text-xs font-cinzel uppercase text-stone-300">Narrator Voice</label>
                        <select
                          value={narratorVoiceID}
                          onChange={(e) => setNarratorVoiceID(e.target.value)}
                          className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
                        >
                          <option value="">Default (Provider Setting)</option>
                          {voiceProfiles.map((profile) => (
                            <option key={profile.id} value={profile.voice_id}>
                              {profile.name} ({profile.voice_id})
                            </option>
                          ))}
                        </select>
                        <p className="text-[11px] text-stone-500">
                          The voice used to narrate scenes, GM responses, and descriptions in this campaign.
                        </p>
                      </div>
```

- [x] **Step 3: Pass `narrator_voice` in `createGame` payload**

In `handleCreateGame`:
```typescript
      const payload: CreateGameRequest = {
        name: newGameName.trim(),
        system_id: effectiveSystemID,
        world_id: effectiveWorldID,
        player_name: newPlayerName.trim() || 'Adventurer',
        narrator_voice: narratorVoiceID || undefined,
        player: {
          appearance: playerAnswers.appearance?.trim() || undefined,
...
```

- [x] **Step 4: Test TypeScript build**

Run: `mise run test:frontend`  
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/LauncherHub.tsx
git commit -m "feat(frontend): add narrator voice selection to campaign creation wizard"
```

---

### Task 7: Full Verification and Build

- [x] **Step 1: Run frontend typecheck**

Run: `mise run test:frontend`  
Expected: PASS

- [x] **Step 2: Run frontend production build**

Run: `mise run build:frontend`  
Expected: PASS, Vite bundle emitted into `pkg/gui/dist`

- [x] **Step 3: Restore tracked placeholder if deleted**

Run: `git checkout -- pkg/gui/dist/.gitkeep 2>/dev/null || true`

- [x] **Step 4: Run Go vet**

Run: `mise run lint`  
Expected: PASS

- [x] **Step 5: Run all backend tests**

Run: `mise run test:backend`  
Expected: PASS

- [x] **Step 6: Run full binary build**

Run: `mise run build`  
Expected: PASS with binary at `bin/localrpg`
