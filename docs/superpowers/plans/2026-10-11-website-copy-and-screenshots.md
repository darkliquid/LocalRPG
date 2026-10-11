# Website Copy and Screenshots Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refresh the LocalRPG showcase website copy, README documentation, gallery captions, and screenshots to accurately represent v0.5.0 features including AI World Generation, Systems Studio test scenarios, local ONNX embeddings, restructured settings tabs, and content packages.

**Architecture:** Update `tools/sitegen/templates/home.html.tmpl` and `README.md` with the new 6-pillar messaging. Update gallery captions in `tools/sitegen/content.go`. Use headless scenario automation (`website/scenarios/screenshots.yaml` and `website/capture-screenshots.sh`) and live UI driver to capture updated 1280x800 JPEG screenshots for `website/screenshots/`. Verify site rendering with `tools/sitegen` and style compliance with Vale and rumdl.

**Tech Stack:** Go (html/template, goldmark, sitegen tool), Markdown, Bash, Chrome DevTools headless automation for screenshots, Vale prose linter, rumdl markdown linter.

---

### Task 1: Update Homepage Copy (`tools/sitegen/templates/home.html.tmpl`)

**Files:**
- Modify: `tools/sitegen/templates/home.html.tmpl:6-112`
- Test: `tools/sitegen/sitegen_test.go`

- [ ] **Step 1: Write a test asserting that home page template renders the new pillar keywords**

In `tools/sitegen/sitegen_test.go`, add `TestHomePageContentPillars`:

```go
func TestHomePageContentPillars(t *testing.T) {
	renderer, err := newRenderer()
	if err != nil {
		t.Fatalf("newRenderer: %v", err)
	}
	page, err := renderer.render("home.html.tmpl", &pageData{
		Root:        "",
		Title:       "Home",
		Description: "A local-first tabletop RPG client",
		IsHome:      true,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	html := string(page)

	requiredPhrases := []string{
		"AI-assisted generation",
		"automated test scenarios",
		"First-Class Local AI &amp; Embeddings",
		"AI World Generation &amp; Living Lore",
		"Custom Systems &amp; Scenario Testing",
		"Dynamic Scene Art &amp; Style Packs",
		"TTS Narration &amp; Voice Dictation",
		"Story Theatre, Packages &amp; Export",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(html, phrase) {
			t.Errorf("rendered home page missing required phrase: %q", phrase)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestHomePageContentPillars ./tools/sitegen`
Expected: FAIL with missing required phrases.

- [ ] **Step 3: Update `tools/sitegen/templates/home.html.tmpl`**

Replace lines 6-26 and 80-112 in `tools/sitegen/templates/home.html.tmpl` with the updated hero lede, hero facts, and 6 feature cards:

```html
    <p class="hero__lede">
      Craft original worlds with AI-assisted generation, build custom tabletop rules with automated
      test scenarios, and embark on solo campaigns with local AI as a first-class citizen. LocalRPG
      brings your adventures to life with multi-voice narration, speech-to-text dictation, style-pack
      scene imagery, and shareable visual novel exports.
    </p>
    <div class="hero__actions">
      <a class="button button--primary" href="{{ .RepoURL }}/releases" target="_blank" rel="noopener">
        {{ icon "download" }} Download
      </a>
      <a class="button" href="{{ .Root }}docs/index.html">{{ icon "book" }} Read the docs</a>
      <a class="button button--ghost" href="{{ .RepoURL }}" target="_blank" rel="noopener">
        {{ icon "github" }} Source
      </a>
    </div>
    <ul class="hero__facts">
      <li><strong>Local AI</strong> First-class local &amp; ONNX embeddings</li>
      <li><strong>Voices</strong> Narration, NPC speech &amp; STT dictation</li>
      <li><strong>Theatre</strong> Replay &amp; export web/video bundles</li>
    </ul>
```

And update the `.cards` container:

