# On-Demand Text Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement on-demand AI text generation across all GUI forms, allowing single-field generation via inline buttons and full-form population via "Generate All" actions.

**Architecture:** A unified backend endpoint `POST /api/generate-text` accepts a form type, field name (or `_all`), and sibling field context to produce coherent text. The endpoint routes prompts according to persona templates (`character`, `world`, `system`, `campaign`) and parses responses via existing robust JSON extraction. The frontend introduces a reusable `<AIGenerateButton>` component and integrates it alongside form-level "Generate All" actions in `NewCampaignModal`, `CampaignSettingsModal`, `WorldsStudio`, and `SystemsStudio`.

**Tech Stack:** Go 1.27.1 (`pkg/gui`, `pkg/harness`, `pkg/core`), React 19 + TypeScript + Tailwind v4, Lucide icons (`Sparkles`, `Wand2`).

---

### Task 1: Backend DTOs and Prompt Builder with Unit Tests

**Files:**
- Create: `pkg/gui/text_generate.go`
- Create: `pkg/gui/text_generate_test.go`

- [x] **Step 1: Write the failing tests for prompt building**

Create `pkg/gui/text_generate_test.go`:
```go
package gui

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestBuildTextGeneratorPrompt_AllFields(t *testing.T) {
	req := GenerateTextRequest{
		FormType:  "world",
		FieldName: "_all",
		Context: map[string]string{
			"genre": "Dark Fantasy",
		},
	}

	prompt := buildTextGeneratorPrompt(req, nil)

	if !strings.Contains(prompt, "Generate values for the following fields:") {
		t.Errorf("expected prompt to instruct generating all fields, got %q", prompt)
	}
	if !strings.Contains(prompt, "genre: Dark Fantasy") {
		t.Errorf("expected prompt to contain context, got %q", prompt)
	}
	if !strings.Contains(prompt, "- name") || !strings.Contains(prompt, "- description") {
		t.Errorf("expected prompt to list default world fields, got %q", prompt)
	}
}

func TestBuildTextGeneratorPrompt_SingleFieldWithSeed(t *testing.T) {
	req := GenerateTextRequest{
		FormType:  "system",
		FieldName: "rules_prompt",
		Seed:      "Use 2d6 rolling over a target difficulty",
		Context: map[string]string{
			"name": "Narrative 2d6",
		},
	}

	prompt := buildTextGeneratorPrompt(req, nil)

	if !strings.Contains(prompt, "Generate a value for the 'rules_prompt' field.") {
		t.Errorf("expected single-field instruction, got %q", prompt)
	}
	if !strings.Contains(prompt, "Use 2d6 rolling over a target difficulty") {
		t.Errorf("expected seed in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "name: Narrative 2d6") {
		t.Errorf("expected context in prompt, got %q", prompt)
	}
}

func TestBuildTextGeneratorPrompt_CharacterFields(t *testing.T) {
	customFields := []core.CharacterCreationField{
		{ID: "hometown", Label: "Hometown", Prompt: "Where they grew up", Generatable: true},
	}
	req := GenerateTextRequest{
		FormType:  "character",
		FieldName: "_all",
		Context: map[string]string{
			"name": "Elena",
		},
	}

	prompt := buildTextGeneratorPrompt(req, customFields)

	if !strings.Contains(prompt, "- hometown (Hometown): Where they grew up") {
		t.Errorf("expected custom character field in prompt, got %q", prompt)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestBuildTextGeneratorPrompt ./pkg/gui/`  
Expected output: FAIL (`undefined: GenerateTextRequest` or `undefined: buildTextGeneratorPrompt`)

- [x] **Step 3: Implement DTOs and prompt builder**

Create `pkg/gui/text_generate.go`:
```go
package gui

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
)

type GenerateTextRequest struct {
	FormType  string            `json:"form_type"` // "character", "world", "system", "campaign"
	FieldName string            `json:"field_name"` // field id or "_all"
	Context   map[string]string `json:"context"`
	WorldID   string            `json:"world_id,omitempty"`
	SystemID  string            `json:"system_id,omitempty"`
	Seed      string            `json:"seed,omitempty"`
}

type GenerateTextResponse struct {
	Fields      map[string]string `json:"fields"`
	GeneratedBy string            `json:"generated_by"`
}

const (
	textGeneratorSystemPromptCharacter = `You invent player characters for a tabletop roleplaying game.
