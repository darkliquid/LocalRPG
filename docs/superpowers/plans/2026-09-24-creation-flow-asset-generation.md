# Creation Flow AI Asset Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide AI-generated banner and icon previews during the creation flows for new campaigns and worlds, while defaulting Worlds Studio to a clean blank slate with an opt-in reference template button.

**Architecture:** A new stateless endpoint `POST /api/generate-asset-preview` renders image bytes in-memory from form metadata without persisting to disk, returning a standard MIME type detected from image bytes. Prompt construction is extracted into a shared helper used by game, world, and preview endpoints. The frontend captures generated images as `Blob`s and uploads them via existing asset upload endpoints upon form creation, while `WorldsStudio` starts with empty inputs and a "Load Reference Template" button.

**Tech Stack:** Go 1.27.1 (`net/http`, `pkg/media`, `pkg/gui`), React 19 + TypeScript + Tailwind v4, Lucide icons (`Sparkles`, `Upload`, `BookOpen`).

---

### Task 1: Extract Shared Prompt Builder with Unit Tests

**Files:**
- Modify: `pkg/gui/service.go:2657-2728`
- Create: `pkg/gui/asset_prompt_test.go`

- [x] **Step 1: Write the failing prompt builder test**

Create `pkg/gui/asset_prompt_test.go`:
```go
package gui

import (
	"strings"
	"testing"
)

func TestBuildAssetPrompt(t *testing.T) {
	tests := []struct {
		name        string
		kind        string
		entityName  string
		description string
		artStyle    string
		genre       string
		wantSubstrs []string
	}{
		{
			name:        "game banner",
			kind:        "banner",
			entityName:  "Shadow over Hollowmere",
			description: "The Sunken Realm",
			artStyle:    "Dark Gothic Oil Painting",
			genre:       "",
			wantSubstrs: []string{"Dark Gothic Oil Painting", "Shadow over Hollowmere", "The Sunken Realm", "widescreen cinematic"},
		},
		{
			name:        "game icon",
			kind:        "icon",
			entityName:  "Shadow over Hollowmere",
			description: "The Sunken Realm",
			artStyle:    "Pixel Art",
			genre:       "",
			wantSubstrs: []string{"Pixel Art", "Shadow over Hollowmere", "The Sunken Realm", "app icon emblem"},
		},
		{
			name:        "world banner",
			kind:        "banner",
			entityName:  "Ember Peak",
			description: "A scorched volcanic crater",
			artStyle:    "Watercolour",
			genre:       "High Fantasy",
			wantSubstrs: []string{"Watercolour", "Ember Peak", "High Fantasy", "panoramic"},
		},
		{
			name:        "world icon",
			kind:        "icon",
			entityName:  "Ember Peak",
			description: "A scorched volcanic crater",
			artStyle:    "Vector Minimalist",
			genre:       "High Fantasy",
			wantSubstrs: []string{"Vector Minimalist", "Ember Peak", "High Fantasy", "badge"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAssetPrompt(tt.kind, tt.entityName, tt.description, tt.artStyle, tt.genre)
			for _, sub := range tt.wantSubstrs {
				if !strings.Contains(got, sub) {
					t.Errorf("buildAssetPrompt() missing substring %q in prompt: %q", sub, got)
				}
			}
		})
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestBuildAssetPrompt ./pkg/gui/`  
Expected output: FAIL (`undefined: buildAssetPrompt`)

- [x] **Step 3: Implement `buildAssetPrompt` and refactor existing generators**