```html
  <div class="cards">
    <article class="card glass">
      <span class="card__icon">{{ icon "cpu" }}</span>
      <h3>First-Class Local AI &amp; Embeddings</h3>
      <p>Connect local LLMs via Ollama, LM Studio, or vLLM, run pure-local ONNX embeddings for semantic memory, or play instantly with zero-GPU built-ins that require no setup.</p>
    </article>
    <article class="card glass">
      <span class="card__icon">{{ icon "sparkles" }}</span>
      <h3>AI World Generation &amp; Living Lore</h3>
      <p>Author factions, locations, and starter lore by hand, or generate complete, cohesive settings with schema-first AI pipelines, batch entity expansion, and draft review.</p>
    </article>
    <article class="card glass">
      <span class="card__icon">{{ icon "shield" }}</span>
      <h3>Custom Systems &amp; Scenario Testing</h3>
      <p>Design dice mechanics, outcome tiers, and character creation prompts with worked examples in Systems Studio, and prove system balance with automated test scenarios.</p>
    </article>
    <article class="card glass">
      <span class="card__icon">{{ icon "image" }}</span>
      <h3>Dynamic Scene Art &amp; Style Packs</h3>
      <p>Bring scenes to life with curated art style packs and procedural vector art, preserving permanent world aesthetics while reflecting dynamic character appearance changes.</p>
    </article>
    <article class="card glass">
      <span class="card__icon">{{ icon "layers" }}</span>
      <h3>TTS Narration &amp; Voice Dictation</h3>
      <p>Hear prose narrated aloud while NPCs speak with distinct voice profiles and pitch. Take your turns hands-free using click-to-talk speech-to-text dictation.</p>
    </article>
    <article class="card glass">
      <span class="card__icon">{{ icon "book" }}</span>
      <h3>Story Theatre, Packages &amp; Export</h3>
      <p>Relive adventures in visual novel Theatre Mode, share worlds and rules via standalone content packages, and export complete campaigns as web bundles or rendered video.</p>
    </article>
  </div>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestHomePageContentPillars ./tools/sitegen`
Expected: PASS

- [ ] **Step 5: Commit changes**

```bash
git add tools/sitegen/templates/home.html.tmpl tools/sitegen/sitegen_test.go
git commit -m "feat(site): update homepage hero copy and capability cards for v0.5.0"
```

---

### Task 2: Align Project Documentation (`README.md`)

**Files:**
- Modify: `README.md:11-30, 67-76`

- [ ] **Step 1: Update `README.md` features, creative cycle, and configuration sections**

Update `## Features` to:
```markdown
## Features

- **First-Class Local AI & Embeddings:** Route GM, narrator, and evaluator roles across local models (Ollama, LM Studio, vLLM, LocalAI) or cloud APIs, with local ONNX embeddings and zero-GPU built-ins.
- **AI World Generation & Living Lore:** Author lore, factions, and entities in Worlds Studio, or generate full settings through schema-first AI pipelines and draft review.
- **Custom Systems & Scenario Testing:** Design dice rules, outcome tiers, and character creation prompts with worked examples, verified against automated test scenarios.
- **Dynamic Scene Art & Style Packs:** Generate atmospheric scenes and character portraits with curated style packs, keeping baseline world aesthetics distinct from live campaign appearances.
- **TTS Narration & Voice Dictation:** Listen to narrated story prose with distinct voice profiles for each NPC, and interact hands-free with speech-to-text voice dictation.
- **Story Theatre, Packages & Export:** Step through visual novel replays, share creations via standalone content packages, and export campaigns as self-contained web bundles or rendered video.
```

Update `## Creating Worlds & Playing Campaigns` steps:
```markdown
1. **Craft a World in Worlds Studio:** Author lore, tone, factions, and key locations by hand or generate complete settings with schema-first AI pipelines, batch entity expansion, and draft review.
2. **Define Rules in Systems Studio:** Create custom resolution systems, dice rules, and character creation prompts with worked examples, and verify system balance using automated test scenarios.
3. **Embark on a Campaign:** Take actions, roll checks, and shape the story. Speak your actions with microphone dictation while the AI Game Master adjudicates rules and narrates vivid scenes with voice and art.
4. **Relive & Export in Story Theatre:** Review past sessions in visual novel Theatre Mode, share worlds and rules via standalone content packages, or export campaigns as interactive web bundles or rendered video.
```

