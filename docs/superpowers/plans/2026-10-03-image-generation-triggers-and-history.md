# Image Generation Triggers and History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement scene break detection with asynchronous turn scene illustration generation, and character appearance evolution with versioned portraits and turn-anchored historical portraits.

**Architecture:** Hybrid detection identifies scene breaks via markdown `---` rules or extractor cues, triggering an asynchronous `SceneWorker` that writes `assets/scenes/turn-<N>.<ext>` and broadcasts `scene_image` events. For character evolution, the extractor flags `appearance_changed` and captures updated `age` and `appearance`, prompting `PortraitWorker` to generate versioned files (`assets/portraits/<id>-v<N>.<ext>`) without deleting previous versions. `history.jsonl` turns permanently record the active portrait version on spoken segments (`TurnSegment.SpeakerPortrait`), allowing historical turns in the frontend to display the era-appropriate portrait.

**Tech Stack:** Go 1.27.1, React 19, TypeScript 5, Tailwind CSS v4, SQLite (modernc.org/sqlite).

---

## File Map

### New Files
- `pkg/engine/scene_worker.go`: Manages asynchronous generation of turn scene illustrations (`assets/scenes/turn-<N>.<ext>`).
- `pkg/engine/scene_worker_test.go`: Unit tests for `SceneWorker` generation, deduplication, and file writing.

### Modified Files
- `pkg/entity/entity.go`: Add `PortraitVersion` and `PortraitHistory` fields to `Entity` frontmatter.
- `pkg/entity/segment.go`: Add `SpeakerPortrait` field to `TurnSegment`.
- `pkg/engine/history.go`: Add `SceneBreak` field to `Turn`.
- `pkg/harness/extractor.go`: Add `ExtractedSceneBreak` to `Extraction`, add `Age` and `AppearanceChanged` to `ExtractedEntity`, update system prompt and `MergeExtractedEntity`.
- `pkg/harness/extractor_test.go`: Unit tests for scene break and appearance change extraction and merge logic.
- `pkg/engine/portrait_worker.go`: Implement versioned file creation (`<id>-v<version>.<ext>`), retain historical files, and update frontmatter versioning.
- `pkg/engine/portrait_worker_test.go`: Unit tests for versioned portrait generation and history retention.
- `pkg/engine/orchestrator.go`: Check for scene breaks and appearance changes during turn processing, invoking `SceneWorker` and anchoring speaker portraits on turn segments.
- `pkg/gui/types.go`: Add `SceneBreak` to `TurnDTO`, `SpeakerPortrait` to `SegmentDTO`, and `TurnNumber`/`Version` to `TurnEvent`.
- `pkg/gui/service.go`: Wire `SceneWorker`, add `GetTurnSceneImage`, support `?v=` in `GetCharacterPortrait`, populate `TurnDTO.ImageURL` and `SegmentDTO.SpeakerPortrait`, broadcast `scene_image` event.
- `pkg/gui/server.go`: Add route `/api/game/{gameID}/turn/{turnNumber}/scene-image` and pass `?v=` to `GetCharacterPortrait`.
- `pkg/gui/service_test.go`: Unit tests for `GetTurnSceneImage`, versioned `GetCharacterPortrait`, and turn DTO population.
- `frontend/src/types.ts`: Update `TurnDTO`, `TurnSegment`, and `TurnEvent` interfaces.
- `frontend/src/components/TurnSegments.tsx`: Use segment-anchored portrait URL before falling back to global active portrait.
- `frontend/src/App.tsx`: Handle `scene_image` events in `chronicle` state and handle `portrait` events with versioning.
- `frontend/src/components/ChronicleView.tsx`: Render scene divider on `turn.scene_break` and display `turn.image_url` scene illustration.

---

## Tasks

### Task 1: Data Model & Entity Extensions

**Files:**
- Modify: `pkg/entity/entity.go`
- Modify: `pkg/entity/segment.go`
- Modify: `pkg/engine/history.go`
- Test: `pkg/entity/entity_test.go`
- Test: `pkg/entity/history_test.go`

- [ ] **Step 1: Write failing test for Entity and TurnSegment fields**