Return one JSON object and nothing else, with a string value for every requested field.
Write in the second person, concrete and vivid, at most two sentences per field.
Do not add fields that were not requested.`

	textGeneratorSystemPromptWorld = `You are a creative world builder for a tabletop roleplaying game.
Return one JSON object and nothing else, with a string value for every requested field.
Write evocative, vivid text. Do not add fields that were not requested.`

	textGeneratorSystemPromptSystem = `You are a technical game system designer for a tabletop roleplaying game.
Return one JSON object and nothing else, with a string value for every requested field.
Write clear, concise technical text. Do not add fields that were not requested.`

	textGeneratorSystemPromptCampaign = `You are a game master planning a tabletop roleplaying game campaign.
Return one JSON object and nothing else, with a string value for every requested field.
Write engaging, atmospheric text. Do not add fields that were not requested.`
)

func buildTextGeneratorPrompt(req GenerateTextRequest, systemFields []core.CharacterCreationField) string {
	var b strings.Builder

	if req.FieldName == "_all" {
		b.WriteString("Generate values for the following fields:\n")
	} else {
		b.WriteString(fmt.Sprintf("Generate a value for the '%s' field.\n", req.FieldName))
	}

	if len(req.Context) > 0 {
		b.WriteString("\nContext (use this to ensure consistency):\n")
		for k, v := range req.Context {
			trimmed := strings.TrimSpace(v)
			if trimmed != "" {
				b.WriteString(fmt.Sprintf("- %s: %s\n", k, trimmed))
			}
		}
	}

	b.WriteString("\nRequested Fields:\n")

	if req.FormType == "character" {
		standardFields := []core.CharacterCreationField{
			{ID: "name", Label: "Character Name"},
			{ID: "age", Label: "Age"},
			{ID: "gender", Label: "Gender"},
			{ID: "pronouns", Label: "Pronouns"},
			{ID: "appearance", Label: "Physical Appearance"},
			{ID: "background", Label: "Background / Backstory"},
		}
		allFields := append(standardFields, systemFields...)
		for _, f := range allFields {
			if req.FieldName == "_all" || req.FieldName == f.ID {
				b.WriteString(fmt.Sprintf("- %s (%s)", f.ID, f.Label))
				if f.Prompt != "" {
					b.WriteString(": " + f.Prompt)
				}
				b.WriteString("\n")
			}
		}
	} else {
		var fields []string
		switch req.FormType {
		case "world":
			fields = []string{"name", "description", "genre", "art_style", "lore_prompt"}
		case "system":
			fields = []string{"name", "description", "rules_prompt"}
		case "campaign":
			fields = []string{"name", "start_location", "opening_prompt"}
		}

		for _, f := range fields {
			if req.FieldName == "_all" || req.FieldName == f {
				b.WriteString(fmt.Sprintf("- %s\n", f))
			}
		}
	}

	if strings.TrimSpace(req.Seed) != "" {
		b.WriteString("\nSeed/Existing Text (extend or improve this):\n" + strings.TrimSpace(req.Seed) + "\n")
	}

	b.WriteString("\nReturn a JSON object keyed by the requested field names.")
	return b.String()
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestBuildTextGeneratorPrompt ./pkg/gui/`  
Expected output: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/gui/text_generate.go pkg/gui/text_generate_test.go
git commit -m "feat(gui): define text generation DTOs and prompt builder"
```

---

### Task 2: Service Method, Route Handler, and Endpoint Tests

**Files:**
- Modify: `pkg/gui/text_generate.go`
- Modify: `pkg/gui/server.go`
- Modify: `pkg/gui/text_generate_test.go`

- [x] **Step 1: Write integration test for the HTTP route**

Append to `pkg/gui/text_generate_test.go`:
```go
func TestHandleGenerateTextRoute_InvalidMethod(t *testing.T) {
	tempDir := t.TempDir()
	service := NewService(tempDir)
	server := NewServer(service, nil, "127.0.0.1:0")

	req := httptest.NewRequest("GET", "/api/generate-text", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed, got %d", w.Code)
	}
}