In `pkg/gui/service.go`, add `buildAssetPrompt` and update `GenerateGameAsset` and `GenerateWorldAsset`:
```go
func buildAssetPrompt(kind, name, description, artStyle, genre string) string {
	if genre == "" {
		// Campaign/Game context: description is world name
		if kind == "icon" {
			return fmt.Sprintf("%s game app icon emblem for %s in %s, high contrast vector emblem, centered dark backdrop", artStyle, name, description)
		}
		return fmt.Sprintf("%s widescreen cinematic concept art landscape for %s in %s, highly detailed masterpiece environment", artStyle, name, description)
	}
	// World context: genre is provided
	if kind == "icon" {
		return fmt.Sprintf("%s emblem icon badge for world %s (%s), %s, clean centered icon", artStyle, name, genre, description)
	}
	return fmt.Sprintf("%s widescreen landscape banner concept art for world %s (%s), %s, atmospheric panoramic background", artStyle, name, genre, description)
}
```

In `GenerateGameAsset`:
Replace the manual `if req.Kind == "icon" { ... } else { ... }` block with:
```go
	if prompt == "" {
		gameDir := s.resolver.GameDir(gameID)
		manifest, _ := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
		gameName := gameID
		worldName := ""
		artStyle := ""
		if manifest != nil {
			if manifest.Name != "" {
				gameName = manifest.Name
			}
			worldDir := s.resolver.WorldDir(manifest.WorldID)
			if wm, err := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml")); err == nil {
				worldName = wm.Name
				artStyle = wm.ArtStyle
			}
		}
		prompt = buildAssetPrompt(req.Kind, gameName, worldName, artStyle, "")
	}
```

In `GenerateWorldAsset`:
Replace the manual prompt construction with:
```go
	if prompt == "" {
		worldDir := s.resolver.WorldDir(worldID)
		wm, _ := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml"))
		worldName := worldID
		artStyle := ""
		genre := ""
		desc := ""
		if wm != nil {
			if wm.Name != "" {
				worldName = wm.Name
			}
			artStyle = wm.ArtStyle
			genre = wm.Genre
			desc = wm.Description
		}
		prompt = buildAssetPrompt(req.Kind, worldName, desc, artStyle, genre)
	}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestBuildAssetPrompt ./pkg/gui/`  
Expected output: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/asset_prompt_test.go
git commit -m "feat(gui): extract shared prompt builder for asset generation"
```

---

### Task 2: Backend `POST /api/generate-asset-preview` Endpoint

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/server.go`
- Create: `pkg/gui/asset_preview_test.go`

- [x] **Step 1: Write failing test for the preview endpoint**

Create `pkg/gui/asset_preview_test.go`:
```go
package gui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
)

func TestHandleGenerateAssetPreview(t *testing.T) {
	tempDir := t.TempDir()
	resolver := core.NewPathResolver(tempDir, tempDir, tempDir, tempDir)
	cfgMgr := config.NewManager(tempDir)
	cfg := cfgMgr.Get()
	cfg.Media.Image.Type = "builtin"
	cfg.Media.Image.BuiltinName = "procedural-art"
	if err := cfgMgr.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	service := NewService(tempDir)
	server := NewServer(service, nil, "127.0.0.1:0")

	body, _ := json.Marshal(GenerateAssetPreviewRequestDTO{
		Kind:        "banner",
		Name:        "Test Realm",
		Description: "A misty land",
		ArtStyle:    "Dark Fantasy",
		Genre:       "Gothic",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/generate-asset-preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	contentType := w.Header().Get("Content-Type")
	if contentType == "" {
		t.Errorf("expected Content-Type header on image response")
	}
	if w.Body.Len() == 0 {
		t.Errorf("expected non-empty image body")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestHandleGenerateAssetPreview ./pkg/gui/`  
Expected output: FAIL (`404 page not found` or `undefined: GenerateAssetPreviewRequestDTO`)

- [x] **Step 3: Implement DTO, Service Method, and Server Handler**

