# New Entity Wizard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every authoring surface a wizard that scaffolds a valid, type-aware entity note and hands it to the existing editor.

**Architecture:** A backend catalogue (`pkg/gui/entitycatalog.go`) is the single source of truth for the offered entity types and the frontmatter keys each one scaffolds with, served at `GET /api/schema/entity-types`. A pure frontend function (`frontend/src/lib/entityScaffold.ts`) turns that catalogue plus a name, id, type and optional voice into markdown. One dialog (`NewEntityWizard.tsx`) drives that function and is wired into the codex, Content Studio, Worlds Studio and the continuity-repair modal.

**Tech Stack:** Go 1.27 (standard library only, no testify), React 19 + TypeScript 5.7 + Tailwind v4, CodeMirror 6, `yaml` npm package, node `check-*.mjs` scripts.

**Spec:** `docs/superpowers/specs/2026-05-10-new-entity-wizard-design.md`

## Global Constraints

- Go tests use `testing` and `t.TempDir()` only. No testify. Use `interface{}`, never `any`. `go vet` must stay clean.
- Errors are wrapped with `fmt.Errorf("...: %w", err)`.
- TypeScript has `strict`, `noUnusedLocals`, `noUnusedParameters`; `npx tsc --noEmit` is the frontend gate and fails on unused imports.
- Reuse `entity.Slugify` semantics: the frontend helper is `frontend/src/lib/slug.ts` `slugify`.
- Never hand-write the checked-in schema: regenerate with `go test ./pkg/gui -update-docs`.
- Conventional Commits with a scope, subject under 72 chars. Never commit unless the user asks.
- New route goes under the already-mounted `/api/schema/` prefix; do not add a mount to `pkg/gui/routes.go`, so `testdata/routes.json` is unchanged.

---

## File Map

| File | Task | Change |
| --- | --- | --- |
| `pkg/gui/entitycatalog.go` | 1 | New: catalogue, `buildEntityTypeCatalog()`, `GetEntityTypeCatalog`, DTOs |
| `pkg/gui/entitycatalog_test.go` | 1 | New: catalogue invariants |
| `pkg/gui/schema.go` | 1 | Extract `entityFrontmatterKeys()`; derive `frontmatterKeyValues["type"]` |
| `pkg/gui/schema/entity-frontmatter.json` | 1 | Regenerated |
| `pkg/gui/server.go` | 1 | Serve `/api/schema/entity-types` |
| `frontend/src/types.ts` | 2 | `EntityTypeSpec`, `EntityTypeCatalog` |
| `frontend/src/api/client.ts` | 2 | `getEntityTypes()` |
| `frontend/src/lib/entityScaffold.ts` | 3 | New: `buildEntityMarkdown` |
| `frontend/scripts/checkEntityScaffold.mjs` | 3 | New: generator check |
| `frontend/package.json` | 3 | `check:entity-scaffold` script |
| `mise.toml` | 3 | Add the check to `test:frontend` |
| `frontend/src/components/NewEntityWizard.tsx` | 4 | New: the dialog |
| `frontend/src/components/CodexDrawer.tsx` | 5 | "New Note" entry point |
| `frontend/src/components/ContentStudio.tsx` | 6 | "+ New" entry point |
| `frontend/src/components/WorldsStudio.tsx` | 7 | Replace the slug modal |
| `frontend/src/components/AddEntityModal.tsx` | 8 | "Edit in Codex" opens the wizard |
| `pkg/gui/docs/10-codex-yaml.md` | 9 | Types list and a "Creating a note" section |

---

### Task 1: Backend entity type catalogue and endpoint

**Files:**
- Create: `pkg/gui/entitycatalog.go`
- Create: `pkg/gui/entitycatalog_test.go`
- Modify: `pkg/gui/schema.go:166-201` (extract `entityFrontmatterKeys`, derive the `type` values)
- Modify: `pkg/gui/server.go:160-176` (`handleSchemaRoutes`)
- Regenerate: `pkg/gui/schema/entity-frontmatter.json`

**Interfaces:**
- Consumes: `frontmatterKeyDescriptions` and `frontmatterKeyValues` (`pkg/gui/schema.go`), `FrontmatterKeySchema` (`pkg/gui/schema.go:107`), `entity.EntityFrontmatter` (`pkg/entity/entity.go:28`), `writeJSON` and `writeGameError` (`pkg/gui/server.go`).
- Produces: `type EntityTypeSpec`, `type EntityTypeCatalog`, `func buildEntityTypeCatalog() EntityTypeCatalog`, `func (s *Service) GetEntityTypeCatalog(context.Context) (EntityTypeCatalog, error)`, and the route `GET /api/schema/entity-types`.

- [ ] **Step 1: Write the failing test**

Create `pkg/gui/entitycatalog_test.go`:

```go
package gui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEntityTypeCatalogKeysResolve(t *testing.T) {
	catalog := buildEntityTypeCatalog()
	if len(catalog.Types) == 0 {
		t.Fatal("the catalogue lists no types")
	}
	for _, spec := range catalog.Types {
		if spec.ID == "" || spec.Label == "" || spec.Description == "" {
			t.Errorf("type %q is missing an id, label or description", spec.ID)
		}
		if len(spec.Keys) == 0 {
			t.Errorf("type %q scaffolds no keys", spec.ID)
		}
		seen := map[string]bool{}
		for _, key := range spec.Keys {
			if strings.TrimSpace(key.Description) == "" {
				t.Errorf("type %q key %q has no description", spec.ID, key.Name)
			}
			if strings.TrimSpace(key.Type) == "" {
				t.Errorf("type %q key %q has no type", spec.ID, key.Name)
			}
			if seen[key.Name] {
				t.Errorf("type %q lists key %q twice", spec.ID, key.Name)
			}
			seen[key.Name] = true
		}
		for _, required := range []string{"id", "name", "type", "state"} {
			if !seen[required] {
				t.Errorf("type %q does not scaffold the %q key", spec.ID, required)
			}
		}
	}
}

func TestEntityTypeCatalogTypeIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, spec := range buildEntityTypeCatalog().Types {
		if seen[spec.ID] {
			t.Errorf("type %q is listed twice", spec.ID)
		}
		seen[spec.ID] = true
	}
}

func TestEntityTypeCatalogTypeValuesMatchTypes(t *testing.T) {
	catalog := buildEntityTypeCatalog()
	ids := make([]string, 0, len(catalog.Types))
	for _, spec := range catalog.Types {
		ids = append(ids, spec.ID)
	}
	got := frontmatterKeyValues["type"]
	if strings.Join(got, ",") != strings.Join(ids, ",") {
		t.Errorf("frontmatterKeyValues[type] = %v, want the catalogue ids %v", got, ids)
	}
}

func TestEntityTypeCatalogVoiceAndAppearance(t *testing.T) {
	byID := map[string]EntityTypeSpec{}
	for _, spec := range buildEntityTypeCatalog().Types {
		byID[spec.ID] = spec
	}
	has := func(id, key string) bool {
		for _, candidate := range byID[id].Keys {
			if candidate.Name == key {
				return true
			}
		}
		return false
	}
	if !has("character", "voice") {
		t.Error("character must offer the voice key")
	}
	for _, id := range []string{"character", "location", "faction", "item"} {
		if !has(id, "appearance") {
			t.Errorf("%s must offer the appearance key", id)
		}
	}
	if has("location", "voice") {
		t.Error("location must not offer the voice key")
	}
}

func TestEntityTypeCatalogMarshals(t *testing.T) {
	encoded, err := json.Marshal(buildEntityTypeCatalog())
	if err != nil {
		t.Fatalf("marshal catalogue: %v", err)
	}
	var decoded EntityTypeCatalog
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal catalogue: %v", err)
	}
	if len(decoded.BaseKeys) == 0 || len(decoded.Types) == 0 {
		t.Fatal("the round-tripped catalogue is empty")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/gui/ -run TestEntityTypeCatalog -v`
