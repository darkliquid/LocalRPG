# Built-in Help and Documentation System Design

**Date:** 2026-09-27
**Status:** Approved
**Scope:** Built-in comprehensive documentation and usage guides accessible via the launcher dock and chronicle view
**Related:** `pkg/gui`, `frontend/src/components/`, `frontend/src/api/`

## 1. Overview & Goals

LocalRPG is a local-first, schema-agnostic turn-based RPG client. Because all RPG mechanics, lore, and content are governed by YAML frontmatter, JavaScript mechanics hooks, and prompt contracts rather than hardcoded classes or stats, users need comprehensive in-app guidance on how the system works and how to author content.

This specification details a built-in documentation system embedded within the application binary. Users can open documentation from anywhere in the interface—either from the launcher dock or while playing a campaign in the chronicle view—to read conceptual explanations, settings/provider guides, studio authoring walkthroughs, and Codex YAML specifications.

### Goals
- Embed comprehensive, structured Markdown documentation directly into the Go binary (`//go:embed`).
- Serve documentation metadata and article content via lightweight `/api/docs` endpoints.
- Provide a responsive, two-pane studio modal (`DocsModal`) with instant search, category filtering, reading breadcrumbs, code blocks with syntax highlighting and one-click copy.
- Expose clear entry points:
  - Launcher Dock: A new Help/Docs button (`HelpCircle`) at the bottom of the left dock.
  - Chronicle Top Bar: A "Docs" button in the floating header navigation.
  - Contextual deep linking: Ability for studios and drawers (e.g. Codex editor) to open directly to relevant guides.
- Cover all core topics:
  - Architecture and core concepts (Worlds, Systems, Campaigns, Turns).
  - Main configuration and providers (LLM, TTS, STT, Image, Agents, Storage/Paths).
  - Studio guides (Building Worlds, Building Custom Systems & `mechanics.js`).
  - Codex & Content Reference (YAML frontmatter schema, entity types, graph modeling via wikilinks).

### Non-Goals
- External web scraping or online doc fetching (all documentation is 100% offline and embedded).
- Editing or modifying documentation articles from within the UI (documentation is authoritative and read-only; authored content is created via Worlds/Systems/Campaigns).
- Full Markdown WYSIWYG editor (the viewer renders Markdown with syntax highlighting and copy buttons).

---

## 2. Architecture & Data Model

### 2.1 Backend Embedded Markdown Storage

Documentation articles live as standalone `.md` files in `pkg/gui/docs/`.
Each article begins with a standard YAML frontmatter header defining its metadata:

```markdown
---
id: codex-yaml
title: Codex Frontmatter Reference
category: Codex & Content
order: 10
description: Complete YAML frontmatter reference for codex entities and lore notes.
---

# Codex Frontmatter Reference
...
```

The Go package `pkg/gui` embeds these files at build time:
```go
// pkg/gui/docs.go
package gui

import (
    "embed"
    // ...
)

//go:embed docs/*.md
var embeddedDocsFS embed.FS
```

### 2.2 Data Transfer Objects (DTOs)

In `pkg/gui/types.go`:

```go
// DocArticleSummaryDTO describes an article in documentation navigation and search.
type DocArticleSummaryDTO struct {
    ID          string `json:"id"`
    Title       string `json:"title"`
    Category    string `json:"category"`
    Order       int    `json:"order"`
    Description string `json:"description"`
}

// DocArticleDTO contains the full article content including markdown prose.
type DocArticleDTO struct {
    DocArticleSummaryDTO
    Content string `json:"content"`
}
```

### 2.3 Service & Server API

In `pkg/gui/service.go`:
- `GetDocsList(ctx context.Context) ([]DocArticleSummaryDTO, error)`: Parses the frontmatter headers of all embedded markdown files and returns them sorted by `category` and `order`.
- `GetDocArticle(ctx context.Context, id string) (*DocArticleDTO, error)`: Retrieves the frontmatter and full markdown body for the specified article ID.