In `pkg/gui/service.go`, define the DTO:
```go
type GenerateAssetPreviewRequestDTO struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ArtStyle    string `json:"art_style"`
	Genre       string `json:"genre,omitempty"`
}

func (s *Service) GenerateAssetPreview(ctx context.Context, req GenerateAssetPreviewRequestDTO) ([]byte, string, error) {
	cfg := s.configMgr.Get()
	client, err := media.NewImageClientWithSharedKey(cfg.Media.Image, cfg.Providers.Gemini.APIKey)
	if err != nil {
		return nil, "", fmt.Errorf("image provider: %w", err)
	}
	prompt := buildAssetPrompt(req.Kind, req.Name, req.Description, req.ArtStyle, req.Genre)
	imgBytes, err := client.GenerateImage(ctx, prompt)
	if err != nil {
		return nil, "", fmt.Errorf("generate image: %w", err)
	}

	ext := media.ArtExtension(imgBytes)
	contentType := "application/octet-stream"
	switch ext {
	case ".svg":
		contentType = "image/svg+xml"
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".webp":
		contentType = "image/webp"
	}

	return imgBytes, contentType, nil
}
```

In `pkg/gui/server.go`:
Register the route in `registerRoutes()`:
```go
s.mux.HandleFunc("/api/generate-asset-preview", s.handleGenerateAssetPreview)
```

And add the handler function in `pkg/gui/server.go`:
```go
func (s *Server) handleGenerateAssetPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req GenerateAssetPreviewRequestDTO
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Kind != "banner" && req.Kind != "icon" {
		http.Error(w, "kind must be 'banner' or 'icon'", http.StatusBadRequest)
		return
	}
	data, contentType, err := s.service.GenerateAssetPreview(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestHandleGenerateAssetPreview ./pkg/gui/`  
Expected output: PASS

- [x] **Step 5: Verify full package and commit**

Run: `go vet ./... && go test -count=1 ./pkg/gui/`  
Expected output: PASS  
```bash
git add pkg/gui/service.go pkg/gui/server.go pkg/gui/asset_preview_test.go
git commit -m "feat(gui): add POST /api/generate-asset-preview endpoint"
```

---

### Task 3: Frontend API Client Support

**Files:**
- Modify: `frontend/src/api/client.ts`

- [x] **Step 1: Add `generateAssetPreview` method to `APIClient`**

In `frontend/src/api/client.ts`, add after `generateWorldAsset`:
```typescript
  static async generateAssetPreview(
    kind: 'banner' | 'icon',
    name: string,
    description: string,
    artStyle: string,
    genre: string = ''
  ): Promise<Blob> {
    const res = await fetch('/api/generate-asset-preview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ kind, name, description, art_style: artStyle, genre }),
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.blob();
  }
```

- [x] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`  
Expected output: Clean exit (0 errors).

- [x] **Step 3: Commit**

```bash
git add frontend/src/api/client.ts
git commit -m "feat(frontend): add generateAssetPreview to APIClient"
```

---

### Task 4: Add AI Generation Buttons to `NewCampaignModal`

**Files:**
- Modify: `frontend/src/components/launcher/NewCampaignModal.tsx`

- [x] **Step 1: Add generating state and handler to `NewCampaignModal`**

In `frontend/src/components/launcher/NewCampaignModal.tsx`:
1. Import `Sparkles`:
```typescript
import { Upload, Sparkles } from 'lucide-react';
```
2. In the component state declarations, add:
```typescript
  const [generatingKind, setGeneratingKind] = useState<'banner' | 'icon' | null>(null);
  const [genError, setGenError] = useState<string | null>(null);
```
3. Add the generator handler:
```typescript
  const handleAIGenerate = async (kind: 'banner' | 'icon') => {
    if (!world) return;
    setGeneratingKind(kind);
    setGenError(null);
    try {
      const blob = await APIClient.generateAssetPreview(
        kind,
        campaignName.trim() || world.name,
        world.description || '',
        world.art_style || ''
      );
      const file = new File([blob], `${kind}.png`, { type: blob.type });
      if (kind === 'banner') {
        setBannerFile(file);
        setBannerPreview(URL.createObjectURL(blob));
      } else {
        setIconFile(file);
        setIconPreview(URL.createObjectURL(blob));
      }
    } catch (err: any) {
      console.error('Failed to generate preview', err);
      setGenError(err.message || 'Generation failed');
    } finally {
      setGeneratingKind(null);
    }
  };