Update `## Configuration & Provider Setup`:
```markdown
## Configuration & Provider Setup

LocalRPG features a unified settings system to connect your AI models and media engines:

- **Structured Studio Tabs:** Manage settings across dedicated tabs for Providers (LLMs, ONNX embeddings, shared API keys), AI Agents, Media Engines (TTS, STT, Image Generation), and Advanced preferences.
- **AI Agent Role Routing:** Route `gm`, `narrator`, and `evaluator` roles across local inference (Ollama, LM Studio, vLLM, LocalAI), cloud APIs, or built-in procedural engines.
- **Multimodal Engines:** Configure TTS (Piper, Kokoro, AllTalk, native-os), STT (Whisper, Web Speech API), and Image Generation (ComfyUI, Automatic1111, procedural-art) with volume, auto-play, and auto-generate art toggles.
- **Live Provider Diagnostics:** Test model and media engine connections directly from the UI with latency and preview feedback.
- **Dual-Access UI:** Access settings anytime from the **Settings** tab in Launcher Hub, or via the in-game header gear icon without leaving an active session.
```

- [ ] **Step 2: Verify prose and docs linting**

Run: `STRICT=1 bash scripts/lint-prose.sh`
Run: `rumdl check README.md`
Expected: 0 errors.

- [ ] **Step 3: Commit changes**

```bash
git add README.md
git commit -m "docs: align README features and studio workflows with v0.5.0"
```

---

### Task 3: Update Gallery Captions (`tools/sitegen/content.go`)

**Files:**
- Modify: `tools/sitegen/content.go:237-268`
- Test: `tools/sitegen/sitegen_test.go`

- [ ] **Step 1: Write a test for showcase shots captions**

In `tools/sitegen/sitegen_test.go`, add `TestShowcaseShotsCaptions`:

```go
func TestShowcaseShotsCaptions(t *testing.T) {
	shots := loadShots(t.TempDir())
	if len(shots) != 6 {
		t.Fatalf("expected 6 showcase shots, got %d", len(shots))
	}

	expectedSubstrings := map[string]string{
		"01-launcher.jpg":       "quick-start creation wizards",
		"02-worlds-studio.jpg":  "generate complete settings with AI pipelines",
		"03-systems-studio.jpg": "verify balance with test scenarios",
		"04-settings.jpg":       "ONNX embeddings",
		"05-codex.jpg":          "dynamic character appearances",
		"06-story-theatre.jpg":  "visual novel",
	}

	for _, s := range shots {
		sub, ok := expectedSubstrings[s.File]
		if !ok {
			t.Errorf("unexpected shot file: %s", s.File)
			continue
		}
		if !strings.Contains(s.Caption, sub) {
			t.Errorf("shot %s caption %q missing expected phrase %q", s.File, s.Caption, sub)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestShowcaseShotsCaptions ./tools/sitegen`
Expected: FAIL with captions missing expected phrases.

- [ ] **Step 3: Update `showcaseShots` in `tools/sitegen/content.go`**

Update `showcaseShots` to:

```go
var showcaseShots = []shot{
	{
		File:    "01-launcher.jpg",
		Title:   "Launcher Hub",
		Caption: "Campaigns, worlds, and systems dock with quick-start creation wizards, ready for immediate play.",
	},
	{
		File:    "02-worlds-studio.jpg",
		Title:   "Worlds Studio",
		Caption: "Author factions, lore, and entity trees by hand or generate complete settings with AI pipelines.",
	},
	{
		File:    "03-systems-studio.jpg",
		Title:   "Systems Studio",
		Caption: "Craft dice mechanics and action modes with worked examples, and verify balance with test scenarios.",
	},
	{
		File:    "04-settings.jpg",
		Title:   "Settings Studio",
		Caption: "Configure local LLMs, ONNX embeddings, multi-voice audio, and media engines across structured tabs.",
	},
	{
		File:    "05-codex.jpg",
		Title:   "Codex Drawer",
		Caption: "Inspect wikilinked entity lore, track dynamic character appearances, and customize voice profiles.",
	},
	{
		File:    "06-story-theatre.jpg",
		Title:   "Story Theatre",
		Caption: "Replay campaign scenes as an interactive visual novel, spoken in each character's distinct voice.",
	},
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestShowcaseShotsCaptions ./tools/sitegen`
Expected: PASS