In `pkg/gui/server.go`:
- `GET /api/docs`: Returns JSON list of all article summaries: `[]DocArticleSummaryDTO`.
- `GET /api/docs/{id}`: Returns JSON `DocArticleDTO` containing frontmatter metadata and `content`.

---

## 3. Documentation Article Catalog

The documentation library contains 10 foundational guides structured into 4 categories:

### Category 1: Core Concepts
1. **`01-overview.md` (Architecture & Core Concepts)**
   - Schema-agnostic design: no hardcoded stats, classes, or mana. State is driven by YAML frontmatter and JS/Wasm hooks.
   - The three-tier separation: `systems/` (rules), `worlds/` (lore), and `games/` (campaigns).
   - How AI agents (GM, Narrator, Extractor) interact with engine rules.
2. **`02-worlds.md` (Worlds & Lore)**
   - World manifest anatomy (`world.yaml`) and lore instructions (`prompts/lore.md`).
   - Starting locations: `settings.start_location` resolution, player wikilink preference, and fallback scenes.
   - System overrides (`system_overrides/<system-id>/hooks.js`).
3. **`03-systems.md` (Systems & Mechanics)**
   - Rules prompts (`prompts/rules.md`) and action modes: `do`, `say`, `story`, `roll`.
   - Dice notations (`2d6`, `1d20+3`, `4dF`), checks, difficulty, and outcomes.
   - Sandboxed JavaScript mechanics engine (`mechanics.js`) and lifecycle hooks (`resolveAction`, `onTurnStart`, `onTurnEnd`).
4. **`04-campaigns.md` (Campaigns & Turns)**
   - Turn execution flow: Player input → GM directives → dice/hook evaluation → narration rewrite → entity extraction → timeline indexing.
   - The `history.jsonl` append-only log and non-destructive `/undo` rewinding.
   - Narrative arcs, faction clocks, and dynamic relationship graph updates.

### Category 2: Configuration & Providers
5. **`05-providers.md` (AI & Media Providers)**
   - Provider types: `builtin`, `cli`, `http`, `mock`.
   - LLMs: Ollama, Gemini API, OpenAI API, Anthropic/custom OpenAI-compatible endpoints, Narrative Oracle.
   - Media: TTS (ElevenLabs, Gemini Voice, Native OS, Piper, Sherpa), STT (Whisper HTTP, CLI), and Image generation (Procedural Art SVG, Imagen, ComfyUI/A1111).
   - Rate limit backoffs (429), Insufficient Funds (402), and Spend Ledger tracking.
6. **`06-agents.md` (AI Agents & Roles)**
   - Agent roles: `gm`, `narrator`, `extractor`.
   - Role inheritance, model overrides, temperature, context token budgets, and fallback chains.
7. **`07-storage-paths.md` (Storage & Directories)**
   - XDG data and cache bases (`~/.local/share/localrpg`, `~/.cache/localrpg`).
   - Workspace mode: relative path resolution and project overrides (`localrpg.yaml`).
   - Cache safety: disposable `cache/index.db` that syncs from Markdown notes.

### Category 3: Studio Creation Guides
8. **`08-worlds-studio.md` (Building Worlds)**
   - Step-by-step walkthrough in the Worlds Studio GUI.
   - Defining genre, art style tags, and lore prompts.
   - Generating procedural banners and icons.
   - Creating starter templates and entities before launching campaigns.
9. **`09-systems-studio.md` (Building Custom Systems)**
   - Authoring GM rules prompts that instruct the LLM on calling checks.
   - Authoring `mechanics.js`: exporting `resolveAction(ctx)` and parsing custom rules.
   - Live testing of dice expressions and roll tables in the studio tester.