```

- [x] **Step 2: Replace Artwork upload buttons with Upload + AI Gen pair**

In `frontend/src/components/launcher/NewCampaignModal.tsx`, update the Custom Artwork section:
```tsx
          {/* Custom Artwork (Banner & Icon Uploads) */}
          <div className="p-3.5 bg-stone-950/70 border border-white/10 rounded-2xl space-y-3">
            <div>
              <div className="text-xs font-sans font-bold text-white">Custom Campaign Artwork (Optional)</div>
              <div className="text-[11px] font-sans text-stone-400">
                A procedural gradient theme will be generated if omitted.
              </div>
              {genError && (
                <div className="text-[11px] text-red-400 font-sans mt-1">{genError}</div>
              )}
            </div>

            <div className="grid grid-cols-2 gap-3 pt-1">
              {/* Banner Upload & Generate */}
              <div className="space-y-2">
                <input
                  type="file"
                  ref={bannerInputRef}
                  onChange={handleBannerSelect}
                  accept="image/png,image/jpeg,image/webp,image/svg+xml"
                  className="hidden"
                />
                <div
                  onClick={() => bannerInputRef.current?.click()}
                  className="w-full h-16 rounded-xl border border-dashed border-white/20 hover:border-purple-400/60 bg-white/[0.02] flex items-center justify-center text-xs font-sans text-stone-300 hover:text-white transition-all cursor-pointer overflow-hidden"
                >
                  {bannerPreview ? (
                    <img src={bannerPreview} alt="Banner Preview" className="w-full h-full object-cover" />
                  ) : (
                    <span className="text-stone-500 text-[11px]">No Banner Selected</span>
                  )}
                </div>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => bannerInputRef.current?.click()}
                    disabled={isSubmitting || generatingKind === 'banner'}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                  >
                    <Upload className="w-3.5 h-3.5" />
                    <span>Upload</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => handleAIGenerate('banner')}
                    disabled={isSubmitting || generatingKind === 'banner'}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                  >
                    <Sparkles className="w-3.5 h-3.5" />
                    <span>{generatingKind === 'banner' ? 'Gen...' : 'AI Gen'}</span>
                  </button>
                </div>
              </div>

              {/* Icon Upload & Generate */}
              <div className="space-y-2">
                <input
                  type="file"
                  ref={iconInputRef}
                  onChange={handleIconSelect}
                  accept="image/png,image/jpeg,image/webp,image/svg+xml"
                  className="hidden"
                />
                <div
                  onClick={() => iconInputRef.current?.click()}
                  className="w-full h-16 rounded-xl border border-dashed border-white/20 hover:border-purple-400/60 bg-white/[0.02] flex items-center justify-center text-xs font-sans text-stone-300 hover:text-white transition-all cursor-pointer overflow-hidden"
                >
                  {iconPreview ? (
                    <img src={iconPreview} alt="Icon Preview" className="w-12 h-12 rounded-lg object-cover" />
                  ) : (
                    <span className="text-stone-500 text-[11px]">No Icon Selected</span>
                  )}
                </div>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => iconInputRef.current?.click()}
                    disabled={isSubmitting || generatingKind === 'icon'}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                  >
                    <Upload className="w-3.5 h-3.5" />
                    <span>Upload</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => handleAIGenerate('icon')}
                    disabled={isSubmitting || generatingKind === 'icon'}
                    className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                  >
                    <Sparkles className="w-3.5 h-3.5" />
                    <span>{generatingKind === 'icon' ? 'Gen...' : 'AI Gen'}</span>
                  </button>
                </div>
              </div>
            </div>
          </div>
```

- [x] **Step 3: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`  
Expected output: Clean exit (0 errors).

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/launcher/NewCampaignModal.tsx
git commit -m "feat(frontend): add AI Gen buttons for banner and icon in NewCampaignModal"
```

---

### Task 5: Default Worlds Studio to Blank Slate & Add "Load Reference Template"

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`

