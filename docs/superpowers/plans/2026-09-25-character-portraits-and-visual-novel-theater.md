# Character Portrait Generation, Backfill, and Visual Novel Theater Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement automated character metadata enrichment (age, gender, appearance), 3/4 bust portrait generation with procedural SVG fallback, speech segment avatar badges in the Chronicle, and a two-sided Visual Novel Story Theater.

**Architecture:**
- **Entity Model**: Extend `entity.Entity` and `EntityFrontmatter` to formally carry `Gender` and `Age` alongside `Appearance` and `Portrait`.
- **Procedural SVG Bust**: Generate deterministic 3/4 bust SVGs for characters when image generation is disabled, offline, or in-flight.
- **Enrichment & Worker**: A `CharacterEnricher` generates missing character metadata via the LLM; a `PortraitWorker` manages prompt compilation (`3/4 bust portrait, looking slightly to the right...`) and background asset generation.
- **API & GUI**: Serve portraits via `/api/game/{id}/character/{character_id}/portrait` and attach `portrait_url` to `SegmentDTO`.
- **Frontend**: Render speech avatar badges in `TurnSegments.tsx` and a two-sided visual novel dialogue stage in `StoryTheater.tsx` (player on left facing right, NPC on right horizontally flipped).

**Tech Stack:** Go (std library, `modernc.org/sqlite`, `genai`, `yaml.v3`), TypeScript, React 19, Tailwind CSS v4, Lucide icons.

---

## File Map

- `pkg/entity/entity.go`: Add `Gender` and `Age` fields to `EntityFrontmatter` and `Entity`; update `ParseMarkdownEntity` and `SerializeMarkdown`.
- `pkg/entity/entity_test.go`: Test frontmatter serialization and parsing of `gender`, `age`, `appearance`, `portrait`.
- `pkg/media/procedural_bust.go`: Pure-Go procedural SVG generator for 3/4 bust character silhouettes/avatars.
- `pkg/media/procedural_bust_test.go`: Unit tests for procedural bust SVG generation.
- `pkg/engine/portrait_worker.go`: Prompt compilation, queueing, deduplication, and file persistence for portrait assets.
- `pkg/engine/portrait_worker_test.go`: Unit tests for portrait prompt compilation and asset handling.
- `pkg/engine/character_enricher.go`: LLM-based metadata enrichment for missing character fields.
- `pkg/engine/character_enricher_test.go`: Unit tests for backfilling missing character demographics.
- `pkg/gui/service.go`: Implement `GetCharacterPortrait`, wire `CharacterEnricher` and `PortraitWorker`, and populate `PortraitURL` in `segmentDTOs`.
- `pkg/gui/server.go`: Register `/api/game/{id}/character/{character_id}/portrait` endpoint.
- `pkg/gui/character_portrait_test.go`: End-to-end tests for portrait serving and segment DTO resolution.
- `frontend/src/types.ts`: Add `portrait_url` to `TurnSegment`, `EntitySummary`, `EntityNote`.
- `frontend/src/components/TurnSegments.tsx`: Render avatar thumbnail for speech segments with SVG fallback.
- `frontend/src/components/StoryTheater.tsx`: Two-sided visual novel staging with protagonist on left and mirrored NPC on right.

---

### Task 1: Entity Frontmatter Schema & Serialization for Gender and Age

**Files:**
- Modify: `pkg/entity/entity.go:27-65`
- Test: `pkg/entity/entity_test.go`

- [ ] **Step 1: Write the failing test in `pkg/entity/entity_test.go`**

```go
func TestEntityFrontmatterGenderAgeAndPortrait(t *testing.T) {
	raw := `---