In `pkg/entity/entity_test.go`:
```go
func TestEntityPortraitVersioningFields(t *testing.T) {
	ent := &Entity{
		ID:              "vera",
		Name:            "Vera",
		Type:            "character",
		Portrait:        "assets/portraits/vera-v2.png",
		PortraitVersion: 2,
		PortraitHistory: []string{"assets/portraits/vera-v1.png"},
	}

	data, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown failed: %v", err)
	}

	reparsed, err := ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("ParseMarkdownEntity failed: %v", err)
	}

	if reparsed.PortraitVersion != 2 {
		t.Errorf("PortraitVersion = %d, want 2", reparsed.PortraitVersion)
	}
	if len(reparsed.PortraitHistory) != 1 || reparsed.PortraitHistory[0] != "assets/portraits/vera-v1.png" {
		t.Errorf("PortraitHistory = %v, want [assets/portraits/vera-v1.png]", reparsed.PortraitHistory)
	}
}
```

In `pkg/entity/history_test.go`:
```go
func TestTurnSegmentSpeakerPortraitRoundTrip(t *testing.T) {
	seg := TurnSegment{
		Kind:            SegmentSpeech,
		Speaker:         "Vera",
		SpeakerID:       "vera",
		SpeakerPortrait: "/api/game/g1/character/vera/portrait?v=2",
		Text:            "It has been a decade.",
	}

	data, err := json.Marshal(seg)
	if err != nil {
		t.Fatalf("marshal TurnSegment failed: %v", err)
	}

	var unmarshaled TurnSegment
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("unmarshal TurnSegment failed: %v", err)
	}

	if unmarshaled.SpeakerPortrait != seg.SpeakerPortrait {
		t.Errorf("SpeakerPortrait = %q, want %q", unmarshaled.SpeakerPortrait, seg.SpeakerPortrait)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -count=1 ./pkg/entity/ -run "TestEntityPortraitVersioningFields|TestTurnSegmentSpeakerPortraitRoundTrip"`
Expected: FAIL due to unknown fields `PortraitVersion`, `PortraitHistory`, and `SpeakerPortrait`.

- [ ] **Step 3: Implement data model changes**

In `pkg/entity/entity.go`:
Add fields to `Entity` struct:
```go
type Entity struct {
	// ... existing fields ...
	PortraitVersion int      `yaml:"portrait_version,omitempty" json:"portrait_version,omitempty"`
	PortraitHistory []string `yaml:"portrait_history,omitempty" json:"portrait_history,omitempty"`
}
```
And update `entityFrontmatter` struct and `ParseMarkdownEntity` / `SerializeMarkdown` mappings.

In `pkg/entity/segment.go`:
Add `SpeakerPortrait` to `TurnSegment`:
```go
type TurnSegment struct {
	Kind            string `json:"kind"`
	Speaker         string `json:"speaker,omitempty"`
	SpeakerID       string `json:"speaker_id,omitempty"`
	SpeakerPortrait string `json:"speaker_portrait,omitempty"`
	Text            string `json:"text"`
	CheckRef        string `json:"check_ref,omitempty"`
	Player          bool   `json:"player,omitempty"`
}
```

In `pkg/engine/history.go`:
Add `SceneBreak` to `Turn`:
```go
type Turn struct {
	// ... existing fields ...
	SceneBreak bool `json:"scene_break,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -count=1 ./pkg/entity/...`
Expected: PASS

- [ ] **Step 5: Commit changes**

```bash
git add pkg/entity/entity.go pkg/entity/segment.go pkg/engine/history.go pkg/entity/entity_test.go pkg/entity/history_test.go
git commit -m "feat(entity): add portrait versioning and speaker portrait fields"
```

---

### Task 2: Extractor Enhancements for Scene Breaks & Character Evolution

**Files:**
- Modify: `pkg/harness/extractor.go`
- Test: `pkg/harness/extractor_test.go`

- [ ] **Step 1: Write failing tests for scene break and appearance change extraction**