Expected: FAIL, `undefined: buildEntityTypeCatalog` (compile error).

- [ ] **Step 3: Extract the shared key builder in `schema.go`**

In `pkg/gui/schema.go`, replace the body of `renderEntityFrontmatterSchema` so the reflection loop moves into a reusable `entityFrontmatterKeys`. Replace lines 170-201:

```go
// entityFrontmatterKeys reflects over the entity frontmatter so the keys the
// editor and the wizard offer cannot drift from the keys the loader accepts. The
// order is the struct's; callers that need a stable order sort it.
func entityFrontmatterKeys() []FrontmatterKeySchema {
	t := reflect.TypeOf(entity.EntityFrontmatter{})
	keys := make([]FrontmatterKeySchema, 0, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name := yamlFieldName(field)
		if name == "" || name == "-" {
			continue
		}
		keys = append(keys, FrontmatterKeySchema{
			Name:        name,
			Type:        frontmatterTypeLabel(field.Type),
			Required:    !strings.Contains(field.Tag.Get("yaml"), "omitempty"),
			Values:      frontmatterKeyValues[name],
			Description: frontmatterKeyDescriptions[name],
		})
	}
	return keys
}

// renderEntityFrontmatterSchema reflects over the entity frontmatter so the keys
// the editor offers cannot drift from the keys the loader accepts.
func renderEntityFrontmatterSchema() string {
	keys := entityFrontmatterKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].Name < keys[j].Name })

	encoded, err := json.MarshalIndent(FrontmatterSchema{AllowUnknown: true, Keys: keys}, "", "  ")
	if err != nil {
		return ""
	}
	return string(encoded) + "\n"
}
```

Then change the `type` entry of `frontmatterKeyValues` (line 167) so it is derived from the catalogue:

```go
// frontmatterKeyValues lists the values worth suggesting for the keys whose
// domain is small. They are suggestions, not a closed set: only the being-like
// types change engine behaviour, and any other type string is accepted. The type
// list is the entity type catalogue, so the editor and the wizard cannot disagree.
var frontmatterKeyValues = map[string][]string{
	"type": entityTypeIDs(),
}
```

- [ ] **Step 4: Create the catalogue**

Create `pkg/gui/entitycatalog.go`:

```go
package gui

import "context"

// EntityTypeSpec describes one offered entity type: how the picker presents it,
// the aliases the engine also recognises, and the frontmatter keys a new note of
// this type is scaffolded with.
type EntityTypeSpec struct {
	ID          string                 `json:"id"`
	Label       string                 `json:"label"`
	Description string                 `json:"description"`
	Aliases     []string               `json:"aliases,omitempty"`
	Keys        []FrontmatterKeySchema `json:"keys"`
}

// EntityTypeCatalog is the served description of the offered entity types. BaseKeys
// is what an unknown type falls back to, so a note created for a type the
// catalogue does not know still parses.
type EntityTypeCatalog struct {
	BaseKeys []FrontmatterKeySchema `json:"base_keys"`
	Types    []EntityTypeSpec       `json:"types"`
}

// entityTypeDef is one catalogue entry before its key names are resolved against
// the frontmatter schema.
type entityTypeDef struct {
	id          string
	label       string
	description string
	aliases     []string
	keys        []string
}

// entityKeyHead and entityKeyTail bracket the per-type extras, so a type's key
// order reads the same way in every note and no type can forget a base key.
var (
	entityKeyHead = []string{"id", "name", "type"}
	entityKeyTail = []string{"tags", "aliases", "location", "faction", "state"}
)

// entityKeys builds one type's ordered key list: the head, the type's own extras,
// then the tail.
func entityKeys(extras ...string) []string {
	keys := make([]string, 0, len(entityKeyHead)+len(extras)+len(entityKeyTail))
	keys = append(keys, entityKeyHead...)
	keys = append(keys, extras...)
	keys = append(keys, entityKeyTail...)
	return keys
}

// entityTypeDefs is the canonical list of offered entity types, in display order.
// npc, person and creature stay accepted by entity.IsCharacterType but are aliases
// of character rather than separate choices. appearance is offered for every type
// that can be depicted or referenced visually.
var entityTypeDefs = []entityTypeDef{
	{
		id:          "character",
		label:       "Character",
		description: "Player characters, NPCs, companions and adversaries.",
		aliases:     []string{"npc", "person", "creature"},
		keys:        entityKeys("appearance", "gender", "age", "voice"),
	},
	{
		id:          "location",
		label:       "Location",
		description: "Towns, rooms, regions and any other place a scene happens in.",
		keys:        entityKeys("appearance"),
	},
	{
		id:          "faction",
		label:       "Faction",
		description: "Guilds, orders, crews and governments the note belongs to or leads.",
		keys:        entityKeys("appearance"),
	},
	{
		id:          "item",
		label:       "Item",
		description: "Relics, weapons, tools and other things a character can carry.",
		keys:        entityKeys("appearance"),
	},
	{
		id:          "concept",
		label:       "Concept",
		description: "An idea, custom, deity or force the world is built around.",
		keys:        entityKeys(),
	},
	{
		id:          "arc",
		label:       "Narrative Arc",
		description: "A running storyline or threat, tracked as a progress clock.",
		keys:        entityKeys(),
	},
	{
		id:          "event",
		label:       "Event",
		description: "Something that happened, or is scheduled to happen.",
		keys:        entityKeys(),
	},
	{
		id:          "quest",
		label:       "Quest",
		description: "An objective the player can pursue, with a state to track it.",
		keys:        entityKeys(),
	},
	{
		id:          "lore",
		label:       "Lore",
		description: "Background history or a piece of world knowledge.",
		keys:        entityKeys(),
	},
}

// entityTypeIDs lists the catalogue's type ids in display order. It is the source
// for the frontmatter schema's type suggestions.
func entityTypeIDs() []string {
	ids := make([]string, 0, len(entityTypeDefs))
	for _, def := range entityTypeDefs {
		ids = append(ids, def.id)
	}
	return ids
}

// buildEntityTypeCatalog resolves each type's key names against the frontmatter
// schema, so a catalogue key and an editor completion describe a key identically.
func buildEntityTypeCatalog() EntityTypeCatalog {
	byName := map[string]FrontmatterKeySchema{}
	for _, key := range entityFrontmatterKeys() {
		byName[key.Name] = key
	}

	types := make([]EntityTypeSpec, 0, len(entityTypeDefs))
	for _, def := range entityTypeDefs {
		keys := make([]FrontmatterKeySchema, 0, len(def.keys))
		for _, name := range def.keys {
			keys = append(keys, byName[name])
		}
		types = append(types, EntityTypeSpec{
			ID:          def.id,
			Label:       def.label,
			Description: def.description,
			Aliases:     def.aliases,
			Keys:        keys,
		})
	}

	base := make([]FrontmatterKeySchema, 0, len(entityKeyHead)+len(entityKeyTail))
	for _, name := range entityKeys() {
		base = append(base, byName[name])
	}

	return EntityTypeCatalog{BaseKeys: base, Types: types}
}

// GetEntityTypeCatalog serves the offered entity types and the frontmatter keys
// each one scaffolds with.
func (s *Service) GetEntityTypeCatalog(_ context.Context) (EntityTypeCatalog, error) {
	return buildEntityTypeCatalog(), nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/gui/ -run TestEntityTypeCatalog -v`