id: elena-vance
name: Elena Vance
type: character
gender: female
age: "32"
appearance: A tall pilot with silver hair.
portrait: assets/portraits/elena-vance.png
---
Experienced navigator of the Maw.`

	ent, err := ParseMarkdownEntity([]byte(raw))
	if err != nil {
		t.Fatalf("ParseMarkdownEntity failed: %v", err)
	}

	if ent.Gender != "female" {
		t.Errorf("Gender = %q, want female", ent.Gender)
	}
	if ent.Age != "32" {
		t.Errorf("Age = %q, want 32", ent.Age)
	}
	if ent.Portrait != "assets/portraits/elena-vance.png" {
		t.Errorf("Portrait = %q, want assets/portraits/elena-vance.png", ent.Portrait)
	}
	if ent.Appearance != "A tall pilot with silver hair." {
		t.Errorf("Appearance = %q, want expected appearance", ent.Appearance)
	}

	// Verify serialization round-trips
	data, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}
	reparsed, err := ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("ParseMarkdownEntity roundtrip failed: %v", err)
	}
	if reparsed.Gender != "female" || reparsed.Age != "32" || reparsed.Portrait != "assets/portraits/elena-vance.png" {
		t.Errorf("Roundtrip mismatch: %+v", reparsed)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestEntityFrontmatterGenderAgeAndPortrait ./pkg/entity`
Expected: FAIL (unknown field `Gender`, `Age` on `Entity`)

- [ ] **Step 3: Update `pkg/entity/entity.go`**

Add `Gender` and `Age` to `EntityFrontmatter` and `Entity`:
```go
type EntityFrontmatter struct {
	ID         string                 `yaml:"id"`
	Name       string                 `yaml:"name"`
	Type       string                 `yaml:"type"`
	Tags       []string               `yaml:"tags,omitempty"`
	Voice      *VoiceConfig           `yaml:"voice,omitempty"`
	Portrait   string                 `yaml:"portrait,omitempty"`
	Location   string                 `yaml:"location,omitempty"`
	Appearance string                 `yaml:"appearance,omitempty" json:"appearance,omitempty"`
	Gender     string                 `yaml:"gender,omitempty" json:"gender,omitempty"`
	Age        string                 `yaml:"age,omitempty" json:"age,omitempty"`
	Aliases    []string               `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Faction    string                 `yaml:"faction,omitempty"`
	History    []int                  `yaml:"history,omitempty" json:"history,omitempty"`
	State      map[string]interface{} `yaml:"state,omitempty"`
	ExtraMeta  map[string]interface{} `yaml:",inline"`
}

type Entity struct {
	ID         string
	Name       string
	Type       string
	Tags       []string
	Voice      *VoiceConfig
	Portrait   string
	Location   string
	Faction    string
	Appearance string
	Gender     string
	Age        string
	Aliases    []string
	History    []int
	State      *state.State
	ExtraMeta  map[string]interface{}
	Body       string
	Wikilinks  []string
	Hash       string
}
```
Update `ParseMarkdownEntity` and `SerializeMarkdown` to map `Gender` and `Age`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestEntityFrontmatterGenderAgeAndPortrait ./pkg/entity`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/entity/entity.go pkg/entity/entity_test.go
git commit -m "feat(entity): add gender and age frontmatter fields with roundtrip support"
```

---

### Task 2: Procedural SVG Bust Generator

**Files:**
- Create: `pkg/media/procedural_bust.go`
- Create: `pkg/media/procedural_bust_test.go`

- [ ] **Step 1: Write the failing test in `pkg/media/procedural_bust_test.go`**

```go
package media

import (
	"strings"
	"testing"
)