In `pkg/harness/extractor_test.go`:
```go
func TestExtractorParsesSceneBreakAndAppearanceChange(t *testing.T) {
	jsonPayload := `{
		"entities": [
			{
				"id": "vera",
				"name": "Vera",
				"type": "character",
				"appearance": "Grey-streaked hair and a hardened gaze.",
				"age": "38",
				"appearance_changed": true,
				"body": "Ten years of wandering have changed her."
			}
		],
		"scene_break": {
			"occurred": true,
			"visual_cue": "Ten years later, the dilapidated courtyard overgrown with ivy under grey skies."
		}
	}`

	var extraction Extraction
	if err := json.Unmarshal([]byte(jsonPayload), &extraction); err != nil {
		t.Fatalf("unmarshal Extraction failed: %v", err)
	}

	if extraction.SceneBreak == nil || !extraction.SceneBreak.Occurred {
		t.Fatalf("expected SceneBreak.Occurred to be true")
	}
	if extraction.SceneBreak.VisualCue != "Ten years later, the dilapidated courtyard overgrown with ivy under grey skies." {
		t.Errorf("unexpected visual cue: %q", extraction.SceneBreak.VisualCue)
	}

	if len(extraction.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(extraction.Entities))
	}
	ent := extraction.Entities[0]
	if !ent.AppearanceChanged {
		t.Errorf("expected AppearanceChanged to be true")
	}
	if ent.Age != "38" {
		t.Errorf("expected Age 38, got %q", ent.Age)
	}
}

func TestMergeExtractedEntityUpdatesAppearanceWhenChanged(t *testing.T) {
	existing := &entity.Entity{
		ID:         "vera",
		Name:       "Vera",
		Type:       "character",
		Appearance: "Youthful scout with bright hazel eyes.",
		Age:        "28",
	}

	// 1. Regular mention without AppearanceChanged preserves authored appearance
	regularMention := &ExtractedEntity{
		ID:         "vera",
		Name:       "Vera",
		Type:       "character",
		Appearance: "Looking weary.",
	}
	merged1 := MergeExtractedEntity(existing, regularMention)
	if merged1.Appearance != "Youthful scout with bright hazel eyes." {
		t.Errorf("expected authored appearance kept, got %q", merged1.Appearance)
	}

	// 2. Evolution with AppearanceChanged updates appearance and age
	evolution := &ExtractedEntity{
		ID:                "vera",
		Name:              "Vera",
		Type:              "character",
		Appearance:        "Grey-streaked hair and a hardened gaze.",
		Age:               "38",
		AppearanceChanged: true,
	}
	merged2 := MergeExtractedEntity(existing, evolution)
	if merged2.Appearance != "Grey-streaked hair and a hardened gaze." {
		t.Errorf("expected updated appearance, got %q", merged2.Appearance)
	}
	if merged2.Age != "38" {
		t.Errorf("expected updated age, got %q", merged2.Age)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -count=1 ./pkg/harness/ -run "TestExtractorParsesSceneBreakAndAppearanceChange|TestMergeExtractedEntityUpdatesAppearanceWhenChanged"`
Expected: FAIL due to missing fields and old `MergeExtractedEntity` logic.

- [ ] **Step 3: Implement extractor enhancements**

In `pkg/harness/extractor.go`:
1. Define `ExtractedSceneBreak`:
```go
type ExtractedSceneBreak struct {
	Occurred  bool   `json:"occurred"`
	VisualCue string `json:"visual_cue,omitempty"`
}
```
2. Add `SceneBreak *ExtractedSceneBreak` to `Extraction`.
3. Add `Age string` and `AppearanceChanged bool` to `ExtractedEntity`.
4. Update `extractorSystemPrompt` to instruct the model on detecting temporal jumps / scene breaks and character physical alterations.
5. In `MergeExtractedEntity`:
```go
func MergeExtractedEntity(existing *entity.Entity, raw *ExtractedEntity) *entity.Entity {
	merged := *existing

	if merged.Name == "" {
		merged.Name = raw.Name
	}
	if merged.Type == "" {
		merged.Type = raw.Type
	}
	if merged.Location == "" {
		merged.Location = raw.Location
	}
	if merged.Faction == "" {
		merged.Faction = raw.Faction
	}

	if raw.AppearanceChanged || merged.Appearance == "" {
		if strings.TrimSpace(raw.Appearance) != "" {
			merged.Appearance = raw.Appearance
		}
	}
	if strings.TrimSpace(raw.Age) != "" {
		merged.Age = raw.Age
	}

	body := strings.TrimSpace(raw.Body)
	if body != "" && !strings.Contains(merged.Body, body) {
		if strings.TrimSpace(merged.Body) == "" {
			merged.Body = body
		} else {
			merged.Body = strings.TrimSpace(merged.Body) + "\n\n" + body
		}
	}

	return &merged
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -count=1 ./pkg/harness/...`
Expected: PASS

- [ ] **Step 5: Commit changes**

```bash
git add pkg/harness/extractor.go pkg/harness/extractor_test.go
git commit -m "feat(harness): add scene break and character evolution extraction"
```

---

### Task 3: Versioned Portrait Worker & Asset Immutability

**Files:**
- Modify: `pkg/engine/portrait_worker.go`
- Test: `pkg/engine/portrait_worker_test.go`