func TestHandleGenerateTextRoute_EmptyBody(t *testing.T) {
	tempDir := t.TempDir()
	service := NewService(tempDir)
	server := NewServer(service, nil, "127.0.0.1:0")

	req := httptest.NewRequest("POST", "/api/generate-text", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK with empty response, got %d", w.Code)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestHandleGenerateTextRoute ./pkg/gui/`  
Expected output: FAIL (`404 page not found`)

- [x] **Step 3: Implement `Service.GenerateText` and `handleGenerateTextRoute`**

Append to `pkg/gui/text_generate.go`:
```go
import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func (s *Service) GenerateText(ctx context.Context, req GenerateTextRequest) (*GenerateTextResponse, error) {
	resp := &GenerateTextResponse{Fields: map[string]string{}, GeneratedBy: "none"}

	systemPrompt := textGeneratorSystemPromptWorld
	role := "gm"
	var systemFields []core.CharacterCreationField

	switch req.FormType {
	case "character":
		systemPrompt = textGeneratorSystemPromptCharacter
		role = "character"
		if req.SystemID != "" {
			path := filepath.Join(s.resolver.SystemDir(req.SystemID), "system.yaml")
			if manifest, err := core.LoadSystemManifest(path); err == nil {
				for _, f := range manifest.CharacterCreation.Fields {
					if f.Kind != "voice" && f.Generatable {
						systemFields = append(systemFields, f)
					}
				}
			}
		}
	case "system":
		systemPrompt = textGeneratorSystemPromptSystem
	case "campaign":
		systemPrompt = textGeneratorSystemPromptCampaign
	}

	router, err := harness.RouterFromConfigWithLogger(s.configMgr.Get(), s.logger)
	if err != nil {
		return resp, nil
	}

	request := harness.GenerateRequest{
		System:    systemPrompt,
		Prompt:    buildTextGeneratorPrompt(req, systemFields),
		MaxTokens: 1000,
	}

	for _, tryRole := range []string{role, "gm"} {
		result, err := router.GenerateForRole(ctx, tryRole, request)
		if err != nil || result == nil || strings.TrimSpace(result.Text) == "" {
			continue
		}
		resp.GeneratedBy = tryRole
		values := decodeGeneratedValues(result.Text)
		for k, v := range values {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				resp.Fields[k] = trimmed
			}
		}
		break
	}

	if len(resp.Fields) == 0 {
		resp.GeneratedBy = "none"
	}
	return resp, nil
}

func (s *Server) handleGenerateTextRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req GenerateTextRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTurnBody)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	resp, err := s.service.GenerateText(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}
```

In `pkg/gui/server.go`, add route registration in `registerRoutes()`:
```go
s.mux.HandleFunc("/api/generate-text", s.handleGenerateTextRoute)
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestHandleGenerateTextRoute ./pkg/gui/`  
Expected output: PASS

- [x] **Step 5: Run full backend vet and tests, then commit**

Run: `go vet ./... && go test -count=1 ./pkg/gui/`  
Expected output: PASS  
```bash
git add pkg/gui/text_generate.go pkg/gui/server.go pkg/gui/text_generate_test.go
git commit -m "feat(gui): add POST /api/generate-text endpoint and service method"
```

---

### Task 3: Frontend Types and APIClient Method

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/api/client.ts`

- [x] **Step 1: Add types to `frontend/src/types.ts`**

Add to `frontend/src/types.ts`:
```typescript
export interface GenerateTextRequest {
  form_type: 'character' | 'world' | 'system' | 'campaign';
  field_name: string;
  context: Record<string, string>;
  world_id?: string;
  system_id?: string;
  seed?: string;
}

export interface GenerateTextResponse {
  fields: Record<string, string>;
  generated_by: string;
}
```

- [x] **Step 2: Add `generateText` method to `APIClient`**

In `frontend/src/api/client.ts`, add:
```typescript
  static async generateText(payload: GenerateTextRequest): Promise<GenerateTextResponse> {
    const res = await fetch('/api/generate-text', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }
```

- [x] **Step 3: Verify TypeScript compilation**

Run: `cd frontend && npx tsc --noEmit`  
Expected output: Clean exit (0 errors).

- [x] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): add generateText API client and types"
```

---

### Task 4: Reusable `<AIGenerateButton>` Component

**Files:**
- Create: `frontend/src/components/ui/AIGenerateButton.tsx`

- [x] **Step 1: Create `AIGenerateButton` component**

Create `frontend/src/components/ui/AIGenerateButton.tsx`:
```typescript
import React, { useState } from 'react';
import { Sparkles, Loader2 } from 'lucide-react';
import { APIClient } from '../../api/client';
import { GenerateTextRequest } from '../../types';

export interface AIGenerateButtonProps {
  formType: 'character' | 'world' | 'system' | 'campaign';
  fieldName: string;
  getContext: () => Record<string, string>;
  onGenerated: (value: string) => void;
  worldID?: string;
  systemID?: string;
  seed?: string;
  disabled?: boolean;
  className?: string;
  title?: string;
}

export const AIGenerateButton: React.FC<AIGenerateButtonProps> = ({
  formType,
  fieldName,
  getContext,
  onGenerated,
  worldID,
  systemID,
  seed,
  disabled = false,
  className = '',
  title,
}) => {
  const [isGenerating, setIsGenerating] = useState(false);

  const handleGenerate = async (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (isGenerating || disabled) return;

    setIsGenerating(true);
    try {
      const payload: GenerateTextRequest = {
        form_type: formType,
        field_name: fieldName,
        context: getContext(),
        world_id: worldID,
        system_id: systemID,
        seed: seed || undefined,
      };

      const res = await APIClient.generateText(payload);
      if (res.fields && res.fields[fieldName]) {
        onGenerated(res.fields[fieldName]);
      }
    } catch (error) {
      console.error(`Failed to generate text for ${fieldName}:`, error);
    } finally {
      setIsGenerating(false);
    }
  };

  return (
    <button
      type="button"
      onClick={handleGenerate}
      disabled={isGenerating || disabled}
      className={`inline-flex items-center justify-center p-1 rounded-md bg-purple-600/15 hover:bg-purple-600/25 border border-purple-500/30 text-purple-300 hover:text-purple-200 transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed ${className}`}
      title={title || `AI Generate ${fieldName}`}
    >
      {isGenerating ? (
        <Loader2 className="w-3 h-3 animate-spin text-purple-400" />
      ) : (
        <Sparkles className="w-3 h-3 text-purple-300" />
      )}
    </button>
  );
};
```

- [x] **Step 2: Verify TypeScript compilation**

Run: `cd frontend && npx tsc --noEmit`  
Expected output: Clean exit (0 errors).

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/ui/AIGenerateButton.tsx
git commit -m "feat(frontend): create reusable AIGenerateButton component"
```

---

### Task 5: Integrate Text Generation into `NewCampaignModal`

**Files:**
- Modify: `frontend/src/components/launcher/NewCampaignModal.tsx`

- [x] **Step 1: Add context helper, generate-all handler, and button imports**

In `frontend/src/components/launcher/NewCampaignModal.tsx`:
1. Import `AIGenerateButton` and `Wand2`:
```typescript
import { Wand2 } from 'lucide-react';
import { AIGenerateButton } from '../ui/AIGenerateButton';
```
2. In the component, add state and context extractor:
```typescript
  const [isGeneratingAll, setIsGeneratingAll] = useState(false);

  const getFormContext = (): Record<string, string> => ({
    world_name: world?.name || '',
    world_description: world?.description || '',
    world_genre: world?.genre || '',
    campaign_name: campaignName,
    player_name: playerName,
    player_appearance: playerAppearance,
    player_background: playerBackground,
    player_age: playerAge,
    player_gender: playerGender,
    player_pronouns: playerPronouns,
    start_location: startLocation,
    opening_prompt: openingPrompt,
  });

  const handleGenerateAll = async () => {
    if (isGeneratingAll) return;
    setIsGeneratingAll(true);
    try {
      const ctx = getFormContext();
      // Generate campaign directives
      const campRes = await APIClient.generateText({
        form_type: 'campaign',
        field_name: '_all',
        context: ctx,
        world_id: world?.id,
        system_id: selectedSystemID,
      });
      if (campRes.fields.name && !campaignName.trim()) setCampaignName(campRes.fields.name);
      if (campRes.fields.start_location && !startLocation.trim()) setStartLocation(campRes.fields.start_location);
      if (campRes.fields.opening_prompt && !openingPrompt.trim()) setOpeningPrompt(campRes.fields.opening_prompt);

      // Generate character fields
      const charRes = await APIClient.generateText({
        form_type: 'character',
        field_name: '_all',
        context: { ...ctx, ...campRes.fields },
        world_id: world?.id,
        system_id: selectedSystemID,
      });
      if (charRes.fields.name && !playerName.trim()) setPlayerName(charRes.fields.name);
      if (charRes.fields.appearance && !playerAppearance.trim()) setPlayerAppearance(charRes.fields.appearance);
      if (charRes.fields.background && !playerBackground.trim()) setPlayerBackground(charRes.fields.background);
      if (charRes.fields.age && !playerAge.trim()) setPlayerAge(charRes.fields.age);
      if (charRes.fields.gender && !playerGender.trim()) setPlayerGender(charRes.fields.gender);
      if (charRes.fields.pronouns && !playerPronouns.trim()) setPlayerPronouns(charRes.fields.pronouns);
    } catch (err) {
      console.error('Failed to generate all fields:', err);
    } finally {
      setIsGeneratingAll(false);
    }
  };
```

- [x] **Step 2: Add inline `AIGenerateButton` to character and campaign fields**

In `frontend/src/components/launcher/NewCampaignModal.tsx`:
1. Campaign Name field label:
```tsx
                <div className="flex items-center justify-between">
                  <label className="text-xs font-sans font-bold text-white">Campaign Name</label>
                  <AIGenerateButton
                    formType="campaign"
                    fieldName="name"
                    getContext={getFormContext}
                    onGenerated={(val) => setCampaignName(val)}
                    worldID={world.id}
                    systemID={selectedSystemID}
                  />
                </div>
```
2. Protagonist Name label:
```tsx
                <div className="flex items-center justify-between">
                  <label className="text-xs font-sans font-bold text-white">Protagonist Name</label>
                  <AIGenerateButton
                    formType="character"
                    fieldName="name"
                    getContext={getFormContext}
                    onGenerated={(val) => setPlayerName(val)}
                    worldID={world.id}
                    systemID={selectedSystemID}
                  />
                </div>
```
3. Age, Gender, Pronouns labels (each get an inline `<AIGenerateButton>`):
```tsx
                  <div className="flex items-center justify-between">
                    <label className="text-[11px] font-sans text-stone-300">Age</label>
                    <AIGenerateButton
                      formType="character"
                      fieldName="age"
                      getContext={getFormContext}
                      onGenerated={(val) => setPlayerAge(val)}
                      worldID={world.id}
                      systemID={selectedSystemID}
                    />
                  </div>
```
(Apply the same pattern to Gender with `fieldName="gender"` and Pronouns with `fieldName="pronouns"`).

4. Appearance and Background textarea labels:
```tsx
                <div className="flex items-center justify-between">
                  <label className="text-[11px] font-sans text-stone-300">Appearance</label>
                  <AIGenerateButton
                    formType="character"
                    fieldName="appearance"
                    getContext={getFormContext}
                    onGenerated={(val) => setPlayerAppearance(val)}
                    worldID={world.id}
                    systemID={selectedSystemID}
                    seed={playerAppearance}
                  />
                </div>
```
(Apply the same pattern to Background with `fieldName="background"` and `seed={playerBackground}`).

5. Start Location and Opening Scene Prompt labels:
```tsx
                <div className="flex items-center justify-between">
                  <label className="text-[11px] font-sans text-stone-300">Start Location Directive</label>
                  <AIGenerateButton
                    formType="campaign"
                    fieldName="start_location"
                    getContext={getFormContext}
                    onGenerated={(val) => setStartLocation(val)}
                    worldID={world.id}
                    systemID={selectedSystemID}
                  />
                </div>
```
(Apply the same pattern to Opening Scene with `fieldName="opening_prompt"`).

6. Add the "Auto-Fill All" button in the footer actions:
```tsx
            <button
              type="button"
              onClick={handleGenerateAll}
              disabled={isSubmitting || isGeneratingAll}
              className="px-3 py-2 rounded-xl border border-purple-500/30 bg-purple-600/10 hover:bg-purple-600/20 text-purple-300 text-xs font-sans font-semibold flex items-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
            >
              <Wand2 className={`w-3.5 h-3.5 ${isGeneratingAll ? 'animate-spin' : ''}`} />
              <span>{isGeneratingAll ? 'Generating...' : 'Auto-Fill Fields'}</span>
            </button>
```

- [x] **Step 3: Verify TypeScript compilation**

Run: `cd frontend && npx tsc --noEmit`  
Expected output: Clean exit (0 errors).

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/launcher/NewCampaignModal.tsx
git commit -m "feat(frontend): integrate AI text generation into NewCampaignModal"
```

---

### Task 6: Integrate Text Generation into `CampaignSettingsModal`

**Files:**
- Modify: `frontend/src/components/launcher/CampaignSettingsModal.tsx`

- [x] **Step 1: Add `AIGenerateButton` to directives in `CampaignSettingsModal`**

In `frontend/src/components/launcher/CampaignSettingsModal.tsx`:
1. Import `AIGenerateButton`:
```typescript
import { AIGenerateButton } from '../ui/AIGenerateButton';
```
2. Add context helper:
```typescript
  const getCampaignContext = (): Record<string, string> => ({
    campaign_name: game.name,
    start_location: startLocation,
    opening_prompt: openingPrompt,
  });
```
3. Update the Start Location label:
```tsx
            <div className="space-y-1">
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-sans text-stone-300 flex items-center gap-1">
                  <MapPin className="w-3 h-3 text-stone-400" />
                  <span>Start Location Directive</span>
                </label>
                <AIGenerateButton
                  formType="campaign"
                  fieldName="start_location"
                  getContext={getCampaignContext}
                  onGenerated={(val) => setStartLocation(val)}
                  worldID={game.world_id}
                  systemID={game.system_id}
                />
              </div>
              <input
                type="text"
                value={startLocation}
                onChange={(e) => setStartLocation(e.target.value)}
                placeholder="e.g. Old Harbour District"
                className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-2 text-xs text-stone-200 focus:outline-none focus:border-purple-500"
              />
            </div>
```
4. Update the Opening Scene Prompt label:
```tsx
            <div className="space-y-1">
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-sans text-stone-300 flex items-center gap-1">
                  <ScrollText className="w-3 h-3 text-stone-400" />
                  <span>Opening Scene Prompt</span>
                </label>
                <AIGenerateButton
                  formType="campaign"
                  fieldName="opening_prompt"
                  getContext={getCampaignContext}
                  onGenerated={(val) => setOpeningPrompt(val)}
                  worldID={game.world_id}
                  systemID={game.system_id}
                  seed={openingPrompt}
                />
              </div>
              <textarea
                value={openingPrompt}
                onChange={(e) => setOpeningPrompt(e.target.value)}
                rows={3}
                placeholder="Custom instruction for turn 1..."
                className="w-full bg-stone-900 border border-white/10 rounded-xl px-3 py-2 text-xs text-stone-200 focus:outline-none focus:border-purple-500 resize-none"
              />
            </div>
```

- [x] **Step 2: Verify TypeScript compilation**

Run: `cd frontend && npx tsc --noEmit`  
Expected output: Clean exit (0 errors).

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/launcher/CampaignSettingsModal.tsx
git commit -m "feat(frontend): integrate AI text generation into CampaignSettingsModal"
```

---

### Task 7: Integrate Text Generation into `WorldsStudio`

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`

- [x] **Step 1: Add context helper and auto-fill handler to `WorldsStudio`**

In `frontend/src/components/WorldsStudio.tsx`:
1. Import `AIGenerateButton` and `Wand2`:
```typescript
import { Wand2 } from 'lucide-react';
import { AIGenerateButton } from './ui/AIGenerateButton';
```
2. Add context helper and form-wide generator:
```typescript
  const [isGeneratingAll, setIsGeneratingAll] = useState(false);

  const getWorldContext = (): Record<string, string> => ({
    name,
    genre,
    art_style: artStyle,
    description,
    lore_prompt: lorePrompt,
    tags,
  });

  const handleGenerateAllWorldFields = async () => {
    if (isGeneratingAll) return;
    setIsGeneratingAll(true);
    try {
      const res = await APIClient.generateText({
        form_type: 'world',
        field_name: '_all',
        context: getWorldContext(),
        system_id: defaultSystem,
      });
      if (res.fields.name && !name.trim()) setName(res.fields.name);
      if (res.fields.genre && !genre.trim()) setGenre(res.fields.genre);
      if (res.fields.art_style && !artStyle.trim()) setArtStyle(res.fields.art_style);
      if (res.fields.description && !description.trim()) setDescription(res.fields.description);
      if (res.fields.lore_prompt && !lorePrompt.trim()) setLorePrompt(res.fields.lore_prompt);
      setToast({ type: 'success', message: 'Auto-filled world fields!' });
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || 'Auto-fill failed' });
    } finally {
      setIsGeneratingAll(false);
    }
  };
```

- [x] **Step 2: Add inline `AIGenerateButton` to World inputs and "Auto-Fill" button to header**

1. Add Auto-Fill button to the tab header toolbar:
```tsx
            <button
              type="button"
              onClick={handleGenerateAllWorldFields}
              disabled={isGeneratingAll || isSaving}
              title="Auto-fill empty world fields with AI"
              className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-purple-500/40 bg-purple-600/15 hover:bg-purple-600/25 text-purple-300 transition-all cursor-pointer disabled:opacity-50"
            >
              <Wand2 className={`w-3.5 h-3.5 ${isGeneratingAll ? 'animate-spin' : ''}`} />
              <span className="hidden sm:inline">{isGeneratingAll ? 'Generating...' : 'Auto-Fill'}</span>
            </button>
```

2. Add `<AIGenerateButton>` inline with each field label:
- World Setting Name:
```tsx
                <div className="flex items-center justify-between">
                  <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                    World Setting Name
                  </label>
                  <AIGenerateButton
                    formType="world"
                    fieldName="name"
                    getContext={getWorldContext}
                    onGenerated={(val) => {
                      setName(val);
                      if (!selectedID) {
                        setSlugID(val.toLowerCase().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, ''));
                      }
                    }}
                    systemID={defaultSystem}
                  />
                </div>
```
- Genre / Setting Style:
```tsx
                <div className="flex items-center justify-between">
                  <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                    Genre / Setting Style
                  </label>
                  <AIGenerateButton
                    formType="world"
                    fieldName="genre"
                    getContext={getWorldContext}
                    onGenerated={(val) => setGenre(val)}
                    systemID={defaultSystem}
                  />
                </div>
```
- Art Style:
```tsx
                <div className="flex items-center justify-between">
                  <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                    Visual Art Style Directive
                  </label>
                  <AIGenerateButton
                    formType="world"
                    fieldName="art_style"
                    getContext={getWorldContext}
                    onGenerated={(val) => setArtStyle(val)}
                    systemID={defaultSystem}
                    seed={artStyle}
                  />
                </div>
```
- World Description:
```tsx
              <div className="flex items-center justify-between">
                <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                  World Description
                </label>
                <AIGenerateButton
                  formType="world"
                  fieldName="description"
                  getContext={getWorldContext}
                  onGenerated={(val) => setDescription(val)}
                  systemID={defaultSystem}
                  seed={description}
                />
              </div>
```
- Lore Prompt (in `activeTab === 'prompt'`):
```tsx
            <div className="flex items-center justify-between">
              <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                World Lore Prompt
              </label>
              <AIGenerateButton
                formType="world"
                fieldName="lore_prompt"
                getContext={getWorldContext}
                onGenerated={(val) => setLorePrompt(val)}
                systemID={defaultSystem}
                seed={lorePrompt}
              />
            </div>
```

- [x] **Step 3: Verify TypeScript compilation**

Run: `cd frontend && npx tsc --noEmit`  
Expected output: Clean exit (0 errors).

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx
git commit -m "feat(frontend): integrate AI text generation into WorldsStudio"
```

---

### Task 8: Integrate Text Generation into `SystemsStudio`

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx`

- [x] **Step 1: Add context helper and auto-fill handler to `SystemsStudio`**

In `frontend/src/components/SystemsStudio.tsx`:
1. Import `AIGenerateButton` and `Wand2`:
```typescript
import { Wand2 } from 'lucide-react';
import { AIGenerateButton } from './ui/AIGenerateButton';
```
2. Add context helper and form-wide generator:
```typescript
  const [isGeneratingAll, setIsGeneratingAll] = useState(false);

  const getSystemContext = (): Record<string, string> => ({
    name,
    description,
    rules_prompt: rulesPrompt,
  });

  const handleGenerateAllSystemFields = async () => {
    if (isGeneratingAll) return;
    setIsGeneratingAll(true);
    try {
      const res = await APIClient.generateText({
        form_type: 'system',
        field_name: '_all',
        context: getSystemContext(),
      });
      if (res.fields.name && !name.trim()) setName(res.fields.name);
      if (res.fields.description && !description.trim()) setDescription(res.fields.description);
      if (res.fields.rules_prompt && !rulesPrompt.trim()) setRulesPrompt(res.fields.rules_prompt);
      setToast({ type: 'success', message: 'Auto-filled system fields!' });
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || 'Auto-fill failed' });
    } finally {
      setIsGeneratingAll(false);
    }
  };
```

- [x] **Step 2: Add inline `AIGenerateButton` to System inputs and "Auto-Fill" button to header**

1. Add Auto-Fill button to the tab header toolbar:
```tsx
            <button
              type="button"
              onClick={handleGenerateAllSystemFields}
              disabled={isGeneratingAll || isSaving}
              title="Auto-fill empty system fields with AI"
              className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-purple-500/40 bg-purple-600/15 hover:bg-purple-600/25 text-purple-300 transition-all cursor-pointer disabled:opacity-50"
            >
              <Wand2 className={`w-3.5 h-3.5 ${isGeneratingAll ? 'animate-spin' : ''}`} />
              <span className="hidden sm:inline">{isGeneratingAll ? 'Generating...' : 'Auto-Fill'}</span>
            </button>
```

2. Add `<AIGenerateButton>` inline with each field label:
- System Name:
```tsx
                <div className="flex items-center justify-between">
                  <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                    System Name
                  </label>
                  <AIGenerateButton
                    formType="system"
                    fieldName="name"
                    getContext={getSystemContext}
                    onGenerated={(val) => {
                      setName(val);
                      if (!selectedID) {
                        setSlugID(val.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''));
                      }
                    }}
                  />
                </div>
```
- System Description:
```tsx
              <div className="flex items-center justify-between">
                <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                  System Description
                </label>
                <AIGenerateButton
                  formType="system"
                  fieldName="description"
                  getContext={getSystemContext}
                  onGenerated={(val) => setDescription(val)}
                  seed={description}
                />
              </div>
```
- Rules Prompt (in `activeTab === 'rules'`):
```tsx
            <div className="flex items-center justify-between">
              <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                Core Rules Prompt (rules.md)
              </label>
              <AIGenerateButton
                formType="system"
                fieldName="rules_prompt"
                getContext={getSystemContext}
                onGenerated={(val) => setRulesPrompt(val)}
                seed={rulesPrompt}
              />
            </div>
```

- [x] **Step 3: Verify TypeScript compilation**

Run: `cd frontend && npx tsc --noEmit`  
Expected output: Clean exit (0 errors).

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/SystemsStudio.tsx
git commit -m "feat(frontend): integrate AI text generation into SystemsStudio"
```

---

### Task 9: Full Verification Suite

**Files:**
- N/A

- [x] **Step 1: Run Go linter and test suite**

Run: `go vet ./... && go test -count=1 ./...`  
Expected output: PASS

- [x] **Step 2: Run frontend typecheck and bundle build**

Run: `cd frontend && npx tsc --noEmit && npm run build`  
Expected output: PASS (dist generated, .gitkeep touched)