func TestGenerateProceduralBustSVG(t *testing.T) {
	svgBytes := GenerateProceduralBustSVG("stretch-layabout", "Stretch Layabout", "non-binary")
	if len(svgBytes) == 0 {
		t.Fatal("expected non-empty SVG bytes")
	}

	svgStr := string(svgBytes)
	if !strings.HasPrefix(svgStr, "<svg") || !strings.HasSuffix(strings.TrimSpace(svgStr), "</svg>") {
		t.Errorf("expected valid SVG root tag, got %s", svgStr)
	}
	if !strings.Contains(svgStr, "viewBox=\"0 0 256 256\"") {
		t.Errorf("expected 256x256 viewBox, got %s", svgStr)
	}

	// Determinism test
	repeatBytes := GenerateProceduralBustSVG("stretch-layabout", "Stretch Layabout", "non-binary")
	if string(svgBytes) != string(repeatBytes) {
		t.Errorf("expected deterministic SVG generation for the same character ID")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestGenerateProceduralBustSVG ./pkg/media`
Expected: FAIL (`GenerateProceduralBustSVG` undefined)

- [ ] **Step 3: Implement `pkg/media/procedural_bust.go`**

Generate a stylish 3/4 bust silhouette looking slightly to the right with deterministic hue/palette derived from FNV hash of the character ID:
```go
package media

import (
	"fmt"
	"hash/fnv"
)

// GenerateProceduralBustSVG generates a deterministic 3/4 bust silhouette looking right.
func GenerateProceduralBustSVG(id, name, gender string) []byte {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id + ":" + name))
	seed := h.Sum32()

	hue1 := seed % 360
	hue2 := (hue1 + 40) % 360

	// 3/4 bust looking slightly right: head slightly offset, angled jaw, angled shoulders
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="100%%" height="100%%">
  <defs>
    <linearGradient id="bgGrad" x1="0%%" y1="0%%" x2="100%%" y2="100%%">
      <stop offset="0%%" stop-color="hsl(%d, 35%%, 18%%)" />
      <stop offset="100%%" stop-color="hsl(%d, 40%%, 10%%)" />
    </linearGradient>
    <linearGradient id="bustGrad" x1="0%%" y1="0%%" x2="100%%" y2="100%%">
      <stop offset="0%%" stop-color="hsl(%d, 50%%, 75%%)" />
      <stop offset="100%%" stop-color="hsl(%d, 55%%, 45%%)" />
    </linearGradient>
  </defs>
  <rect width="256" height="256" rx="32" fill="url(#bgGrad)" />
  <!-- Shoulders / Torso angled 3/4 to the right -->
  <path d="M 40 256 C 45 200, 75 170, 115 160 C 130 156, 155 156, 175 165 C 215 180, 235 210, 240 256 Z" fill="url(#bustGrad)" opacity="0.9" />
  <!-- Neck -->
  <path d="M 120 162 L 126 125 L 158 128 L 160 165 Z" fill="url(#bustGrad)" opacity="0.95" />
  <!-- Head 3/4 turned to the right -->
  <ellipse cx="146" cy="95" rx="44" ry="54" fill="url(#bustGrad)" />
  <!-- Jawline contour emphasizing looking right -->
  <path d="M 125 105 Q 145 150 178 125 Q 192 100 188 78 Z" fill="url(#bustGrad)" />
</svg>`, hue1, hue2, hue1, hue2)

	return []byte(svg)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestGenerateProceduralBustSVG ./pkg/media`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/procedural_bust.go pkg/media/procedural_bust_test.go
git commit -m "feat(media): implement procedural 3/4 bust SVG character generator"
```

---

### Task 3: Portrait Prompt Builder & Portrait Worker

**Files:**
- Create: `pkg/engine/portrait_worker.go`
- Create: `pkg/engine/portrait_worker_test.go`

- [ ] **Step 1: Write the failing test in `pkg/engine/portrait_worker_test.go`**

```go
package engine

import (
	"strings"
	"testing"
)

func TestBuildPortraitPrompt(t *testing.T) {
	prompt := BuildPortraitPrompt("Elena Vance", "female", "32", "Athletic pilot with silver hair and blast goggles.", "gritty retro sci-fi watercolor")

	if !strings.HasPrefix(prompt, "3/4 bust portrait, looking slightly to the right") {
		t.Errorf("prompt must start with 3/4 bust facing right framing, got: %s", prompt)
	}
	if !strings.Contains(prompt, "gritty retro sci-fi watercolor") {
		t.Errorf("prompt must contain world art style, got: %s", prompt)
	}
	if !strings.Contains(prompt, "female") || !strings.Contains(prompt, "32 years old") {
		t.Errorf("prompt must contain gender and age, got: %s", prompt)
	}
	if !strings.Contains(prompt, "silver hair and blast goggles") {
		t.Errorf("prompt must contain appearance details, got: %s", prompt)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestBuildPortraitPrompt ./pkg/engine`
Expected: FAIL (`BuildPortraitPrompt` undefined)

- [ ] **Step 3: Implement `pkg/engine/portrait_worker.go`**

```go
package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// BuildPortraitPrompt formats the 3/4 bust prompt with world art style and character traits.
func BuildPortraitPrompt(name, gender, age, appearance, artStyle string) string {
	if artStyle == "" {
		artStyle = "digital illustration, character concept art"
	}
	var descParts []string
	if gender != "" {
		descParts = append(descParts, gender)
	}
	if age != "" {
		descParts = append(descParts, age+" years old")
	}
	if appearance != "" {
		descParts = append(descParts, strings.TrimSpace(appearance))
	}
	desc := strings.Join(descParts, ", ")
	if desc == "" {
		desc = "detailed character features"
	}

	return fmt.Sprintf("3/4 bust portrait, looking slightly to the right, head and upper torso centered, neutral plain studio backdrop, %s, %s, clean composition, high quality character portrait, no text, no borders", artStyle, desc)
}

// PortraitGenerator abstracts image generation for the worker.
type PortraitGenerator interface {
	GenerateImage(ctx context.Context, kind, prompt string) ([]byte, error)
}

// PortraitWorker manages queued portrait generation with in-memory deduplication.
type PortraitWorker struct {
	mu        sync.Mutex
	resolver  *core.PathResolver
	store     *storage.Store
	generator PortraitGenerator
	inFlight  map[string]bool
}

func NewPortraitWorker(resolver *core.PathResolver, store *storage.Store, gen PortraitGenerator) *PortraitWorker {
	return &PortraitWorker{
		resolver:  resolver,
		store:     store,
		generator: gen,
		inFlight:  make(map[string]bool),
	}
}

func (w *PortraitWorker) Enqueue(gameID string, ent *entity.Entity, artStyle string) {
	if w.generator == nil || ent == nil || ent.ID == "" || ent.Type != "character" {
		return
	}
	if ent.Portrait != "" {
		return
	}

	key := gameID + ":" + ent.ID
	w.mu.Lock()
	if w.inFlight[key] {
		w.mu.Unlock()
		return
	}
	w.inFlight[key] = true
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.inFlight, key)
			w.mu.Unlock()
		}()

		prompt := BuildPortraitPrompt(ent.Name, ent.Gender, ent.Age, ent.Appearance, artStyle)
		imgBytes, err := w.generator.GenerateImage(context.Background(), "portrait", prompt)
		if err != nil || len(imgBytes) == 0 {
			return
		}

		ext := media.ArtExtension(imgBytes)
		if ext == "" {
			ext = ".png"
		}
		relPath := filepath.Join("assets", "portraits", ent.ID+ext)
		fullPath := filepath.Join(w.resolver.GameDir(gameID), relPath)

		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return
		}
		if err := os.WriteFile(fullPath, imgBytes, 0644); err != nil {
			return
		}

		ent.Portrait = relPath
		notePath := filepath.Join(w.resolver.GameDir(gameID), "entities", ent.ID+".md")
		if data, err := ent.SerializeMarkdown(); err == nil {
			_ = os.WriteFile(notePath, data, 0644)
			_ = storage.NewSyncer(w.store).SyncFile(notePath)
		}
	}()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestBuildPortraitPrompt ./pkg/engine`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/portrait_worker.go pkg/engine/portrait_worker_test.go
git commit -m "feat(engine): add portrait prompt builder and background portrait worker"
```