- [ ] **Step 1: Write failing tests for versioned portrait generation**

In `pkg/engine/portrait_worker_test.go`:
```go
func TestPortraitWorker_VersionedPortraitsAndHistoryRetention(t *testing.T) {
	tempDir := t.TempDir()
	resolver := core.NewPathResolver(filepath.Join(tempDir, "config"), filepath.Join(tempDir, "data"), filepath.Join(tempDir, "cache"))
	gameID := "test-game"
	gameDir := resolver.GameDir(gameID)
	_ = os.MkdirAll(filepath.Join(gameDir, "entities"), 0755)

	gen := &mockPortraitGen{data: []byte("<svg>portrait v1</svg>")}
	worker := NewPortraitWorker(resolver, nil, gen)

	vera := &entity.Entity{
		ID:         "vera",
		Name:       "Vera",
		Type:       "character",
		Appearance: "Young scout",
	}

	// 1. Initial generation creates v1
	path1, err := worker.Regenerate(context.Background(), gameID, vera, "fantasy art")
	if err != nil {
		t.Fatalf("first Regenerate failed: %v", err)
	}
	if path1 != filepath.Join("assets", "portraits", "vera-v1.svg") {
		t.Errorf("expected path assets/portraits/vera-v1.svg, got %s", path1)
	}

	// Verify v1 file exists
	fullPath1 := filepath.Join(gameDir, path1)
	if _, err := os.Stat(fullPath1); err != nil {
		t.Fatalf("v1 file does not exist: %v", err)
	}

	// Verify note frontmatter
	notePath := filepath.Join(gameDir, "entities", "vera.md")
	noteBytes, _ := os.ReadFile(notePath)
	parsedVera, _ := entity.ParseMarkdownEntity(noteBytes)
	if parsedVera.PortraitVersion != 1 {
		t.Errorf("expected PortraitVersion 1, got %d", parsedVera.PortraitVersion)
	}

	// 2. Second generation (evolution) creates v2 and preserves v1
	gen.data = []byte("<svg>portrait v2</svg>")
	path2, err := worker.Regenerate(context.Background(), gameID, parsedVera, "fantasy art")
	if err != nil {
		t.Fatalf("second Regenerate failed: %v", err)
	}
	if path2 != filepath.Join("assets", "portraits", "vera-v2.svg") {
		t.Errorf("expected path assets/portraits/vera-v2.svg, got %s", path2)
	}

	// Both v1 and v2 files must exist on disk!
	if _, err := os.Stat(fullPath1); err != nil {
		t.Errorf("v1 file was deleted! Must be retained: %v", err)
	}
	fullPath2 := filepath.Join(gameDir, path2)
	if _, err := os.Stat(fullPath2); err != nil {
		t.Fatalf("v2 file does not exist: %v", err)
	}

	// Verify note frontmatter updated
	noteBytes2, _ := os.ReadFile(notePath)
	parsedVera2, _ := entity.ParseMarkdownEntity(noteBytes2)
	if parsedVera2.PortraitVersion != 2 {
		t.Errorf("expected PortraitVersion 2, got %d", parsedVera2.PortraitVersion)
	}
	if len(parsedVera2.PortraitHistory) != 1 || parsedVera2.PortraitHistory[0] != path1 {
		t.Errorf("expected PortraitHistory [%s], got %v", path1, parsedVera2.PortraitHistory)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -count=1 ./pkg/engine/ -run "TestPortraitWorker_VersionedPortraitsAndHistoryRetention"`
Expected: FAIL due to unversioned file naming `<id>.<ext>`.

- [ ] **Step 3: Update `PortraitWorker` to generate versioned portraits**

In `pkg/engine/portrait_worker.go`:
1. In `writePortrait`:
   - Compute version:
     ```go
     version := ent.PortraitVersion + 1
     if version <= 1 {
         version = 1
     }
     filename := fmt.Sprintf("%s-v%d%s", ent.ID, version, ext)
     relPath := filepath.Join("assets", "portraits", filename)
     ```
   - Only remove stale portraits for the *same version* (`<id>-v<version>.*` with different extensions), keeping earlier versions intact.
   - In `updateNote`:
     - If `existingEnt.Portrait != ""` and `existingEnt.Portrait != relPath`, append `existingEnt.Portrait` to `existingEnt.PortraitHistory`.
     - Update `existingEnt.Portrait = relPath` and `existingEnt.PortraitVersion = version`.