Expected: PASS.

- [ ] **Step 6: Serve the route**

In `pkg/gui/server.go`, replace `handleSchemaRoutes` (lines 160-176) with:

```go
// handleSchemaRoutes serves the generated schemas the editor and the new-entity
// wizard consume.
func (s *Server) handleSchemaRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/api/schema/entity-frontmatter":
		schema, err := s.service.GetEntityFrontmatterSchema(r.Context())
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, schema)
	case "/api/schema/entity-types":
		catalog, err := s.service.GetEntityTypeCatalog(r.Context())
		if err != nil {
			writeGameError(w, err)
			return
		}
		writeJSON(w, catalog)
	default:
		http.NotFound(w, r)
	}
}
```

- [ ] **Step 7: Regenerate the embedded schema**

The `type` key's suggestion list changed from seven values to the catalogue's nine.

Run: `go test ./pkg/gui -update-docs`
Expected: no output; `pkg/gui/schema/entity-frontmatter.json` is rewritten.

- [ ] **Step 8: Run the whole package test suite**

Run: `go test -count=1 ./pkg/gui/`
Expected: PASS. `TestEntityFrontmatterSchemaIsCurrent`, `TestFrontendPathsResolveToMounts` and the route manifest tests must all still pass.

- [ ] **Step 9: Vet**

Run: `go vet ./...`
Expected: clean.

- [ ] **Step 10: Commit**

```bash
git add pkg/gui/entitycatalog.go pkg/gui/entitycatalog_test.go pkg/gui/schema.go pkg/gui/server.go pkg/gui/schema/entity-frontmatter.json
git commit -m "feat(gui): add a canonical entity type catalogue and endpoint"
```

---

### Task 2: Frontend catalogue types and API client

**Files:**
- Modify: `frontend/src/types.ts:1002` (after `FrontmatterSchema`)
- Modify: `frontend/src/api/client.ts:1-30` (type import) and `frontend/src/api/client.ts:766-770` (add the method)

**Interfaces:**
- Consumes: the `GET /api/schema/entity-types` response from Task 1.
- Produces: `interface EntityTypeSpec`, `interface EntityTypeCatalog`, `APIClient.prototype.getEntityTypes(): Promise<EntityTypeCatalog>`.

- [ ] **Step 1: Add the types**

In `frontend/src/types.ts`, immediately after the `FrontmatterSchema` interface (line 1002), add:

```ts
// EntityTypeSpec is one offered entity type, as the server describes it: how the
// picker labels it and the frontmatter keys a new note of this type is built with.
export interface EntityTypeSpec {
  id: string;
  label: string;
  description: string;
  aliases?: string[];
  keys: FrontmatterKeySchema[];
}

// EntityTypeCatalog is the served list of offered entity types. base_keys is what
// an unknown type falls back to.
export interface EntityTypeCatalog {
  base_keys: FrontmatterKeySchema[];
  types: EntityTypeSpec[];
}
```

- [ ] **Step 2: Add the client method**

In `frontend/src/api/client.ts`, add `EntityTypeCatalog` to the type import block (alphabetical position near `EntitySummary`), then add the method immediately after `getEntityFrontmatterSchema` (line 770):

```ts
  async getEntityTypes(): Promise<EntityTypeCatalog> {
    const res = await fetch('/api/schema/entity-types');
    if (!res.ok) throw new Error(`getEntityTypes: ${res.statusText}`);
    return res.json();
  }
```

- [ ] **Step 3: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS with no errors.

- [ ] **Step 4: Confirm the route scanner still passes**

Run: `go test ./pkg/gui/ -run TestFrontendPathsResolveToMounts -v`
Expected: PASS. `/api/schema/entity-types` is covered by the `/api/schema/` mount.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): describe the entity type catalogue in the API client"
```

---

### Task 3: The pure scaffold generator and its check

**Files:**
- Create: `frontend/src/lib/entityScaffold.ts`
- Create: `frontend/scripts/checkEntityScaffold.mjs`
- Modify: `frontend/package.json:6-13` (scripts)
- Modify: `mise.toml:52-60` (`test:frontend`)

**Interfaces:**
- Consumes: `EntityTypeCatalog`, `EntityTypeSpec`, `FrontmatterKeySchema` from Task 2.
- Produces: `interface VoiceSelection`, `interface ScaffoldInput`, `function buildEntityMarkdown(catalog: EntityTypeCatalog, input: ScaffoldInput): string`. Tasks 4-8 consume `buildEntityMarkdown`.

- [ ] **Step 1: Write the failing check**

Create `frontend/scripts/checkEntityScaffold.mjs`. It bundles the generator with esbuild and asserts on its output, in the style of `checkTreeModel.mjs`:

```js
// Checks the entity scaffold generator, which is pure TypeScript with no React, so
// it can be bundled and executed here rather than only exercised by hand.
//
// This exists because the generator decides what a brand new note looks like: which
// keys are present, what each one is documented as, and that the result parses. A
// regression here ships a note the loader cannot read.
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { parse as parseYaml } from 'yaml';

const here = fileURLToPath(new URL('..', import.meta.url));
const out = mkdtempSync(join(tmpdir(), 'localrpg-scaffold-'));

const key = (name, type, required, description) => ({ name, type, required, description });

const baseKeys = [
  key('id', 'string', true, 'The note id.'),
  key('name', 'string', true, 'The display name.'),
  key('type', 'string', true, 'What kind of thing the note is.'),
  key('tags', '[]string', false, 'Free-form labels.'),
  key('aliases', '[]string', false, 'Other names it is known by.'),
  key('location', 'string', false, 'Where the note is.'),
  key('faction', 'string', false, 'The group it belongs to.'),
  key('state', 'map<string, any>', false, 'Arbitrary per-note state.'),
];

const characterKeys = [
  key('id', 'string', true, 'The note id.'),
  key('name', 'string', true, 'The display name.'),
  key('type', 'string', true, 'What kind of thing the note is.'),
  key('appearance', 'string', false, 'A physical description.'),
  key('gender', 'string', false, 'The gender, as free text.'),
  key('age', 'string', false, 'The age, as free text.'),
  key('voice', 'VoiceConfig', false, 'The voice this note speaks with.'),
  key('tags', '[]string', false, 'Free-form labels.'),
  key('aliases', '[]string', false, 'Other names it is known by.'),
  key('location', 'string', false, 'Where the note is.'),
  key('faction', 'string', false, 'The group it belongs to.'),
  key('state', 'map<string, any>', false, 'Arbitrary per-note state.'),
];