- [x] **Step 1: Update initial state to blank and update generic starter entity**

In `frontend/src/components/WorldsStudio.tsx`:
Replace `STARTER_ENTITY_TEMPLATE`:
```typescript
const STARTER_ENTITY_TEMPLATE = `---
name: New Location
type: location
state:
  danger_level: 1
wikilinks: []
---
An intriguing location waiting to be explored.
`;
```

Update initial state (lines 28–53) to be blank by default:
```typescript
  // World form state (defaults to blank slate)
  const [name, setName] = useState('');
  const [slugID, setSlugID] = useState('');
  const [genre, setGenre] = useState('');
  const [defaultSystem, setDefaultSystem] = useState('');
  const [artStyle, setArtStyle] = useState('');
  const [tags, setTags] = useState('');
  const [description, setDescription] = useState('');
  const [lorePrompt, setLorePrompt] = useState('');

  // Entities state
  const [entities, setEntities] = useState<WorldEntitySummary[]>([]);
  const [selectedEntityID, setSelectedEntityID] = useState<string | null>(null);
  const [entityMarkdown, setEntityMarkdown] = useState('');
  const [entityDrafts, setEntityDrafts] = useState<Record<string, string>>({});
  const [isNewEntityModal, setIsNewEntityModal] = useState(false);
  const [newEntitySlug, setNewEntitySlug] = useState('');
```

Update `handleNewWorld` to clear fields rather than repopulate Ashen Reach:
```typescript
  const handleNewWorld = (sysList?: SystemInfo[]) => {
    setSelectedID(null);
    setName('');
    setSlugID('');
    setGenre('');
    const availableSys = sysList && sysList.length > 0 ? sysList : systems;
    setDefaultSystem(availableSys[0]?.id ?? '');
    setArtStyle('');
    setTags('');
    setDescription('');
    setLorePrompt('');
    setEntities([]);
    setSelectedEntityID(null);
    setEntityMarkdown('');
    setEntityDrafts({});
    setBannerFile(null);
    setIconFile(null);
    setBannerPreview(null);
    setIconPreview(null);
    setActiveTab('lore');
  };
```

Update `handleResetToReference` / rename button to `Load Reference Template`:
```typescript
  const handleLoadReferenceTemplate = () => {
    setName(REFERENCE_WORLD_TEMPLATE.name);
    if (!selectedID) {
      setSlugID(REFERENCE_WORLD_TEMPLATE.id);
    }
    setGenre(REFERENCE_WORLD_TEMPLATE.genre);
    const matchingSys = systems.find((s) => s.id === REFERENCE_WORLD_TEMPLATE.default_system);
    if (matchingSys) setDefaultSystem(matchingSys.id);
    setArtStyle(REFERENCE_WORLD_TEMPLATE.art_style);
    setTags(REFERENCE_WORLD_TEMPLATE.tags.join(', '));
    setDescription(REFERENCE_WORLD_TEMPLATE.description);
    setLorePrompt(REFERENCE_WORLD_TEMPLATE.lore_prompt);
    setEntities(
      REFERENCE_WORLD_TEMPLATE.entities.map((e) => ({ id: e.id, name: e.name, type: e.type }))
    );
    setSelectedEntityID(REFERENCE_WORLD_TEMPLATE.entities[0].id);
    setEntityMarkdown(REFERENCE_WORLD_TEMPLATE.entities[0].markdown);
    const initialDrafts: Record<string, string> = {};
    REFERENCE_WORLD_TEMPLATE.entities.forEach((e) => {
      initialDrafts[e.id] = e.markdown;
    });
    setEntityDrafts(initialDrafts);
    setToast({ type: 'success', message: 'Loaded The Ashen Reach reference template' });
  };
```