2. Add `EnqueueVersion(gameID string, ent *entity.Entity, artStyle string, forceNewVersion bool)` to allow triggering when `AppearanceChanged` is true.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -count=1 ./pkg/engine/ -run "TestPortraitWorker.*"`
Expected: PASS

- [ ] **Step 5: Commit changes**

```bash
git add pkg/engine/portrait_worker.go pkg/engine/portrait_worker_test.go
git commit -m "feat(engine): add versioned portrait generation and history retention"
```

---

### Task 4: Scene Worker & Turn Scene Illustration Pipeline

**Files:**
- Create: `pkg/engine/scene_worker.go`
- Create: `pkg/engine/scene_worker_test.go`

- [ ] **Step 1: Write failing test for `SceneWorker`**

In `pkg/engine/scene_worker_test.go`:
```go
package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
)

type mockSceneGen struct {
	data []byte
	err  error
}

func (m *mockSceneGen) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	return m.data, m.err
}

func TestBuildScenePrompt(t *testing.T) {
	loc := &entity.Entity{
		ID:         "tavern",
		Name:       "The Rusty Nail",
		Appearance: "Old wooden beams and a cracked hearth.",
	}
	cue := "Ten years later, the courtyard is quiet and mossy."
	prompt := BuildScenePrompt(cue, loc, "moody oil painting")

	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !strings.Contains(prompt, cue) {
		t.Errorf("expected prompt to contain cue, got %q", prompt)
	}
	if !strings.Contains(prompt, "The Rusty Nail") {
		t.Errorf("expected prompt to contain location name, got %q", prompt)
	}
}