---

### Task 4: Character Metadata Enricher

**Files:**
- Create: `pkg/engine/character_enricher.go`
- Create: `pkg/engine/character_enricher_test.go`

- [ ] **Step 1: Write the failing test in `pkg/engine/character_enricher_test.go`**

```go
package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

type stubRouter struct {
	response string
}

func (s *stubRouter) GenerateForRole(ctx context.Context, role string, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: s.response}, nil
}

func TestCharacterEnricher_EnrichesMissingFields(t *testing.T) {
	router := &stubRouter{
		response: `{"gender":"female","age":"29","pronouns":"she/her","appearance":"A sharp-eyed scout with braided auburn hair and leather flight gear."}`,
	}
	enricher := NewCharacterEnricher(router)

	ent := &entity.Entity{
		ID:   "mara-jade",
		Name: "Mara Jade",
		Type: "character",
		Body: "A smuggler spotted in the lower cantina.",
	}

	enriched, err := enricher.Enrich(context.Background(), ent, "Space Opera Sci-Fi")
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if enriched.Gender != "female" {
		t.Errorf("Gender = %q, want female", enriched.Gender)
	}
	if enriched.Age != "29" {
		t.Errorf("Age = %q, want 29", enriched.Age)
	}
	if !strings.Contains(enriched.Appearance, "sharp-eyed scout") {
		t.Errorf("Appearance = %q, want generated appearance", enriched.Appearance)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestCharacterEnricher_EnrichesMissingFields ./pkg/engine`