const locationKeys = [
  key('id', 'string', true, 'The note id.'),
  key('name', 'string', true, 'The display name.'),
  key('type', 'string', true, 'What kind of thing the note is.'),
  key('appearance', 'string', false, 'A physical description.'),
  key('tags', '[]string', false, 'Free-form labels.'),
  key('aliases', '[]string', false, 'Other names it is known by.'),
  key('location', 'string', false, 'Where the note is.'),
  key('faction', 'string', false, 'The group it belongs to.'),
  key('state', 'map<string, any>', false, 'Arbitrary per-note state.'),
];

const catalog = {
  base_keys: baseKeys,
  types: [
    { id: 'character', label: 'Character', description: 'People.', keys: characterKeys },
    { id: 'location', label: 'Location', description: 'Places.', keys: locationKeys },
  ],
};

try {
  const bundle = join(out, 'entityScaffold.mjs');
  execFileSync(
    'npx',
    ['esbuild', 'src/lib/entityScaffold.ts', '--bundle', '--format=esm', `--outfile=${bundle}`, '--log-level=error'],
    { cwd: here, stdio: 'inherit' },
  );

  const { buildEntityMarkdown } = await import(pathToFileURL(bundle).href);

  let failures = 0;
  const check = (label, condition, detail = '') => {
    if (!condition) {
      console.error(`FAIL ${label}${detail ? `\n  ${detail}` : ''}`);
      failures++;
      return;
    }
    console.log(`ok   ${label}`);
  };

  const frontmatterOf = (markdown) => {
    const end = markdown.indexOf('\n---', 3);
    return markdown.slice(4, end);
  };

  const character = buildEntityMarkdown(catalog, { id: 'lady-evelyn', name: 'Lady Evelyn Vance', type: 'character' });
  const fm = frontmatterOf(character);

  check('required keys carry their values', fm.includes('id: "lady-evelyn"') && fm.includes('name: "Lady Evelyn Vance"') && fm.includes('type: "character"'));
  check('an empty list is emitted for tags', fm.includes('tags: []'));
  check('an empty map is emitted for state', fm.includes('state: {}'));
  check('an empty string is emitted for a free-text key', fm.includes('appearance: ""'));
  check('the body starts with the note name as an H1', character.trimEnd().endsWith('# Lady Evelyn Vance'));

  const lines = fm.split('\n');
  let undocumented = 0;
  lines.forEach((line, index) => {
    if (/^[a-z_]+:/.test(line) && !(lines[index - 1] ?? '').startsWith('# ')) undocumented++;
  });
  check('every key is preceded by its description comment', undocumented === 0, `${undocumented} undocumented key(s)`);

  const emptyVoice = buildEntityMarkdown(catalog, { id: 'a', name: 'A', type: 'character' });
  check('a character without a voice still gets the documented block', frontmatterOf(emptyVoice).includes('voice:\n  provider: ""'));

  const voiced = buildEntityMarkdown(catalog, {
    id: 'a',
    name: 'A',
    type: 'character',
    voice: { provider: 'elevenlabs', voice_id: '21m00', pitch: 1, speech_rate: 1.05 },
  });
  const voicedFm = frontmatterOf(voiced);
  check('a chosen voice is written into the block', voicedFm.includes('provider: "elevenlabs"') && voicedFm.includes('voice_id: "21m00"') && voicedFm.includes('speech_rate: 1.05'));

  const location = frontmatterOf(buildEntityMarkdown(catalog, { id: 'the-ashen-bastion', name: 'The Ashen Bastion', type: 'location' }));
  check('a location offers appearance', location.includes('appearance: ""'));
  check('a location does not offer voice, gender or age', !location.includes('voice:') && !location.includes('gender:') && !location.includes('age:'));

  const unknown = frontmatterOf(buildEntityMarkdown(catalog, { id: 'x', name: 'X', type: 'does-not-exist' }));
  check('an unknown type falls back to the base key set', unknown.includes('id: "x"') && !unknown.includes('voice:') && unknown.includes('state: {}'));

  const parsed = parseYaml(fm);
  check('the generated frontmatter parses as YAML', parsed && parsed.id === 'lady-evelyn' && parsed.type === 'character');

  if (failures > 0) {
    console.error(`\n${failures} entity scaffold check(s) failed`);
    process.exit(1);
  }
  console.log('\nentity scaffold is consistent');
} finally {
  rmSync(out, { recursive: true, force: true });
}
```

- [ ] **Step 2: Run the check to verify it fails**

Run: `cd frontend && npm run check:entity-scaffold`
Expected: FAIL, because the script is not registered and `src/lib/entityScaffold.ts` does not exist.

- [ ] **Step 3: Create the generator**

Create `frontend/src/lib/entityScaffold.ts`:

```ts
import type { EntityTypeCatalog, FrontmatterKeySchema } from '../types';

export interface VoiceSelection {
  provider: string;
  voice_id: string;
  pitch?: number;
  speech_rate?: number;
  options?: Record<string, unknown>;
}

export interface ScaffoldInput {
  id: string;
  name: string;
  type: string;
  voice?: VoiceSelection;
}

// yamlScalar quotes a value so a name containing a colon, hash or quote cannot
// break the block it is written into.
const yamlScalar = (value: string): string => JSON.stringify(value);

// voiceBlock renders the voice key. With no selection it still writes every
// sub-key empty, so the shape is discoverable.
function voiceBlock(voice?: VoiceSelection): string[] {
  const lines = [
    'voice:',
    `  provider: ${yamlScalar(voice?.provider ?? '')}`,
    `  voice_id: ${yamlScalar(voice?.voice_id ?? '')}`,
    `  pitch: ${voice?.pitch ?? 1}`,
    `  speech_rate: ${voice?.speech_rate ?? 1}`,
  ];
  if (voice?.options && Object.keys(voice.options).length > 0) {
    lines.push('  options:');
    for (const [key, value] of Object.entries(voice.options)) {
      lines.push(`    ${key}: ${JSON.stringify(value)}`);
    }
  }
  return lines;
}

function keyLines(key: FrontmatterKeySchema, input: ScaffoldInput): string[] {
  switch (key.name) {
    case 'id':
      return [`id: ${yamlScalar(input.id)}`];
    case 'name':
      return [`name: ${yamlScalar(input.name)}`];
    case 'type':
      return [`type: ${yamlScalar(input.type)}`];
    case 'voice':
      return voiceBlock(input.voice);
    case 'tags':
    case 'aliases':
      return [`${key.name}: []`];
    case 'state':
      return [`${key.name}: {}`];
    default:
      return [`${key.name}: ""`];
  }
}

// buildEntityMarkdown renders a new note: the frontmatter keys the chosen type
// offers, each preceded by the description the server supplies, then an H1 the
// user writes under. It is pure so it can be checked without a browser.
export function buildEntityMarkdown(catalog: EntityTypeCatalog, input: ScaffoldInput): string {
  const spec = catalog.types.find((candidate) => candidate.id === input.type);
  const keys = spec ? spec.keys : catalog.base_keys;

  const lines: string[] = ['---'];
  for (const key of keys) {
    lines.push(`# ${key.description}`);
    lines.push(...keyLines(key, input));
  }
  lines.push('---', '', `# ${input.name}`, '');
  return lines.join('\n');
}
```

- [ ] **Step 4: Register the script**

In `frontend/package.json`, add to `scripts`:

```json
    "check:entity-scaffold": "node scripts/checkEntityScaffold.mjs",