func TestSceneWorker_GeneratesTurnSceneIllustration(t *testing.T) {
	tempDir := t.TempDir()
	resolver := core.NewPathResolver(filepath.Join(tempDir, "config"), filepath.Join(tempDir, "data"), filepath.Join(tempDir, "cache"))
	gameID := "test-game"
	_ = os.MkdirAll(filepath.Join(resolver.GameDir(gameID), "assets", "scenes"), 0755)

	gen := &mockSceneGen{data: []byte("<svg>scene turn 10</svg>")}
	worker := NewSceneWorker(resolver, gen)

	readyCh := make(chan string, 1)
	worker.SetOnReady(func(gID string, turnNum int, relPath string) {
		if gID == gameID && turnNum == 10 {
			readyCh <- relPath
		}
	})

	worker.Enqueue(gameID, 10, "A dramatic autumn scene")

	select {
	case relPath := <-readyCh:
		expected := filepath.Join("assets", "scenes", "turn-10.svg")
		if relPath != expected {
			t.Errorf("expected relPath %s, got %s", expected, relPath)
		}
		fullPath := filepath.Join(resolver.GameDir(gameID), relPath)
		if _, err := os.Stat(fullPath); err != nil {
			t.Fatalf("expected scene file on disk: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for scene image generation")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -count=1 ./pkg/engine/ -run "TestBuildScenePrompt|TestSceneWorker_GeneratesTurnSceneIllustration"`
Expected: FAIL due to missing `SceneWorker` and `BuildScenePrompt`.

- [ ] **Step 3: Implement `SceneWorker` and prompt builder**

In `pkg/engine/scene_worker.go`:
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
)

// BuildScenePrompt composes the generation prompt for a turn scene illustration.
func BuildScenePrompt(visualCue string, location *entity.Entity, worldStyle string) string {
	parts := make([]string, 0, 4)
	if cue := strings.TrimSpace(visualCue); cue != "" {
		parts = append(parts, cue)
	}
	if location != nil {
		if locName := strings.TrimSpace(location.Name); locName != "" {
			parts = append(parts, "location: "+locName)
		}
		if appearance := strings.TrimSpace(location.Appearance); appearance != "" {
			parts = append(parts, appearance)
		}
	}
	if style := strings.TrimSpace(worldStyle); style != "" {
		parts = append(parts, style)
	}
	parts = append(parts, "cinematic scene illustration, high quality, atmospheric lighting, detailed environment, no text, no borders")
	return strings.Join(parts, ", ")
}

type SceneGenerator interface {
	GenerateImage(ctx context.Context, prompt string) ([]byte, error)
}

type SceneWorker struct {
	mu        sync.Mutex
	resolver  *core.PathResolver
	generator SceneGenerator
	inFlight  map[string]bool
	onReady   func(gameID string, turnNumber int, relPath string)
}

func NewSceneWorker(resolver *core.PathResolver, gen SceneGenerator) *SceneWorker {
	return &SceneWorker{
		resolver:  resolver,
		generator: gen,
		inFlight:  make(map[string]bool),
	}
}

func (w *SceneWorker) SetOnReady(fn func(gameID string, turnNumber int, relPath string)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onReady = fn
}

func (w *SceneWorker) Enqueue(gameID string, turnNumber int, prompt string) {
	if w == nil || w.generator == nil || gameID == "" || turnNumber <= 0 || strings.TrimSpace(prompt) == "" {
		return
	}

	key := fmt.Sprintf("%s:%d", gameID, turnNumber)
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

		_, _ = w.writeScene(context.Background(), gameID, turnNumber, prompt)
	}()
}

func (w *SceneWorker) writeScene(ctx context.Context, gameID string, turnNumber int, prompt string) (string, error) {
	imgBytes, err := w.generator.GenerateImage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("generate scene image: %w", err)
	}
	if len(imgBytes) == 0 {
		return "", fmt.Errorf("scene generator returned no image")
	}

	ext := media.ArtExtension(imgBytes)
	if ext == "" {
		ext = ".png"
	}

	filename := fmt.Sprintf("turn-%d%s", turnNumber, ext)
	relPath := filepath.Join("assets", "scenes", filename)
	fullPath := filepath.Join(w.resolver.GameDir(gameID), relPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("create scene dir: %w", err)
	}
	if err := os.WriteFile(fullPath, imgBytes, 0644); err != nil {
		return "", fmt.Errorf("write scene image: %w", err)
	}

	w.mu.Lock()
	cb := w.onReady
	w.mu.Unlock()
	if cb != nil {
		cb(gameID, turnNumber, relPath)
	}
	return relPath, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -count=1 ./pkg/engine/ -run "TestBuildScenePrompt|TestSceneWorker_GeneratesTurnSceneIllustration"`
Expected: PASS

- [ ] **Step 5: Commit changes**

```bash
git add pkg/engine/scene_worker.go pkg/engine/scene_worker_test.go
git commit -m "feat(engine): add scene worker for asynchronous turn illustrations"
```

---

### Task 5: GUI Service & HTTP Routes for Scene Art and Versioned Portraits

**Files:**
- Modify: `pkg/gui/types.go`
- Modify: `pkg/gui/service.go`
- Modify: `pkg/gui/server.go`
- Test: `pkg/gui/service_test.go`

- [ ] **Step 1: Write failing tests for scene image endpoint and versioned portrait query**

In `pkg/gui/service_test.go`:
```go
func TestGetTurnSceneImage(t *testing.T) {
	svc, gameID := setupTestService(t)
	scenesDir := filepath.Join(svc.resolver.GameDir(gameID), "assets", "scenes")
	_ = os.MkdirAll(scenesDir, 0755)
	sceneFile := filepath.Join(scenesDir, "turn-5.png")
	pngBytes := []byte("\x89PNG\r\n\x1a\nfake png data")
	_ = os.WriteFile(sceneFile, pngBytes, 0644)

	data, contentType, err := svc.GetTurnSceneImage(context.Background(), gameID, 5)
	if err != nil {
		t.Fatalf("GetTurnSceneImage failed: %v", err)
	}
	if contentType != "image/png" {
		t.Errorf("expected contentType image/png, got %s", contentType)
	}
	if !bytes.Equal(data, pngBytes) {
		t.Errorf("data mismatch")
	}

	// Turn 99 (does not exist) returns error
	_, _, err = svc.GetTurnSceneImage(context.Background(), gameID, 99)
	if err == nil {
		t.Errorf("expected error for non-existent scene image, got nil")
	}
}

func TestGetCharacterPortraitVersionQuery(t *testing.T) {
	svc, gameID := setupTestService(t)
	portraitsDir := filepath.Join(svc.resolver.GameDir(gameID), "assets", "portraits")
	_ = os.MkdirAll(portraitsDir, 0755)

	v1Bytes := []byte("\x89PNG\r\n\x1a\nportrait v1")
	v2Bytes := []byte("\x89PNG\r\n\x1a\nportrait v2")
	_ = os.WriteFile(filepath.Join(portraitsDir, "elena-v1.png"), v1Bytes, 0644)
	_ = os.WriteFile(filepath.Join(portraitsDir, "elena-v2.png"), v2Bytes, 0644)

	// Save entity note with v2 active
	ent := &entity.Entity{
		ID:              "elena",
		Name:            "Elena",
		Type:            "character",
		Portrait:        "assets/portraits/elena-v2.png",
		PortraitVersion: 2,
		PortraitHistory: []string{"assets/portraits/elena-v1.png"},
	}
	noteBytes, _ := ent.SerializeMarkdown()
	_ = os.WriteFile(filepath.Join(svc.resolver.GameDir(gameID), "entities", "elena.md"), noteBytes, 0644)

	// Requesting v=1 returns v1 bytes
	data1, _, err := svc.GetCharacterPortrait(context.Background(), gameID, "elena", 1)
	if err != nil {
		t.Fatalf("GetCharacterPortrait v=1 failed: %v", err)
	}
	if !bytes.Equal(data1, v1Bytes) {
		t.Errorf("expected v1 bytes, got %s", string(data1))
	}

	// Requesting without version returns active v2 bytes
	data2, _, err := svc.GetCharacterPortrait(context.Background(), gameID, "elena")
	if err != nil {
		t.Fatalf("GetCharacterPortrait default failed: %v", err)
	}
	if !bytes.Equal(data2, v2Bytes) {
		t.Errorf("expected v2 bytes, got %s", string(data2))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -count=1 ./pkg/gui/ -run "TestGetTurnSceneImage|TestGetCharacterPortraitVersionQuery"`
Expected: FAIL due to missing `GetTurnSceneImage` and unversioned `GetCharacterPortrait`.

- [ ] **Step 3: Implement GUI service endpoints and routes**

1. In `pkg/gui/types.go`:
   - `TurnDTO`: add `SceneBreak bool json:"scene_break,omitempty"`
   - `SegmentDTO`: add `SpeakerPortrait string json:"speaker_portrait,omitempty"`
   - `TurnEvent`: add `TurnNumber int json:"turn_number,omitempty"`, `Version int json:"version,omitempty"`
2. In `pkg/gui/service.go`:
   - Add `sceneWorker *engine.SceneWorker` to `Service`.
   - Implement `GetTurnSceneImage(ctx context.Context, gameID string, turnNumber int) ([]byte, string, error)`:
     Reads `assets/scenes/turn-<turnNumber>.*`, returns bytes and MIME type.
   - Update `GetCharacterPortrait(ctx context.Context, gameID, characterID string, version ...int) ([]byte, string, error)`:
     If `len(version) > 0 && version[0] > 0`: check `assets/portraits/<characterID>-v<version>.*`, falling back to `portrait_history` or legacy file.
   - Implement `broadcastSceneImageReady(gameID string, turnNumber int, relPath string)` firing `TurnEvent{Type: "scene_image", TurnNumber: turnNumber, ImageURL: fmt.Sprintf("/api/game/%s/turn/%d/scene-image", gameID, turnNumber)}`.
3. In `pkg/gui/server.go`:
   - Route `GET /api/game/{gameID}/turn/{turnNumber}/scene-image` -> calls `service.GetTurnSceneImage`.
   - Update `GET /api/game/{gameID}/character/{characterID}/portrait` route to parse query `v`, e.g. `strconv.Atoi(r.URL.Query().Get("v"))`, and pass to `GetCharacterPortrait`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -count=1 ./pkg/gui/ -run "TestGetTurnSceneImage|TestGetCharacterPortraitVersionQuery"`
Expected: PASS

- [ ] **Step 5: Commit changes**

```bash
git add pkg/gui/types.go pkg/gui/service.go pkg/gui/server.go pkg/gui/service_test.go
git commit -m "feat(gui): add scene image and versioned portrait endpoints"
```

---

### Task 6: Turn Orchestrator & Turn DTO Integration

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Modify: `pkg/gui/service.go`
- Test: `pkg/engine/orchestrator_test.go`
- Test: `pkg/gui/service_test.go`

- [ ] **Step 1: Write failing test for orchestrator scene break detection and speaker portrait anchoring**

In `pkg/engine/orchestrator_test.go`:
```go
func TestOrchestrator_DetectsSceneBreakAndAnchorsSpeakerPortraits(t *testing.T) {
	// Verify that when prose contains '---' or extractor signals scene_break,
	// turn.SceneBreak is set to true.
	// Verify speech segments anchor speaker portrait with ?v=<version>.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -count=1 ./pkg/engine/ -run "TestOrchestrator_DetectsSceneBreakAndAnchorsSpeakerPortraits"`
Expected: FAIL.

- [ ] **Step 3: Implement orchestrator and service turn mapping logic**

1. In `pkg/engine/orchestrator.go`:
   - Check if `turn.Narration` contains a standalone `---` rule or if `extraction.SceneBreak != nil && extraction.SceneBreak.Occurred`.
   - If true: mark `turn.SceneBreak = true`.
   - When building dialogue / speech segments:
     - Check speaker character's current `ent.PortraitVersion`.
     - Populate `segment.SpeakerPortrait` with `/api/game/<gameID>/character/<speakerID>/portrait?v=<version>`.
   - If `extraction.Entities` has any character with `AppearanceChanged: true`, invoke `portraitWorker` to generate next version.
   - If `turn.SceneBreak` is true, invoke `sceneWorker.Enqueue(gameID, turn.Number, scenePrompt)`.
2. In `pkg/gui/service.go`:
   - In `turnDTO`:
     - Set `dto.SceneBreak = turn.SceneBreak`.
     - If `assets/scenes/turn-<N>.*` exists, set `dto.ImageURL = fmt.Sprintf("/api/game/%s/turn/%d/scene-image", gameID, turn.Number)`.
   - In `segmentDTOs`:
     - If `segment.SpeakerPortrait != ""`, set `dto.SpeakerPortrait = segment.SpeakerPortrait` and `dto.PortraitURL = segment.SpeakerPortrait`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -count=1 ./pkg/engine/... ./pkg/gui/...`
Expected: PASS

- [ ] **Step 5: Commit changes**

```bash
git add pkg/engine/orchestrator.go pkg/gui/service.go pkg/engine/orchestrator_test.go pkg/gui/service_test.go
git commit -m "feat(orchestrator): anchor turn portraits and trigger scene illustrations"
```

---

### Task 7: Frontend Turn Presentation & Real-Time Event Handlers

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/components/TurnSegments.tsx`
- Modify: `frontend/src/components/ChronicleView.tsx`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Update TypeScript types**

In `frontend/src/types.ts`:
```ts
export interface TurnDTO {
  // ...
  image_url?: string;
  scene_break?: boolean;
  // ...
}

export interface TurnSegment {
  // ...
  speaker_portrait?: string;
  // ...
}

export interface TurnEvent {
  // ...
  turn_number?: number;
  version?: number;
  // ...
}
```

- [ ] **Step 2: Update `TurnSegments.tsx` to preserve historical portraits**

In `frontend/src/components/TurnSegments.tsx`:
Update portrait URL resolution:
```tsx
const portraitURL = segment.speaker_portrait || segment.portrait_url ||
  (charId && characterPortraits?.[charId]?.url);
```
Historical speech segments with `speaker_portrait` or versioned `portrait_url` permanently display their era's portrait, avoiding override by current live state.

- [ ] **Step 3: Update `App.tsx` and `ChronicleView.tsx` for real-time scene illustrations**

In `frontend/src/App.tsx`:
Handle `scene_image` turn events:
```ts
if (event.type === 'scene_image' && event.turn_number && event.image_url) {
  setChronicle((prev) =>
    prev.map((turn) =>
      turn.turn_number === event.turn_number
        ? { ...turn, image_url: event.image_url }
        : turn
    )
  );
}
```

In `frontend/src/components/ChronicleView.tsx`:
If `turn.scene_break` is true, render a styled horizontal rule / scene transition marker before the turn's prose.
Ensure `turn.image_url` lightbox card renders whenever present.

- [ ] **Step 4: Run frontend TypeScript checks**

Run: `mise run test:frontend` (or `cd frontend && npx tsc --noEmit`)
Expected: PASS with 0 errors.

- [ ] **Step 5: Commit changes**

```bash
git add frontend/src/types.ts frontend/src/components/TurnSegments.tsx frontend/src/components/ChronicleView.tsx frontend/src/App.tsx
git commit -m "feat(frontend): support scene break illustrations and historical portraits"
```

---

### Task 8: Full End-to-End Integration Verification & Test Suite

**Files:**
- Test: All unit, e2e, and integration tests across backend and frontend

- [ ] **Step 1: Run full backend test suite**

Run: `mise run test:backend`
Expected: PASS with all tests passing.

- [ ] **Step 2: Run full frontend test suite**

Run: `mise run test:frontend`
Expected: PASS with 0 errors.

- [ ] **Step 3: Run full linter suite**

Run: `mise run lint`
Expected: PASS with clean vet, markdownlint, and actionlint.

- [ ] **Step 4: Commit any final test cleanups or doc updates**

```bash
git commit --allow-empty -m "chore: verify full test suite passes for image triggers and history"
```