### Category 4: Codex & Content Reference
10. **`10-codex-yaml.md` (Codex YAML Frontmatter & Graph Reference)**
    - Markdown note structure: YAML frontmatter block between `---` fences followed by body prose.
    - Supported frontmatter schema keys:
      - `name`: Entity display name
      - `type`: `character` | `location` | `faction` | `item` | `concept`
      - `tags`: Array of string descriptors (e.g. `[npc, merchant, hostile]`)
      - `location`: Wikilink or slug of current location
      - `faction`: Wikilink or slug of affiliated faction
      - `voice`: Embedded voice configuration (`provider`, `voice_id`, `pitch`, `rate`)
      - `state`: Custom key-value mechanics properties (e.g. `hp: 14`, `equipped: [[iron-sword]]`)
      - `inventory`: Array of carried items or wikilinks
    - Graph modeling: How `[[Target Entity|Display Label]]` wikilinks and frontmatter fields form directional edges in the campaign knowledge graph.

---

## 4. Frontend Component & UX Design

### 4.1 Documentation Studio Modal (`DocsModal.tsx`)
A centered modal matching the glassmorphic aesthetic of `SettingsStudio` and `WorldsStudio`.
- **Top Bar**:
  - Title with book icon (`BookOpen` / `HelpCircle`): "Documentation & Reference".
  - Quick Search input with search icon: live filtering by title, category, and content keywords.
  - Close button (`X` icon and `Escape` keyboard shortcut).
- **Left Sidebar**:
  - Grouped category list with count badges:
    - 📖 Core Concepts
    - ⚙️ Configuration & Providers
    - 🛠️ Studio Guides
    - 📜 Codex & YAML Reference
  - List of articles within each category. Clicking selects and loads the article.
  - Active article is highlighted with purple accent background and glow border.
- **Right Reader Pane**:
  - Breadcrumb navigation (`Docs > Studio Guides > Building Custom Systems`).
  - Article title and description.
  - Rendered Markdown using an enhanced `MarkdownDocViewer`:
    - Headings with hierarchy styling.
    - Syntax-highlighted code blocks with language labels (e.g. `yaml`, `javascript`, `markdown`) and a one-click **Copy** button.
    - Callout boxes (`NOTE`, `TIP`, `IMPORTANT`, `WARNING`).
    - Tables and bulleted lists.
  - Footer with "← Previous" and "Next →" article navigation links.

### 4.2 Entry Points
1. **Launcher Dock (`LauncherDock.tsx`)**:
   - New button at the bottom of the dock, positioned below `Settings`.
   - Icon: `HelpCircle`.
   - Tooltip: "Help & Documentation".
   - Clicking opens `DocsModal`.
2. **Chronicle Top Bar (`App.tsx`)**:
   - New button in the acrylic header alongside Character, Graph, Codex, World, Context, Theater, and Settings.
   - Icon: `HelpCircle`.
   - Label: "Docs".
   - Tooltip: "Help & Documentation".
   - Clicking opens `DocsModal` over the current campaign without disrupting the turn or state.
3. **Contextual Deep Linking**:
   - `DocsModal` accepts `initialArticleID?: string`.
   - Allows drawers (such as `CodexDrawer`) or studio panels to provide direct "Help" links opening straight to the relevant guide (e.g., `codex-yaml`).

---

## 5. Verification & Testing Plan

### 5.1 Backend Tests
- `pkg/gui/docs_test.go`:
  - Verify embedded markdown files are loaded without error.
  - Verify frontmatter parsing extracts valid `id`, `title`, `category`, and `order`.
  - Verify `GET /api/docs` returns all 10 articles sorted correctly.
  - Verify `GET /api/docs/{id}` returns the complete markdown content.
  - Verify requesting an unknown document ID returns HTTP 404.

### 5.2 Frontend Tests & Type Checking
- `npx tsc --noEmit` and `mise run test:frontend`: Confirm zero TypeScript compile errors.
- `mise run build`: Confirm Vite bundles `DocsModal` and embedded Go binary builds cleanly.
- `mise run lint`: Confirm `go vet ./...` clean.
- Manual smoke verification:
  - Open documentation from launcher dock.
  - Open documentation from chronicle view top bar.
  - Verify search filter dynamically narrows articles.
  - Verify code block copy button copies YAML snippets to clipboard.
  - Verify responsive layout on compact/standard viewport sizes.