- [ ] **Step 5: Commit changes**

```bash
git add tools/sitegen/content.go tools/sitegen/sitegen_test.go
git commit -m "feat(site): update showcase screenshot captions for v0.5.0"
```

---

### Task 4: Automated Screenshot Capture Updates (`website/scenarios/screenshots.yaml`)

**Files:**
- Modify: `website/scenarios/screenshots.yaml`
- Outputs: `website/screenshots/01-launcher.jpg`, `website/screenshots/02-worlds-studio.jpg`, `website/screenshots/03-systems-studio.jpg`, `website/screenshots/04-settings.jpg`

- [ ] **Step 1: Inspect and verify scenario selectors**

Check `website/scenarios/screenshots.yaml` to ensure:
1. `01-launcher.jpg`: Initial `/` load, with demo campaigns loaded.
2. `02-worlds-studio.jpg`: Navigates to Worlds Studio, showing the entity tree and authoring pane.
3. `03-systems-studio.jpg`: Navigates to Systems Studio, showing the Mechanics / Scenarios tabs.
4. `04-settings.jpg`: Navigates to Settings Studio, showing the Providers tab with Embeddings / LLM.

- [ ] **Step 2: Run screenshot capture script**

Run: `mise run site:screenshots`
Expected: Server starts on port 8080 against `website/demo`, test runner captures `01-launcher.jpg`, `02-worlds-studio.jpg`, `03-systems-studio.jpg`, `04-settings.jpg` into `website/screenshots/`.

- [ ] **Step 3: Verify the generated images**

Run: `file website/screenshots/01-launcher.jpg website/screenshots/02-worlds-studio.jpg website/screenshots/03-systems-studio.jpg website/screenshots/04-settings.jpg`
Verify: All 4 are JPEG images (1280x800).

- [ ] **Step 4: Commit updated studio screenshots**

```bash
git add website/scenarios/screenshots.yaml website/screenshots/01-launcher.jpg website/screenshots/02-worlds-studio.jpg website/screenshots/03-systems-studio.jpg website/screenshots/04-settings.jpg
git commit -m "feat(site): refresh studio screenshots 01-04 with updated UI"
```

---

### Task 5: Refresh Gameplay Screenshots (`05-codex.jpg` & `06-story-theatre.jpg`)

**Files:**
- Modify/Output: `website/screenshots/05-codex.jpg`, `website/screenshots/06-story-theatre.jpg`

- [ ] **Step 1: Capture or generate fresh `05-codex.jpg` and `06-story-theatre.jpg`**

Add scenario steps to a headless capture or live driver against `website/demo` campaign:
- Open campaign `website/demo/games/ashen-reach` (or demo fixture campaign).
- For `05-codex.jpg`: Open the Codex drawer, select a character entity note (showing traits, voice profile, and current appearance), capture 1280x800 JPEG.
- For `06-story-theatre.jpg`: Open Story Theatre for turn 1, with speaker portrait, stage backdrop, and dialogue segment visible, capture 1280x800 JPEG.

- [ ] **Step 2: Verify image format and resolution**

Run: `file website/screenshots/05-codex.jpg website/screenshots/06-story-theatre.jpg`
Verify: JPEG images, 1280x800.

- [ ] **Step 3: Commit gameplay screenshots**

```bash
git add website/screenshots/05-codex.jpg website/screenshots/06-story-theatre.jpg
git commit -m "feat(site): refresh gameplay screenshots 05-codex and 06-story-theatre"
```

---

### Task 6: Site Generation & Lint Verification

**Files:**
- Output: `website/dist/`

- [ ] **Step 1: Run static site build**

Run: `mise run site:build`
Expected: `sitegen: wrote 26 pages and 6/6 screenshots to website/dist`

- [ ] **Step 2: Run prose linting**

Run: `STRICT=1 bash scripts/lint-prose.sh`
Expected: 0 errors across all 26 files.

- [ ] **Step 3: Run markdown and repository lints**

Run: `mise run lint:docs`
Run: `go test ./tools/sitegen`
Expected: All tests and linters PASS.

- [ ] **Step 4: Final commit and verify git status**

Run: `git status`
Expected: Clean working tree.