Update the header button in `WorldsStudio.tsx`:
```tsx
            <button
              type="button"
              onClick={handleLoadReferenceTemplate}
              title="Load the comprehensive Ashen Reach reference template"
              className="flex items-center gap-1.5 text-xs font-sans px-3 py-2 rounded-xl border border-stone-800 hover:border-purple-500/50 bg-stone-900/60 hover:bg-stone-800 text-stone-300 hover:text-purple-400 transition-all cursor-pointer"
            >
              <BookOpen className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">Load Reference Template</span>
            </button>
```

- [x] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`  
Expected output: Clean exit (0 errors).

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx
git commit -m "feat(frontend): default WorldsStudio to blank slate with opt-in reference template button"
```

---

### Task 6: Add Artwork Management and AI Generation to `WorldsStudio`

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`

- [x] **Step 1: Add artwork state, refs, and handlers in `WorldsStudio`**

In `frontend/src/components/WorldsStudio.tsx`:
1. Import `Upload`, `Sparkles`:
```typescript
import { Globe, Plus, Save, Info, FileText, Check, AlertCircle, Trash2, Tag, Palette, RotateCcw, BookOpen, Upload, Sparkles } from 'lucide-react';
```
2. Add artwork state and file refs:
```typescript
  const [bannerFile, setBannerFile] = useState<File | null>(null);
  const [iconFile, setIconFile] = useState<File | null>(null);
  const [bannerPreview, setBannerPreview] = useState<string | null>(null);
  const [iconPreview, setIconPreview] = useState<string | null>(null);
  const [generatingKind, setGeneratingKind] = useState<'banner' | 'icon' | null>(null);

  const bannerInputRef = React.useRef<HTMLInputElement>(null);
  const iconInputRef = React.useRef<HTMLInputElement>(null);
```
3. In `handleSelectWorld(worldId)`: reset files and set existing asset URLs:
```typescript
  setBannerFile(null);
  setIconFile(null);
  setBannerPreview(`/api/world/${encodeURIComponent(worldId)}/banner?t=${Date.now()}`);
  setIconPreview(`/api/world/${encodeURIComponent(worldId)}/icon?t=${Date.now()}`);
```
4. Add generation handler:
```typescript
  const handleAIGenerate = async (kind: 'banner' | 'icon') => {
    setGeneratingKind(kind);
    try {
      if (selectedID) {
        await APIClient.generateWorldAsset(selectedID, kind);
        if (kind === 'banner') {
          setBannerPreview(`/api/world/${encodeURIComponent(selectedID)}/banner?t=${Date.now()}`);
        } else {
          setIconPreview(`/api/world/${encodeURIComponent(selectedID)}/icon?t=${Date.now()}`);
        }
        setToast({ type: 'success', message: `Generated world ${kind}!` });
      } else {
        const blob = await APIClient.generateAssetPreview(
          kind,
          name.trim() || 'New World',
          description.trim(),
          artStyle.trim(),
          genre.trim()
        );
        const file = new File([blob], `${kind}.png`, { type: blob.type });
        if (kind === 'banner') {
          setBannerFile(file);
          setBannerPreview(URL.createObjectURL(blob));
        } else {
          setIconFile(file);
          setIconPreview(URL.createObjectURL(blob));
        }
        setToast({ type: 'success', message: `Previewed world ${kind}!` });
      }
    } catch (err: any) {
      setToast({ type: 'error', message: err.message || `Failed to generate ${kind}` });
    } finally {
      setGeneratingKind(null);
    }
  };
```
5. In `handleSaveWorld`, upload any staged `bannerFile` / `iconFile`:
```typescript
      if (bannerFile) {
        await APIClient.uploadWorldAsset(saved.id, 'banner', bannerFile).catch(console.error);
      }
      if (iconFile) {
        await APIClient.uploadWorldAsset(saved.id, 'icon', iconFile).catch(console.error);
      }