```

In `mise.toml`, add the check to the `test:frontend` task's `run` block (line 59):

```toml
run = """
npx tsc --noEmit
npm run check:tree-model
npm run check:player-bundle
npm run check:entity-scaffold
"""
```

- [ ] **Step 5: Run the check to verify it passes**

Run: `cd frontend && npm run check:entity-scaffold`
Expected: PASS, every `ok` line and `entity scaffold is consistent`.

- [ ] **Step 6: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/lib/entityScaffold.ts frontend/scripts/checkEntityScaffold.mjs frontend/package.json mise.toml
git commit -m "feat(frontend): generate entity frontmatter from the type catalogue"
```

---

### Task 4: The New Entity Wizard dialog

**Files:**
- Create: `frontend/src/components/NewEntityWizard.tsx`

**Interfaces:**
- Consumes: `buildEntityMarkdown`, `ScaffoldInput`, `VoiceSelection` (Task 3); `EntityTypeCatalog`, `EntityTypeSpec`, `TTSConfig` (`frontend/src/types.ts`); `APIClient.getEntityTypes`, `APIClient.inspectTTS` (Task 2 and existing); `useTTSInspect` (`frontend/src/hooks/useTTSInspect.ts`); `slugify` (`frontend/src/lib/slug.ts`); `useMountTransition` (`frontend/src/hooks/useMountTransition`).
- Produces: `NewEntityWizard` React component with the props below. Tasks 5-8 consume it.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/NewEntityWizard.tsx`:

```tsx
import React, { useEffect, useMemo, useState } from 'react';
import { Check, Sparkles, X } from 'lucide-react';
import type { EntityTypeCatalog, TTSConfig } from '../types';
import { APIClient } from '../api/client';
import { useMountTransition } from '../hooks/useMountTransition';
import { useTTSInspect } from '../hooks/useTTSInspect';
import { buildEntityMarkdown, type VoiceSelection } from '../lib/entityScaffold';
import { slugify } from '../lib/slug';

export interface NewEntityWizardProps {
  isOpen: boolean;
  initialName?: string;
  existingIds: string[];
  ttsConfig?: TTSConfig;
  confirmLabel?: string;
  onConfirm: (input: { id: string; name: string; markdown: string }) => void | Promise<void>;
  onOpenExisting?: (id: string) => void;
  onClose: () => void;
}

// NewEntityWizard scaffolds a note's frontmatter from the server's entity type
// catalogue and hands the markdown to its host. It does not save: the codex, the
// content studio and the world studio each write to a different endpoint.
export const NewEntityWizard: React.FC<NewEntityWizardProps> = ({
  isOpen,
  initialName,
  existingIds,
  ttsConfig,
  confirmLabel = 'Create note',
  onConfirm,
  onOpenExisting,
  onClose,
}) => {
  const { mounted, state } = useMountTransition(isOpen, 200);
  const [catalog, setCatalog] = useState<EntityTypeCatalog | null>(null);
  const [catalogError, setCatalogError] = useState('');
  const [resolvedTTS, setResolvedTTS] = useState<TTSConfig | null>(ttsConfig ?? null);
  const [name, setName] = useState(initialName ?? '');
  const [id, setId] = useState(initialName ? slugify(initialName) : '');
  const [idTouched, setIdTouched] = useState(false);
  const [type, setType] = useState('');
  const [voiceId, setVoiceId] = useState('');
  const [voiceQuery, setVoiceQuery] = useState('');
  const [saving, setSaving] = useState(false);

  const spec = catalog?.types.find((candidate) => candidate.id === type);
  const wantsVoice = spec?.keys.some((key) => key.name === 'voice') ?? false;
  const { inspect, loading: inspecting } = useTTSInspect(wantsVoice ? resolvedTTS : null, wantsVoice);

  useEffect(() => {
    if (!isOpen) return;
    setName(initialName ?? '');
    setId(initialName ? slugify(initialName) : '');
    setIdTouched(false);
    setType('');
    setVoiceId('');
    setVoiceQuery('');
    setCatalogError('');
    let cancelled = false;
    new APIClient('').getEntityTypes()
      .then((next) => {
        if (cancelled) return;
        setCatalog(next);
        setType((current) => current || next.types[0]?.id || '');
      })
      .catch((err) => {
        if (!cancelled) setCatalogError(err instanceof Error ? err.message : 'Could not load entity types');
      });
    return () => {
      cancelled = true;
    };
  }, [isOpen, initialName]);

  useEffect(() => {
    if (ttsConfig || resolvedTTS) return;
    new APIClient('').getSettings()
      .then((settings) => setResolvedTTS(settings.config.media.tts))
      .catch(() => setResolvedTTS(null));
  }, [ttsConfig, resolvedTTS]);

  const voices = useMemo(() => {
    const needle = voiceQuery.trim().toLowerCase();
    return (inspect?.catalog.voices ?? []).filter(
      (voice) => !needle || voice.name.toLowerCase().includes(needle) || voice.id.toLowerCase().includes(needle),
    );
  }, [inspect, voiceQuery]);

  const collision = id !== '' && existingIds.includes(id);
  const idValid = /^[a-z0-9][a-z0-9-]*$/.test(id);
  const canConfirm = !!catalog && !!spec && idValid && !collision && name.trim() !== '' && !saving;

  const selection: VoiceSelection | undefined = voiceId
    ? { provider: inspect?.provider_key ?? '', voice_id: voiceId }
    : undefined;

  const preview = catalog && spec
    ? buildEntityMarkdown(catalog, { id, name: name.trim() || 'Name', type: spec.id, voice: selection })
    : '';

  if (!mounted) return null;

  const confirm = async () => {
    if (!catalog || !spec || !canConfirm) return;
    setSaving(true);
    try {
      await onConfirm({
        id,
        name: name.trim(),
        markdown: buildEntityMarkdown(catalog, { id, name: name.trim(), type: spec.id, voice: selection }),
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div
      data-state={state}
      className={`fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm ${
        state === 'enter' ? 'anim-fade-in' : 'anim-fade-out pointer-events-none'
      }`}
      style={{ '--anim-dur': '200ms' } as React.CSSProperties}
    >
      <div
        className={`relative flex max-h-[85vh] w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-purple-500/40 bg-stone-900 shadow-2xl ${
          state === 'enter' ? 'anim-scale-in' : 'anim-scale-out'
        }`}
      >
        <div className="flex items-center justify-between border-b border-stone-800 px-6 py-4">
          <div className="flex items-center gap-2">
            <Sparkles className="h-5 w-5 text-purple-400" />
            <h3 className="font-sans text-base font-bold text-purple-400">New Note</h3>
          </div>
          <button onClick={onClose} className="cursor-pointer rounded-lg p-1 text-stone-400 hover:bg-stone-800 hover:text-white">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-6">
          {catalogError && (
            <p className="rounded border border-red-900/60 bg-red-950/40 p-2 text-xs text-red-200">{catalogError}</p>
          )}

          <div className="space-y-2">
            <label className="block text-xs uppercase tracking-wider text-stone-400">Name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                if (!idTouched) setId(slugify(e.target.value));
              }}
              placeholder="Lady Evelyn Vance"
              className="w-full rounded-xl border border-stone-800 bg-stone-950 px-3 py-2 text-xs text-stone-100 focus:border-purple-500/50 focus:outline-none"
            />
            <input
              type="text"
              value={id}
              onChange={(e) => {
                setIdTouched(true);
                setId(e.target.value);
              }}
              placeholder="lady-evelyn"
              className="w-full rounded-xl border border-stone-800 bg-stone-950 px-3 py-2 font-mono text-xs text-stone-300 focus:border-purple-500/50 focus:outline-none"
            />
            {collision && (
              <p className="text-xs text-amber-300">
                A note with this id already exists.{' '}
                {onOpenExisting && (
                  <button type="button" onClick={() => onOpenExisting(id)} className="underline">
                    Open it instead
                  </button>
                )}
              </p>
            )}
            {!collision && id !== '' && !idValid && (
              <p className="text-xs text-red-300">The id may only contain lowercase letters, digits and hyphens.</p>
            )}
          </div>

          <div className="space-y-2">
            <label className="block text-xs uppercase tracking-wider text-stone-400">Type</label>
            <div className="grid grid-cols-3 gap-1.5">
              {(catalog?.types ?? []).map((candidate) => (
                <button
                  key={candidate.id}
                  type="button"
                  onClick={() => setType(candidate.id)}
                  title={candidate.description}
                  className={`rounded-lg border px-2 py-1.5 text-center font-mono text-xs transition-colors cursor-pointer ${
                    type === candidate.id
                      ? 'border-purple-500 bg-purple-600/30 font-bold text-purple-200'
                      : 'border-stone-800 bg-black/40 text-stone-400 hover:text-stone-200'
                  }`}
                >
                  {candidate.label}
                </button>
              ))}
            </div>
            {spec && <p className="text-xs text-stone-500">{spec.description}</p>}
          </div>

          {wantsVoice && (
            <div className="space-y-2">
              <label className="block text-xs uppercase tracking-wider text-stone-400">Voice</label>
              {inspect?.error && <p className="text-xs text-red-300">{inspect.error}</p>}
              {!inspect?.catalog.available && !inspect?.error && (
                <p className="text-xs text-stone-500">
                  {inspecting ? 'Loading voices...' : 'No voice catalog is available; the voice block will be left empty.'}
                </p>
              )}
              {inspect?.catalog.available && (
                <>
                  <input
                    type="text"
                    value={voiceQuery}
                    onChange={(e) => setVoiceQuery(e.target.value)}
                    placeholder="Search voices..."
                    className="w-full rounded border border-stone-800 bg-stone-900 px-2 py-1 text-xs text-stone-200 focus:outline-none"
                  />
                  <div className="max-h-40 space-y-1 overflow-y-auto pr-1">
                    <button
                      type="button"
                      onClick={() => setVoiceId('')}
                      className={`w-full rounded border px-2 py-1 text-left text-xs ${
                        voiceId === '' ? 'border-purple-500 text-purple-200' : 'border-stone-800 text-stone-400'
                      }`}
                    >
                      None
                    </button>
                    {voices.map((voice) => (
                      <button
                        key={voice.id}
                        type="button"
                        onClick={() => setVoiceId(voice.id)}
                        className={`w-full rounded border px-2 py-1 text-left text-xs ${
                          voiceId === voice.id ? 'border-purple-500 text-purple-200' : 'border-stone-800 text-stone-300'
                        }`}
                      >
                        {voice.name}
                      </button>
                    ))}
                  </div>
                </>
              )}
            </div>
          )}

          <div className="space-y-2">
            <label className="block text-xs uppercase tracking-wider text-stone-400">Preview</label>
            <pre className="max-h-40 overflow-auto rounded-lg border border-stone-800 bg-black/40 p-2 font-mono text-[10px] leading-relaxed text-stone-400">
              {preview}
            </pre>
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-stone-800 px-6 py-3">
          <button type="button" onClick={onClose} className="cursor-pointer px-3 py-1.5 text-xs text-stone-400 hover:text-white">
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void confirm()}
            disabled={!canConfirm}
            className="flex cursor-pointer items-center gap-1.5 rounded-xl bg-purple-600 px-4 py-1.5 text-xs font-bold text-white disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Check className="h-3.5 w-3.5" />
            <span>{confirmLabel}</span>
          </button>
        </div>
      </div>
    </div>
  );
};
```

- [ ] **Step 2: Verify the client method names used exist**

Confirm `APIClient.getSettings` exists and returns a `SettingsResponse` whose `config.media.tts` is a `TTSConfig`. If the method is named differently, use the existing settings accessor rather than adding one.

Run: `grep -n "getSettings\|getEntityTypes" frontend/src/api/client.ts`
Expected: both methods present.

- [ ] **Step 3: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS. The component is not yet imported anywhere, so `noUnusedLocals` does not apply to it.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/NewEntityWizard.tsx
git commit -m "feat(frontend): add the new entity wizard dialog"
```