Expected: FAIL (`CharacterEnricher` undefined)

- [ ] **Step 3: Implement `pkg/engine/character_enricher.go`**

```go
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

type RoleGenerator interface {
	GenerateForRole(ctx context.Context, role string, req harness.GenerateRequest) (*harness.GenerateResponse, error)
}

type CharacterEnricher struct {
	router RoleGenerator
}

func NewCharacterEnricher(router RoleGenerator) *CharacterEnricher {
	return &CharacterEnricher{router: router}
}

type characterEnrichmentResult struct {
	Gender     string `json:"gender"`
	Age        string `json:"age"`
	Pronouns   string `json:"pronouns"`
	Appearance string `json:"appearance"`
}

func (e *CharacterEnricher) NeedsEnrichment(ent *entity.Entity) bool {
	if ent == nil || ent.Type != "character" {
		return false
	}
	return strings.TrimSpace(ent.Appearance) == "" || strings.TrimSpace(ent.Gender) == "" || strings.TrimSpace(ent.Age) == ""
}

func (e *CharacterEnricher) Enrich(ctx context.Context, ent *entity.Entity, worldGenre string) (*entity.Entity, error) {
	if !e.NeedsEnrichment(ent) {
		return ent, nil
	}

	prompt := fmt.Sprintf(`Generate the physical appearance and demographic attributes for this RPG character based on their background and setting.
Character Name: %s
World Genre: %s
Current Notes: %s

Respond ONLY with a valid JSON object matching this schema:
{
  "gender": "demographic gender",
  "age": "approximate age in years or developmental stage",
  "pronouns": "preferred pronouns",
  "appearance": "2-3 concise sentences describing their face, build, distinguishing marks, clothing, and posture"
}`, ent.Name, worldGenre, ent.Body)

	resp, err := e.router.GenerateForRole(ctx, "extractor", harness.GenerateRequest{
		System: "You are a concise character designer for a tabletop RPG. Output only valid JSON without markdown fences.",
		Prompt: prompt,
	})
	if err != nil {
		// Fallback to GM role if extractor fails or is unconfigured
		resp, err = e.router.GenerateForRole(ctx, "gm", harness.GenerateRequest{
			System: "You are a concise character designer for a tabletop RPG. Output only valid JSON without markdown fences.",
			Prompt: prompt,
		})
		if err != nil {
			return ent, fmt.Errorf("enrich character model call: %w", err)
		}
	}

	cleanJSON := strings.TrimSpace(resp.Text)
	if strings.HasPrefix(cleanJSON, "```json") {
		cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
		cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	} else if strings.HasPrefix(cleanJSON, "```") {
		cleanJSON = strings.TrimPrefix(cleanJSON, "```")
		cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	}
	cleanJSON = strings.TrimSpace(cleanJSON)

	var res characterEnrichmentResult
	if err := json.Unmarshal([]byte(cleanJSON), &res); err != nil {
		return ent, fmt.Errorf("parse character enrichment JSON: %w (raw: %s)", err, cleanJSON)
	}

	if ent.Gender == "" {
		ent.Gender = res.Gender
	}
	if ent.Age == "" {
		ent.Age = res.Age
	}
	if ent.Appearance == "" {
		ent.Appearance = res.Appearance
	}
	if res.Pronouns != "" && ent.ExtraMeta != nil {
		if _, ok := ent.ExtraMeta["pronouns"]; !ok {
			ent.ExtraMeta["pronouns"] = res.Pronouns
		}
	}

	return ent, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestCharacterEnricher_EnrichesMissingFields ./pkg/engine`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/character_enricher.go pkg/engine/character_enricher_test.go
git commit -m "feat(engine): implement character metadata enricher for missing attributes"
```