```

- [x] **Step 2: Render Artwork section in the `activeTab === 'lore'` view**

In `frontend/src/components/WorldsStudio.tsx`, right after the Description textarea:
```tsx
            {/* World Artwork (Banner & Icon) */}
            <div className="space-y-3 pt-2">
              <label className="text-xs font-sans uppercase tracking-wider text-stone-300">
                World Artwork
              </label>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                {/* Banner */}
                <div className="p-3 bg-stone-950 border border-stone-800 rounded-xl space-y-2">
                  <span className="text-[11px] font-sans font-semibold text-stone-400">World Banner</span>
                  <input
                    type="file"
                    ref={bannerInputRef}
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) {
                        setBannerFile(file);
                        setBannerPreview(URL.createObjectURL(file));
                      }
                    }}
                    accept="image/png,image/jpeg,image/webp,image/svg+xml"
                    className="hidden"
                  />
                  <div
                    onClick={() => bannerInputRef.current?.click()}
                    className="h-24 rounded-lg overflow-hidden border border-stone-800 bg-stone-900/40 flex items-center justify-center cursor-pointer"
                  >
                    {bannerPreview ? (
                      <img
                        src={bannerPreview}
                        alt="Banner Preview"
                        className="w-full h-full object-cover"
                        onError={() => setBannerPreview(null)}
                      />
                    ) : (
                      <span className="text-stone-600 text-xs">Click to upload banner</span>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={() => bannerInputRef.current?.click()}
                      disabled={isSaving || generatingKind === 'banner'}
                      className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                    >
                      <Upload className="w-3.5 h-3.5" />
                      <span>Upload</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => handleAIGenerate('banner')}
                      disabled={isSaving || generatingKind === 'banner'}
                      className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                    >
                      <Sparkles className="w-3.5 h-3.5" />
                      <span>{generatingKind === 'banner' ? 'Gen...' : 'AI Gen'}</span>
                    </button>
                  </div>
                </div>

                {/* Icon */}
                <div className="p-3 bg-stone-950 border border-stone-800 rounded-xl space-y-2">
                  <span className="text-[11px] font-sans font-semibold text-stone-400">World Icon</span>
                  <input
                    type="file"
                    ref={iconInputRef}
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) {
                        setIconFile(file);
                        setIconPreview(URL.createObjectURL(file));
                      }
                    }}
                    accept="image/png,image/jpeg,image/webp,image/svg+xml"
                    className="hidden"
                  />
                  <div
                    onClick={() => iconInputRef.current?.click()}
                    className="h-24 rounded-lg overflow-hidden border border-stone-800 bg-stone-900/40 flex items-center justify-center cursor-pointer"
                  >
                    {iconPreview ? (
                      <img
                        src={iconPreview}
                        alt="Icon Preview"
                        className="w-16 h-16 rounded-xl object-cover"
                        onError={() => setIconPreview(null)}
                      />
                    ) : (
                      <span className="text-stone-600 text-xs">Click to upload icon</span>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={() => iconInputRef.current?.click()}
                      disabled={isSaving || generatingKind === 'icon'}
                      className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                    >
                      <Upload className="w-3.5 h-3.5" />
                      <span>Upload</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => handleAIGenerate('icon')}
                      disabled={isSaving || generatingKind === 'icon'}
                      className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
                    >
                      <Sparkles className="w-3.5 h-3.5" />
                      <span>{generatingKind === 'icon' ? 'Gen...' : 'AI Gen'}</span>
                    </button>
                  </div>
                </div>
              </div>
            </div>
```

- [x] **Step 3: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`  
Expected output: Clean exit (0 errors).

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx
git commit -m "feat(frontend): add artwork upload and AI Gen to WorldsStudio"
```

---

### Task 7: Full Verification Suite

**Files:**
- N/A

- [x] **Step 1: Run Go linter and tests**

Run: `go vet ./... && go test -count=1 ./...`  
Expected output: PASS

- [x] **Step 2: Run frontend build and typecheck**

Run: `cd frontend && npx tsc --noEmit && npm run build`  
Expected output: PASS (dist built cleanly)