---

### Task 5: Wire the wizard into the codex

**Files:**
- Modify: `frontend/src/components/CodexDrawer.tsx` (sidebar header, empty state, imports, state)

**Interfaces:**
- Consumes: `NewEntityWizard` (Task 4); `entities` (already a prop); `ttsConfig` (already a prop); `onSelect` (already a prop); `client.saveEntity`.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Add the state and import**

In `frontend/src/components/CodexDrawer.tsx`, add `NewEntityWizard` to the imports (line 10 area) and add state next to `isMergeOpen` (line 52):

```tsx
import { NewEntityWizard } from './NewEntityWizard';
```

```tsx
  const [isNewNoteOpen, setIsNewNoteOpen] = useState(false);
```

- [ ] **Step 2: Add the sidebar header button**

In the notes sidebar header, immediately after the `Memories (...)` tab button closes (line 267, before the closing `</div>` of the tab row), add:

```tsx
              <button
                type="button"
                onClick={() => setIsNewNoteOpen(true)}
                className="ml-auto rounded-lg border border-purple-500/50 bg-purple-600/20 px-2 py-1 text-xs font-sans font-bold text-purple-200 transition-colors cursor-pointer hover:bg-purple-600/40"
              >
                + New
              </button>
```

- [ ] **Step 3: Add the empty-state button**

Replace the empty-state block (lines 368-372) with one that offers the wizard:

```tsx
        {!entity ? (
          <div className="h-full flex flex-col items-center justify-center text-center gap-3 text-stone-500 italic p-6">
            <BookOpen className="w-8 h-8 text-purple-500/40" />
            <span className="text-sm">Choose a note from the codex to read or edit it.</span>
            {gameID && (
              <button
                type="button"
                onClick={() => setIsNewNoteOpen(true)}
                className="not-italic rounded-xl bg-purple-600 px-3 py-1.5 text-xs font-sans font-bold text-white transition-colors cursor-pointer hover:bg-purple-500"
              >
                Create a note
              </button>
            )}
          </div>
        ) : (
```

- [ ] **Step 4: Render the wizard**

Immediately before the closing `</div>` of the component's outermost element, add:

```tsx
      {gameID && (
        <NewEntityWizard
          isOpen={isNewNoteOpen}
          existingIds={(entities ?? []).map((candidate) => candidate.id)}
          ttsConfig={ttsConfig}
          onClose={() => setIsNewNoteOpen(false)}
          onOpenExisting={(id) => {
            setIsNewNoteOpen(false);
            onSelect(id);
          }}
          onConfirm={async ({ id, markdown }) => {
            await new APIClient(gameID).saveEntity(id, markdown);
            invalidateEntityIndex();
            setLinkTargets(await loadEntityIndex(new APIClient(gameID), gameID, true));
            setIsNewNoteOpen(false);
            onSelect(id);
          }}
        />
      )}
```

- [ ] **Step 5: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 6: Build and smoke-test by hand**

Run: `mise run build` then `mise run dev:gui`, open the codex, click **+ New**, create a `character`, and confirm the note opens with the frontmatter and comments in the editor.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/CodexDrawer.tsx
git commit -m "feat(frontend): create notes from the codex with the wizard"
```

---

### Task 6: Wire the wizard into Content Studio

**Files:**
- Modify: `frontend/src/components/ContentStudio.tsx`

**Interfaces:**
- Consumes: `NewEntityWizard` (Task 4); `entities` (existing state); `client.saveEntity`; `loadEntityIndex`.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Add the import and state**

Add to the imports (line 6 area) and state (line 29 area):

```tsx
import { NewEntityWizard } from './NewEntityWizard';
```

```tsx
  const [isNewNoteOpen, setIsNewNoteOpen] = useState(false);
```

- [ ] **Step 2: Add the "+ New" button above the tree**

Replace the `<aside>` opening block (lines 143-144) so a button sits above the tree:

```tsx
        <aside className="w-64 shrink-0 border-r border-white/10 p-3">
          <button
            type="button"
            onClick={() => setIsNewNoteOpen(true)}
            className="mb-2 w-full rounded-lg border border-purple-500/50 bg-purple-600/20 px-2 py-1 text-xs font-sans font-bold text-purple-200 transition-colors cursor-pointer hover:bg-purple-600/40"
          >
            + New
          </button>
          <EntityTree
```

- [ ] **Step 3: Render the wizard**

Immediately before the closing `</div>` of the component's outermost element (line 192), add:

```tsx
      <NewEntityWizard
        isOpen={isNewNoteOpen}
        existingIds={entities.map((candidate) => candidate.id)}
        onClose={() => setIsNewNoteOpen(false)}
        onOpenExisting={async (id) => {
          setIsNewNoteOpen(false);
          setNote(await client.getEntity(id));
        }}
        onConfirm={async ({ id, markdown }) => {
          await client.saveEntity(id, markdown);
          invalidateEntityIndex();
          setEntities(await client.listEntities());
          setLinkTargets(await loadEntityIndex(client, gameID, true));
          setIsNewNoteOpen(false);
          setNote(await client.getEntity(id));
        }}
      />
```

- [ ] **Step 4: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Smoke-test by hand**

Run: `mise run build && mise run dev:gui`, open Content Studio, click **+ New**, create a `location`, and confirm it appears in the tree and loads into the editor.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/ContentStudio.tsx
git commit -m "feat(frontend): create notes from the content studio with the wizard"
```

---

### Task 7: Replace the Worlds Studio slug modal

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx` (state, `handleCreateNewEntity`, the modal JSX)

**Interfaces:**
- Consumes: `NewEntityWizard` (Task 4); `entities` (existing state, `WorldEntitySummary[]`); `APIClient.saveWorldEntity`; `handleSelectEntity`; `markDirty`; `isDraft`; `entityDrafts` state.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Read the current creation path**

Read `frontend/src/components/WorldsStudio.tsx:480-514` (`handleCreateNewEntity`) and `:1147-1187` (the modal). The draft branch stores markdown in `entityDrafts`; the saved branch calls `APIClient.saveWorldEntity` and reloads.

- [ ] **Step 2: Add the import and remove the slug state**

Add the import near line 10, and delete the `newEntitySlug` state (line 56):

```tsx
import { NewEntityWizard } from './NewEntityWizard';
```

- [ ] **Step 3: Rewrite `handleCreateNewEntity` to take the wizard's input**

Replace `handleCreateNewEntity` (lines 480-514) with:

```tsx
  const handleCreateNewEntity = async (input: { id: string; name: string; markdown: string }) => {
    const { id: slug, markdown } = input;

    if (isDraft) {
      if (entities.some((e) => e.id === slug)) {
        setToast({ type: 'error', message: `Entity "${slug}" already exists` });
        return;
      }
      const newSummary: WorldEntitySummary = { id: slug, name: input.name, type: typeFromMarkdown(markdown) };
      setEntities((prev) => [...prev, newSummary]);
      setEntityDrafts((prev) => ({
        ...prev,
        ...(selectedEntityID ? { [selectedEntityID]: entityMarkdown } : {}),
        [slug]: markdown,
      }));
      setSelectedEntityID(slug);
      setEntityMarkdown(markdown);
      setIsNewEntityModal(false);
      markDirty();
      return;
    }
    if (!savedID) return;

    try {
      await APIClient.saveWorldEntity(savedID, slug, markdown);
      setIsNewEntityModal(false);
      await loadWorldDetail(savedID);
      await handleSelectEntity(slug);
    } catch (err) {
      setToast({ type: 'error', message: errorMessage(err) || 'Failed to create entity' });
    }
  };
```

Add a small helper next to it that reads the type out of the scaffolded frontmatter:

```tsx
// typeFromMarkdown reads the type the wizard wrote, so the tree summary matches
// the note without a round trip.
function typeFromMarkdown(markdown: string): string {
  const match = markdown.match(/^type:\s*"?([a-z0-9-]+)"?/m);
  return match ? match[1] : 'concept';
}
```

- [ ] **Step 4: Replace the modal JSX**

Replace the "New Entity Modal" block (lines 1147-1187) with the wizard:

```tsx
      <NewEntityWizard
        isOpen={isNewEntityModal}
        existingIds={entities.map((e) => e.id)}
        confirmLabel={isDraft ? 'Add template' : 'Create template'}
        onClose={() => setIsNewEntityModal(false)}
        onConfirm={handleCreateNewEntity}
      />
```

- [ ] **Step 5: Remove the now-unused `STARTER_ENTITY_TEMPLATE` import if it is unused**

Run: `grep -n "STARTER_ENTITY_TEMPLATE" frontend/src/components/WorldsStudio.tsx`
If the only remaining match is the import, delete that import line. `noUnusedLocals` fails the build otherwise.

- [ ] **Step 6: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 7: Smoke-test by hand**

Run: `mise run build && mise run dev:gui`, open Worlds Studio, add a template in both draft and saved modes, and confirm the tree shows the new template and the editor holds the scaffold.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx
git commit -m "feat(frontend): scaffold world templates with the entity wizard"
```

---

### Task 8: Route the continuity-repair modal into the wizard

**Files:**
- Modify: `frontend/src/App.tsx` (`handleEditInCodexEntity`, the `AddEntityModal` render)

**Interfaces:**
- Consumes: `NewEntityWizard` (Task 4); `modalEntity`, `entities`, `client`, `refreshCorpus`, `setSelectedEntity`, `setActiveDrawer` (all existing in App).
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Read the current handlers**

Read `frontend/src/App.tsx:656-701` (`handleQuickCreateEntity`, `handleEditInCodexEntity`) and `:1137-1146` (the `AddEntityModal` render). The modal takes `entityName`, `onQuickCreate`, `onEditInCodex`, `onClose`.