---

### Task 5: GUI Service Integration & Portrait Serving Endpoint

**Files:**
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/server.go`
- Create: `pkg/gui/character_portrait_test.go`

- [ ] **Step 1: Write the failing test in `pkg/gui/character_portrait_test.go`**

```go
package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetCharacterPortraitEndpoint_ProceduralFallback(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(tmpDir)

	gameID, err := svc.CreateGame(context.Background(), CreateGameRequestDTO{
		Name:       "Test Portrait Game",
		SystemID:   "freeform",
		WorldID:    "default",
		PlayerName: "Hero Vance",
	})
	if err != nil {
		t.Fatalf("CreateGame failed: %v", err)
	}

	server := NewServer(svc)
	req := httptest.NewRequest("GET", "/api/game/"+gameID.ID+"/character/hero-vance/portrait", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "image/svg+xml") {
		t.Errorf("expected image/svg+xml fallback, got %s", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "<svg") {
		t.Errorf("expected SVG body, got %s", w.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestGetCharacterPortraitEndpoint_ProceduralFallback ./pkg/gui`
Expected: FAIL (404 Not Found)

- [ ] **Step 3: Update `pkg/gui/service.go` and `pkg/gui/server.go`**

1. In `pkg/gui/service.go`, add `GetCharacterPortrait`:
```go
func (s *Service) GetCharacterPortrait(ctx context.Context, gameID, characterID string) ([]byte, string, error) {
	store, err := s.store(gameID)
	if err != nil {
		return nil, "", err
	}
	ent, err := store.GetEntity(characterID)
	if err != nil || ent == nil {
		return nil, "", fmt.Errorf("character %q not found", characterID)
	}

	if ent.Portrait != "" {
		portraitPath := filepath.Join(s.resolver.GameDir(gameID), ent.Portrait)
		if data, err := os.ReadFile(portraitPath); err == nil && len(data) > 0 {
			return data, imageContentType(data), nil
		}
	}

	// Procedural SVG fallback
	svg := media.GenerateProceduralBustSVG(ent.ID, ent.Name, ent.Gender)
	return svg, "image/svg+xml", nil
}
```

2. In `pkg/gui/service.go:segmentDTOs`, add `PortraitURL`:
```go
		if segment.Kind == "speech" {
			refID := segment.SpeakerID
			if refID == "" && resolve != nil {
				refID = resolve(segment.Speaker)
			}
			if refID != "" {
				dto.PortraitURL = fmt.Sprintf("/api/game/%s/character/%s/portrait", gameID, refID)
			}
		}
```

3. In `pkg/gui/server.go`, register route:
```go
	r.GET("/api/game/{id}/character/{character_id}/portrait", s.handleGetCharacterPortrait)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestGetCharacterPortraitEndpoint_ProceduralFallback ./pkg/gui`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/server.go pkg/gui/character_portrait_test.go
git commit -m "feat(gui): add character portrait endpoint and segment portrait resolution"
```

---

### Task 6: Frontend Speech Segment Portrait Avatars

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/components/TurnSegments.tsx`

- [ ] **Step 1: Update `frontend/src/types.ts`**

Add `portrait_url` to `TurnSegment`:
```typescript
export interface TurnSegment {
  kind: 'narration' | 'speech';
  speaker?: string;
  speaker_id?: string;
  text: string;
  audio_url?: string;
  portrait_url?: string;
  audio_key?: string;
  player?: boolean;
  duration?: number;
}
```

- [ ] **Step 2: Update `frontend/src/components/TurnSegments.tsx`**

Render the speaker avatar thumbnail adjacent to the speaker header:
```tsx
            <div className="flex items-center gap-3">
              {segment.portrait_url && (
                <div
                  onClick={() => segment.speaker_id && onEntityClick?.(segment.speaker_id)}
                  className={`w-9 h-9 rounded-full overflow-hidden shrink-0 border-2 shadow-md cursor-pointer transition-transform hover:scale-105 ${
                    segment.player ? 'border-sky-400/80' : 'border-purple-400/80'
                  }`}
                  title={segment.speaker || 'Character'}
                >
                  <img
                    src={segment.portrait_url}
                    alt={segment.speaker || 'Speaker portrait'}
                    className="w-full h-full object-cover"
                    loading="lazy"
                  />
                </div>
              )}
              {hasAudio ? (
                <button
                  onClick={() => (serverPlayback ? startServerPlayback(i) : playFrom(i))}
                  className={`text-xs font-sans font-bold tracking-widest hover:opacity-80 cursor-pointer ${
                    segment.player ? 'text-sky-300' : 'text-purple-400'
                  }`}
                >
                  {segment.speaker || 'UNKNOWN'}
                  {segment.player && <span className="ml-2 text-stone-400 normal-case">(you)</span>}
                </button>
              ) : (
                <div
                  className={`text-xs font-sans font-bold tracking-widest ${
                    segment.player ? 'text-sky-300' : 'text-purple-400'
                  }`}
                >
                  {segment.speaker || 'UNKNOWN'}
                  {segment.player && <span className="ml-2 text-stone-400 normal-case">(you)</span>}
                </div>
              )}
            </div>
```

- [ ] **Step 3: Run frontend build to verify compilation**

Run: `npm --prefix frontend run build`
Expected: PASS with 0 type errors

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/TurnSegments.tsx
git commit -m "feat(frontend): render character portrait avatar badges in speech segments"
```

---

### Task 7: Visual Novel Story Theater Staging

**Files:**
- Modify: `frontend/src/components/StoryTheater.tsx`

- [ ] **Step 1: Implement Two-Sided Stage in `frontend/src/components/StoryTheater.tsx`**

Update `StoryTheater.tsx` to:
1. Identify the protagonist and active speaker of the current segment.
2. Render protagonist portrait on stage left (unflipped, looking right).
3. Render active NPC portrait on stage right (flipped horizontally with `scale-x-[-1]`, looking left towards player).
4. Apply dynamic active speaker focus lighting (`opacity-100 scale-105` vs `opacity-50 brightness-75 scale-95`).
5. Render bottom translucent VN dialogue card.

```tsx
        {/* Visual Novel Character Stage */}
        <div className="relative w-full max-w-5xl h-[420px] flex items-end justify-between px-12 pointer-events-none">
          {/* Protagonist (Stage Left, facing Right) */}
          {playerPortrait && (
            <div
              className={`relative w-64 h-80 transition-all duration-500 transform origin-bottom ${
                isPlayerSpeaking ? 'opacity-100 scale-105 drop-shadow-[0_10px_25px_rgba(56,189,248,0.3)] z-20' : 'opacity-40 brightness-75 scale-95 z-10'
              }`}
            >
              <img
                src={playerPortrait}
                alt="Protagonist"
                className="w-full h-full object-contain"
              />
            </div>
          )}

          {/* NPC Interlocutor (Stage Right, flipped facing Left) */}
          {npcPortrait && (
            <div
              className={`relative w-64 h-80 transition-all duration-500 transform origin-bottom scale-x-[-1] ${
                isNpcSpeaking ? 'opacity-100 scale-105 drop-shadow-[0_10px_25px_rgba(168,85,247,0.3)] z-20' : 'opacity-40 brightness-75 scale-95 z-10'
              }`}
            >
              <img
                src={npcPortrait}
                alt="Interlocutor"
                className="w-full h-full object-contain"
              />
            </div>
          )}
        </div>
```

- [ ] **Step 2: Run frontend build to verify compilation**

Run: `npm --prefix frontend run build`
Expected: PASS with 0 type errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/StoryTheater.tsx
git commit -m "feat(frontend): implement visual novel two-sided stage in Story Theater"
```

---

### Task 8: Full Verification & Integration Test

**Files:**
- Test all backend and frontend suites

- [ ] **Step 1: Run all backend tests and vet**

Run: `go test -v -count=1 ./... && go vet ./...`
Expected: PASS with 0 failures and clean vet

- [ ] **Step 2: Run full build**

Run: `npm --prefix frontend run build && go build ./cmd/localrpg`
Expected: PASS

- [ ] **Step 3: Final Commit and Push**

```bash
git push origin main
```