- [ ] **Step 2: Add a wizard state and handler**

Add next to the `modalEntity` state:

```tsx
  const [wizardEntity, setWizardEntity] = useState<{ name: string; turnNumber: number } | null>(null);
```

Add a handler that saves the wizard's markdown and opens the codex:

```tsx
  const handleWizardCreateEntity = async ({ id, markdown }: { id: string; name: string; markdown: string }) => {
    if (!client || !wizardEntity) return;
    try {
      await client.saveEntity(id, markdown);
      await client.addressFinding(wizardEntity.turnNumber, 'continuity');
      setAddressed((prev) => new Set(prev).add(wizardEntity.turnNumber));
      refreshCorpus();
      setSelectedEntity(await client.getEntity(id));
      setActiveDrawer('codex');
    } catch (err) {
      console.error('wizard create entity failed:', err);
    } finally {
      setWizardEntity(null);
      setModalEntity(null);
    }
  };
```

- [ ] **Step 3: Point "Edit in Codex" at the wizard**

Replace `handleEditInCodexEntity` (lines 681-701) with a version that opens the wizard instead of writing a three-key note:

```tsx
  const handleEditInCodexEntity = (name: string) => {
    if (!modalEntity) return;
    setWizardEntity({ name, turnNumber: modalEntity.turnNumber });
    setModalEntity(null);
  };
```

Update the `AddEntityModal` render (line 1142) so the prop matches the new signature:

```tsx
          onEditInCodex={handleEditInCodexEntity}
```

`AddEntityModal`'s `onEditInCodex` prop keeps its `(name: string, type: string)` type; the `type` argument is ignored, so no change is needed in `AddEntityModal.tsx`.

- [ ] **Step 4: Render the wizard**

Immediately after the `AddEntityModal` render block (line 1146), add:

```tsx
      {wizardEntity && (
        <NewEntityWizard
          isOpen={wizardEntity !== null}
          initialName={wizardEntity.name}
          existingIds={entities.map((candidate) => candidate.id)}
          ttsConfig={config?.media.tts}
          onClose={() => setWizardEntity(null)}
          onOpenExisting={async (id) => {
            setWizardEntity(null);
            setSelectedEntity(await client!.getEntity(id));
            setActiveDrawer('codex');
          }}
          onConfirm={handleWizardCreateEntity}
        />
      )}
```

Add the import near line 17:

```tsx
import { NewEntityWizard } from './components/NewEntityWizard';
```

- [ ] **Step 5: Type-check**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 6: Smoke-test by hand**

Run: `mise run build && mise run dev:gui`, play a turn that mentions an unknown entity, click the continuity prompt's "Edit in Codex", and confirm the wizard opens pre-filled with the mention and lands in the codex on confirm.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/App.tsx
git commit -m "feat(frontend): open the entity wizard from continuity repair"
```

---

### Task 9: Document the wizard and the type list

**Files:**
- Modify: `pkg/gui/docs/10-codex-yaml.md:54-60`

**Interfaces:**
- Consumes: the catalogue's offered types from Task 1.
- Produces: nothing.

- [ ] **Step 1: Update the supported types list**

Replace the "Supported Entity Types" section (lines 54-60) so the list matches the catalogue:

```markdown
## Supported Entity Types

The New Note wizard offers these types. Each one scaffolds the frontmatter keys
it uses, every key preceded by a line explaining what it is for. The engine
itself accepts any type string, so a type not listed here still loads.

- **`character`**: Player characters, NPCs, companions, and adversaries. Offers `voice`, `appearance`, `gender`, and `age`.
- **`location`**: Towns, rooms, regions, and any other place a scene happens in. Offers `appearance`, and nests inside a parent location through `location: "[[parent-zone]]"`.
- **`faction`**: Guilds, orders, crews, and governments. Offers `appearance`; influence, reputation, and rivalries live in `state`.
- **`item`**: Relics, weapons, tools, and keys. Offers `appearance`.
- **`concept`**: Ideas, customs, deities, and forces the world is built around.
- **`arc`**: A running storyline or threat, tracked as a progress clock in `state`.
- **`event`**: Something that happened, or is scheduled to happen.
- **`quest`**: An objective the player can pursue, with a `state` to track it.
- **`lore`**: Background history or a piece of world knowledge.

## Creating a Note

The **+ New** button in the codex, the Content Studio, and the Worlds Studio opens
the same wizard. Give the note a name, pick its type, and answer the options the
type asks for, such as a voice for a character. The wizard writes the frontmatter
with every key for that type present and documented, and opens it in the editor,
where the autosuggester completes keys and known values as you type.
```

- [ ] **Step 2: Lint the docs**

Run: `mise run lint:docs`
Expected: PASS.

- [ ] **Step 3: Lint the prose**

Run: `mise run lint:prose`
Expected: 0 errors. The page is in Vale's scope; if a new word is reported as unknown, add it to `styles/config/vocabularies/LocalRPG/accept.txt` rather than rewording.

- [ ] **Step 4: Rebuild the showcase site to prove the page still renders**

Run: `mise run site:build`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/docs/10-codex-yaml.md
git commit -m "docs: describe the new note wizard and the entity types it offers"
```

---

### Task 10: Full verification

**Files:** none.

- [ ] **Step 1: Run the whole backend suite**

Run: `mise run test:backend`
Expected: PASS.

- [ ] **Step 2: Run the whole frontend suite**

Run: `mise run test:frontend`
Expected: PASS, including the new `check:entity-scaffold`.

- [ ] **Step 3: Lint**

Run: `mise run lint`
Expected: PASS.

- [ ] **Step 4: Build the app**

Run: `mise run build`
Expected: PASS, and `git status` shows no change under `pkg/gui/dist`.

---

## Self-Review

**Spec coverage**

| Spec section | Task |
| --- | --- |
| §3 catalogue, endpoint, list de-duplication | 1 |
| §3.3 response shape | 1 (adds `base_keys`, the mechanism for the fallback in §4.3) |
| §4 scaffold format and voice block | 3 |
| §4.3 pure generator | 3 |
| §5.1 component API | 4 |
| §5.2 steps | 4 |
| §5.3 TTS configuration | 4 |
| §5.4 codex | 5 |
| §5.4 Content Studio | 6 |
| §5.4 Worlds Studio | 7 |
| §5.4 continuity repair | 8 |
| §6 error handling | 4 (validation, collision, catalog unavailable, unknown type), 5-8 (save failure surfaces from the host's existing error path) |
| §7 testing | 1 (Go), 3 (node check) |
| §8 docs | 9 |

**Type consistency**

`EntityTypeCatalog` and `EntityTypeSpec` are the same names in Go (Task 1) and TypeScript (Task 2). `buildEntityTypeCatalog` (Go) and `buildEntityMarkdown` (TS) are the two generators, each referenced with the same name in later tasks. `NewEntityWizardProps.onConfirm` takes `{ id, name, markdown }` and every host (Tasks 5-8) destructures exactly those keys.

**Placeholder scan**

No "TBD", "add error handling", or "similar to Task N". Every code step shows the code.

**Known deviation from the spec text**

§3.3's JSON example lists only `types`. The plan adds a `base_keys` array to the same response, which is the concrete mechanism for §4.3's "falls back to the base key set". No other deviation.
