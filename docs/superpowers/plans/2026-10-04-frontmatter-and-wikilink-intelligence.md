# Frontmatter and Wikilink Intelligence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the editor know what a note is: complete frontmatter keys and values from a schema generated from the Go struct, lint broken YAML while typing, report a parse failure at the line it happened on, and complete `[[wikilinks]]` from the notes that exist.

**Architecture:** `entity.EntityFrontmatter` stays the single source of truth. The reflection walker that already generates the configuration reference moves out of the test and gains a second consumer, emitting a committed `pkg/gui/schema/entity-frontmatter.json` that a staleness test keeps honest and a route serves to the client. The client turns that schema into CodeMirror completion, hover and lint sources, and a second source completes wikilinks from a cached entity list.

**Tech Stack:** Go standard library (`reflect`, `encoding/json`, `gopkg.in/yaml.v3`), `@codemirror/autocomplete`, `@codemirror/lint`, `@codemirror/view`, the `yaml` npm package for client-side parsing, React 19, TypeScript.

**Spec:** `docs/superpowers/specs/2026-10-04-frontmatter-and-wikilink-intelligence-design.md`

**Depends on:** `docs/superpowers/plans/2026-10-04-content-editor-core.md` (the `MarkdownEditor` component and the language switch) and `docs/superpowers/plans/2026-10-04-content-organisation.md` (the `folder` field and `aliases` on `EntitySummary`).

## Global Constraints

- `entity.EntityFrontmatter` is the only place the accepted keys exist. The schema is generated, never hand-maintained, and a test fails when it drifts.
- An unknown frontmatter key is **info**, never an error. `EntityFrontmatter.ExtraMeta` is an inline catch-all and the engine is deliberately schema-agnostic.
- Save is never disabled by the client linter. The client and `gopkg.in/yaml.v3` will not agree on every edge case, and a false positive must not trap the author.
- The generated artifact is committed and embedded; regenerate with `go test ./pkg/gui -update-docs`. A new route needs `go test ./pkg/gui -update-routes` too.
- Go tests use the standard library only; `interface{}`, not `any`; `go vet ./...` clean. Errors wrapped with `fmt.Errorf("...: %w", err)`.
- Reuse `entity.Slugify`, `entity.WikilinkTarget` and `frontend/src/lib/slug.ts`; do not write local slug or link parsing.
- When adding an endpoint, update `Service`, `pkg/gui/server.go`, `pkg/gui/types.go`, `frontend/src/types.ts` and `frontend/src/api/client.ts` together.
- `tsconfig.json` sets `strict`, `noUnusedLocals`, `noUnusedParameters`.
- Commits are Conventional Commits with a scope; subject under 72 characters.

---

### File Map

- **`pkg/gui/schema.go`** (new) — the reflection helpers moved out of the test, the frontmatter schema generator, and the embedded artifact.
- **`pkg/gui/docs_schema_test.go`** — keeps the configuration reference generator; loses the shared helpers.
- **`pkg/gui/schema_test.go`** (new) — staleness, description coverage, required keys.
- **`pkg/gui/schema/entity-frontmatter.json`** (new, generated) — the committed artifact.
- **`pkg/gui/service.go`** — `GetEntityFrontmatterSchema`.
- **`pkg/gui/server.go`** — the schema route, the `routePattern` entry, the structured frontmatter error.
- **`pkg/gui/routes.go`** — the mount.
- **`pkg/gui/testdata/routes.json`** (generated) — regenerated.
- **`pkg/entity/entity.go`** — `FrontmatterError`, `Line`, `Column`.
- **`pkg/entity/entity_test.go`** — the offset and the line/column mapping.
- **`frontend/src/types.ts`** — the schema types and the save error shape.
- **`frontend/src/api/client.ts`** — `getEntityFrontmatterSchema`, and `saveEntity` surfacing line and column.
- **`frontend/src/components/editor/frontmatter.ts`** (new) — the frontmatter ranges, completion, hover and lint.
- **`frontend/src/components/editor/wikilink.ts`** (new) — wikilink completion and the smart insert form.
- **`frontend/src/components/editor/entityIndex.ts`** (new) — the cached entity list.
- **`frontend/src/components/editor/MarkdownEditor.tsx`** — the intelligence props and the server-error highlight.

---

### Task 1: Generate the frontmatter schema

**Files:**
- Create: `pkg/gui/schema.go`, `pkg/gui/schema_test.go`, `pkg/gui/schema/entity-frontmatter.json`
- Modify: `pkg/gui/docs_schema_test.go`

**Interfaces:**
- Produces: `type FrontmatterKeySchema struct{ Name, Type, Description string; Required bool; Values []string }`; `type FrontmatterSchema struct{ AllowUnknown bool; Keys []FrontmatterKeySchema }`; `func EntityFrontmatterSchema() FrontmatterSchema`; `func renderEntityFrontmatterSchema() string`

- [ ] **Step 1: Move the shared reflection helpers out of the test**

Create `pkg/gui/schema.go` with the helpers cut verbatim from `pkg/gui/docs_schema_test.go`, so both generators share one walker:

```go
package gui

import (
	"embed"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

//go:embed schema/entity-frontmatter.json
var entityFrontmatterSchemaJSON []byte

// schemaEntry is one key and its type, as the configuration reference lists them.
type schemaEntry struct {
	key string
	typ string
}

func walkSchema(prefix string, t reflect.Type) []schemaEntry {
	return walkSchemaInto(prefix, t, make(map[reflect.Type]bool))
}

func walkSchemaInto(prefix string, t reflect.Type, seen map[reflect.Type]bool) []schemaEntry {
	t = derefType(t)
	switch t.Kind() {
	case reflect.Struct:
		if seen[t] {
			return nil
		}
		seen[t] = true
		var out []schemaEntry
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := yamlFieldName(field)
			if name == "" || name == "-" {
				continue
			}
			out = append(out, walkSchemaInto(prefix+"."+name, field.Type, seen)...)
		}
		return out
	case reflect.Slice, reflect.Array:
		elem := derefType(t.Elem())
		if elem.Kind() == reflect.Struct {
			return walkSchemaInto(prefix+"[]", elem, seen)
		}
		return []schemaEntry{{key: prefix, typ: "[]" + typeLabel(elem)}}
	case reflect.Map:
		value := derefType(t.Elem())
		if value.Kind() == reflect.Struct {
			return walkSchemaInto(prefix+".<key>", value, seen)
		}
		if t.Key().Kind() == reflect.String {
			return []schemaEntry{{key: prefix, typ: fmt.Sprintf("map<string, %s>", typeLabel(value))}}
		}
		return []schemaEntry{{key: prefix, typ: "map"}}
	case reflect.Interface:
		return []schemaEntry{{key: prefix, typ: "any"}}
	default:
		return []schemaEntry{{key: prefix, typ: typeLabel(t)}}
	}
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func typeLabel(t reflect.Type) string {
	t = derefType(t)
	switch t.Kind() {
	case reflect.Interface:
		return "any"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Struct:
		return t.Name()
	default:
		return t.Kind().String()
	}
}

func yamlFieldName(field reflect.StructField) string {
	name := field.Tag.Get("yaml")
	if idx := strings.Index(name, ","); idx >= 0 {
		name = name[:idx]
	}
	return strings.TrimSpace(name)
}
```

Delete those same six declarations (`schemaEntry`, `walkSchema`, `walkSchemaInto`, `derefType`, `typeLabel`, `yamlFieldName`) from `pkg/gui/docs_schema_test.go`. Its remaining `renderConfigurationReference` keeps using them, now from the package. Run `go build ./pkg/gui/` and remove any import the compiler now reports as unused in that test file.

- [ ] **Step 2: Write the generator**

Append to `pkg/gui/schema.go`:

```go
// FrontmatterKeySchema describes one accepted frontmatter key.
type FrontmatterKeySchema struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Values      []string `json:"values,omitempty"`
	Description string   `json:"description"`
}

// FrontmatterSchema is the generated description of an entity note's frontmatter.
// AllowUnknown is true because EntityFrontmatter has an inline catch-all: the
// engine is schema-agnostic, so the schema describes the accepted keys rather than
// gating them.
type FrontmatterSchema struct {
	AllowUnknown bool                   `json:"allowUnknown"`
	Keys         []FrontmatterKeySchema `json:"keys"`
}

// EntityFrontmatterSchema returns the generated schema the editor completes from.
func EntityFrontmatterSchema() FrontmatterSchema {
	var schema FrontmatterSchema
	if err := json.Unmarshal(entityFrontmatterSchemaJSON, &schema); err != nil {
		// A schema that will not parse is not worth failing an editor over: the
		// document still edits, only without completion.
		return FrontmatterSchema{AllowUnknown: true}
	}
	return schema
}

// frontmatterKeyDescriptions documents each accepted key. A test asserts every
// field of EntityFrontmatter has an entry, so a new field cannot ship
// undocumented.
var frontmatterKeyDescriptions = map[string]string{
	"id":               "The note's stable identity. It is unique per collection and does not change when the note moves between folders.",
	"name":             "The display name shown in the codex and used when the engine matches a mention.",
	"type":             "What kind of thing the note is. Types that read as a being (character, npc, person, creature) are the only ones the engine treats specially.",
	"tags":             "Free-form labels for grouping and search.",
	"voice":            "The text-to-speech voice this note speaks with, including pitch and speech rate.",
	"portrait":         "The path of the note's portrait image, when it has one.",
	"portrait_version": "A counter bumped each time the portrait is regenerated, so the client can bust its image cache.",
	"portrait_history": "Previous portrait paths, kept so an earlier portrait can be restored.",
	"location":         "Where the note is. May contain a [[wikilink]] to a location note.",
	"appearance":       "A physical description used when generating art or describing the note.",
	"gender":           "The note's gender, as free text.",
	"age":              "The note's age, as free text, because a campaign may count in years, seasons or reigns.",
	"aliases":          "Other names the same thing is known by. They are matched when resolving mentions and when completing wikilinks.",
	"faction":          "The group the note belongs to. May contain a [[wikilink]] to a faction note.",
	"history":          "The turn numbers this note took part in. Written by the engine, not by hand.",
	"state":            "Arbitrary per-note state the mechanics hooks read and write. The engine does not interpret the keys.",
}

// frontmatterKeyValues lists the values worth suggesting for the keys whose
// domain is small. They are suggestions, not a closed set: only the being-like
// types change engine behaviour, and any other type string is accepted.
var frontmatterKeyValues = map[string][]string{
	"type": {"character", "location", "faction", "item", "event", "quest", "lore"},
}

// renderEntityFrontmatterSchema reflects over the entity frontmatter so the keys
// the editor offers cannot drift from the keys the loader accepts.
func renderEntityFrontmatterSchema() string {
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

	sort.Slice(keys, func(i, j int) bool { return keys[i].Name < keys[j].Name })

	encoded, err := json.MarshalIndent(FrontmatterSchema{AllowUnknown: true, Keys: keys}, "", "  ")
	if err != nil {
		return ""
	}
	return string(encoded) + "\n"
}

// frontmatterTypeLabel names a field's type for a reader: "[]string" for a list,
// "map<string, any>" for a free-form map, and the struct name for a nested one.
func frontmatterTypeLabel(t reflect.Type) string {
	base := derefType(t)
	switch base.Kind() {
	case reflect.Slice:
		return "[]" + typeLabel(base.Elem())
	case reflect.Map:
		return fmt.Sprintf("map<%s, %s>", typeLabel(base.Key()), typeLabel(base.Elem()))
	default:
		return typeLabel(base)
	}
}
```

`ExtraMeta` is skipped automatically: `yamlFieldName` returns `""` for the `,inline` tag.

- [ ] **Step 3: Create the artifact directory**

Run: `mkdir -p pkg/gui/schema`

The embedded file must exist before the package compiles, so the directory is created now and the file is written by the generator in Step 5. If you prefer to write it by hand first, paste the output of `renderEntityFrontmatterSchema()` into `pkg/gui/schema/entity-frontmatter.json`; Step 5's staleness test will tell you if it is wrong.

- [ ] **Step 4: Write the tests**

Create `pkg/gui/schema_test.go`:

```go
package gui

import (
	"encoding/json"
	"strings"
	"testing"
)

const entityFrontmatterSchemaPath = "schema/entity-frontmatter.json"

func TestEntityFrontmatterSchemaIsCurrent(t *testing.T) {
	assertGeneratedDoc(t, entityFrontmatterSchemaPath, renderEntityFrontmatterSchema())
}

func TestEveryFrontmatterKeyHasADescription(t *testing.T) {
	var schema FrontmatterSchema
	if err := json.Unmarshal([]byte(renderEntityFrontmatterSchema()), &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if len(schema.Keys) == 0 {
		t.Fatal("the schema lists no keys")
	}
	for _, key := range schema.Keys {
		if strings.TrimSpace(key.Description) == "" {
			t.Errorf("frontmatter key %q has no description; add one to frontmatterKeyDescriptions", key.Name)
		}
		if strings.TrimSpace(key.Type) == "" {
			t.Errorf("frontmatter key %q has no type", key.Name)
		}
	}
}

func TestFrontmatterSchemaMarksRequiredKeys(t *testing.T) {
	var schema FrontmatterSchema
	if err := json.Unmarshal([]byte(renderEntityFrontmatterSchema()), &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	required := map[string]bool{}
	for _, key := range schema.Keys {
		required[key.Name] = key.Required
	}
	for _, want := range []string{"id", "name", "type"} {
		if !required[want] {
			t.Errorf("%q must be required, because it has no omitempty tag", want)
		}
	}
	if required["tags"] {
		t.Error("tags is optional and must not be marked required")
	}
	if _, ok := required["extra"]; ok {
		t.Error("the inline catch-all must not appear as a key")
	}
}

func TestFrontmatterSchemaAllowsUnknownKeys(t *testing.T) {
	var schema FrontmatterSchema
	if err := json.Unmarshal([]byte(renderEntityFrontmatterSchema()), &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if !schema.AllowUnknown {
		t.Fatal("the engine carries unknown keys in ExtraMeta, so allowUnknown must be true")
	}
}

func TestEmbeddedFrontmatterSchemaParses(t *testing.T) {
	schema := EntityFrontmatterSchema()
	if len(schema.Keys) == 0 {
		t.Fatal("the embedded schema is empty; regenerate with go test ./pkg/gui -update-docs")
	}
	if !schema.AllowUnknown {
		t.Error("the embedded schema must allow unknown keys")
	}
}
```

- [ ] **Step 5: Generate the artifact and run the tests**

Run: `go test ./pkg/gui -run TestEntityFrontmatterSchemaIsCurrent -update-docs`
Then: `go test ./pkg/gui -run 'FrontmatterSchema|EntityFrontmatter' -v`
Expected: PASS. Confirm `pkg/gui/schema/entity-frontmatter.json` exists, lists `id`, `name`, `type` as required, and contains no `extra` key.

- [ ] **Step 6: Prove the staleness test bites**

Add a field to `entity.EntityFrontmatter`, for example `TestKey string \`yaml:"test_key,omitempty"\``, run `go test ./pkg/gui -run TestEntityFrontmatterSchemaIsCurrent`, and confirm it fails with the regenerate instruction. Revert the field and re-run to confirm it passes.

- [ ] **Step 7: Commit**

```bash
git add pkg/gui/schema.go pkg/gui/schema_test.go pkg/gui/schema/entity-frontmatter.json pkg/gui/docs_schema_test.go
git commit -m "feat(gui): generate the entity frontmatter schema from the struct"
```

---

### Task 2: Serve the schema

**Files:**
- Modify: `pkg/gui/service.go`, `pkg/gui/routes.go`, `pkg/gui/server.go`, `pkg/gui/testdata/routes.json`
- Modify: `frontend/src/types.ts`, `frontend/src/api/client.ts`

**Interfaces:**
- Consumes: `EntityFrontmatterSchema()` (Task 1)
- Produces: `GET /api/schema/entity-frontmatter`; `Service.GetEntityFrontmatterSchema`; `APIClient.getEntityFrontmatterSchema()`; `FrontmatterSchema` and `FrontmatterKeySchema` in `frontend/src/types.ts`

- [ ] **Step 1: Add the service method**

In `pkg/gui/service.go`:

```go
// GetEntityFrontmatterSchema serves the generated schema the editor completes
// frontmatter keys and values from.
func (s *Service) GetEntityFrontmatterSchema(_ context.Context) (FrontmatterSchema, error) {
	return EntityFrontmatterSchema(), nil
}
```

- [ ] **Step 2: Add the mount and the handler**

In `pkg/gui/routes.go`, add to `mounts`:

```go
	{"/api/schema/", "handleSchemaRoutes", (*Server).handleSchemaRoutes},
```

In `pkg/gui/server.go`, add the handler:

```go
// handleSchemaRoutes serves the generated schemas the editor consumes.
func (s *Server) handleSchemaRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/api/schema/entity-frontmatter" {
		http.NotFound(w, r)
		return
	}
	schema, err := s.service.GetEntityFrontmatterSchema(r.Context())
	if err != nil {
		writeGameError(w, err)
		return
	}
	writeJSON(w, schema)
}
```

- [ ] **Step 3: Name the route for tracing**

In `routePattern` (`pkg/gui/server.go:50`), add the path to the exact-match case list:

```go
		path == "/api/usage" || path == "/api/limits" ||
		path == "/api/schema/entity-frontmatter":
```

`TestRoutePatternNamesEveryMount` fails if this is missed.

- [ ] **Step 4: Regenerate the route manifest**

Run: `go test ./pkg/gui -update-routes && go test ./pkg/gui -run 'Route' -v`
Expected: PASS, and `pkg/gui/testdata/routes.json` now contains `/api/schema/`.

- [ ] **Step 5: Add the client types**

In `frontend/src/types.ts`:

```ts
export interface FrontmatterKeySchema {
  name: string;
  type: string;
  required: boolean;
  values?: string[];
  description: string;
}

export interface FrontmatterSchema {
  allowUnknown: boolean;
  keys: FrontmatterKeySchema[];
}

// SaveErrorBody is what the entity save route returns when the frontmatter will
// not parse, so the editor can point at the offending line.
export interface SaveErrorBody {
  error: string;
  line?: number;
  column?: number;
}
```

- [ ] **Step 6: Add the client method**

In `frontend/src/api/client.ts`, beside `listEntities`:

```ts
  async getEntityFrontmatterSchema(): Promise<FrontmatterSchema> {
    const res = await fetch('/api/schema/entity-frontmatter');
    if (!res.ok) throw new Error(`getEntityFrontmatterSchema: ${res.statusText}`);
    return res.json();
  }
```

Add `FrontmatterSchema` to the type import at the top of the file.

- [ ] **Step 7: Verify**

Run: `go test ./pkg/gui/ -v && cd frontend && npx tsc --noEmit`
Expected: PASS and clean. `TestFrontendPathsResolveToMounts` passes because `/api/schema/` is mounted.

- [ ] **Step 8: Commit**

```bash
git add pkg/gui/service.go pkg/gui/routes.go pkg/gui/server.go pkg/gui/testdata/routes.json frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(gui): serve the frontmatter schema to the editor"
```

---

### Task 3: A parse failure that names its line

**Files:**
- Modify: `pkg/entity/entity.go`, `pkg/entity/entity_test.go`, `pkg/gui/server.go`

**Interfaces:**
- Produces: `type FrontmatterError struct{ Offset int; Msg string }` implementing `error`; `func (e *FrontmatterError) Line(document string) int`; `func (e *FrontmatterError) Column(document string) int`; the entity save route answering `422` with `{error, line, column}`

- [ ] **Step 1: Write the failing test**

Append to `pkg/entity/entity_test.go`:

```go
func TestFrontmatterErrorNamesItsLine(t *testing.T) {
	// The second frontmatter line is invalid: a value with an unclosed quote.
	doc := "---\nid: silver-hand\nname: \"unclosed\ntype: faction\n---\n\nBody.\n"

	_, err := ParseMarkdownEntity([]byte(doc))
	if err == nil {
		t.Fatal("expected a parse failure")
	}

	var fe *FrontmatterError
	if !errors.As(err, &fe) {
		t.Fatalf("error is %T, want *FrontmatterError", err)
	}
	if fe.Offset <= 0 {
		t.Fatalf("Offset = %d, want a positive offset into the document", fe.Offset)
	}
	if line := fe.Line(doc); line < 3 || line > 4 {
		t.Errorf("Line = %d, want the broken line (3 or 4)", line)
	}
	if column := fe.Column(doc); column < 1 {
		t.Errorf("Column = %d, want a 1-based column", column)
	}
}

func TestFrontmatterErrorOffsetPointsIntoTheDocument(t *testing.T) {
	doc := "---\nid: silver-hand\nname: X\n\tbad: [unclosed\n---\n\nBody.\n"

	_, err := ParseMarkdownEntity([]byte(doc))
	if err == nil {
		t.Fatal("expected a parse failure")
	}

	var fe *FrontmatterError
	if !errors.As(err, &fe) {
		t.Fatalf("error is %T, want *FrontmatterError", err)
	}
	if fe.Offset >= len(doc) {
		t.Fatalf("Offset = %d, want it inside the %d-byte document", fe.Offset, len(doc))
	}
	// The offset must land on the frontmatter block, never on the "---" header.
	if fe.Offset < len("---\n") {
		t.Fatalf("Offset = %d, want it past the frontmatter header", fe.Offset)
	}
}
```

Add `"errors"` to that file's imports if it is not already there.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/entity/ -run TestFrontmatterError -v`
Expected: FAIL, `FrontmatterError` undefined.

- [ ] **Step 3: Implement the typed error**

In `pkg/entity/entity.go`, add:

```go
// FrontmatterError is a frontmatter parse failure carrying the byte offset of the
// offending position, so a caller can point at the line instead of quoting a yaml
// error at the author.
type FrontmatterError struct {
	Offset int
	Msg    string
}

func (e *FrontmatterError) Error() string {
	return "parse frontmatter: " + e.Msg
}

// Line reports the 1-based line of the failure within a document, so a caller can
// place a diagnostic without knowing how the frontmatter is delimited.
func (e *FrontmatterError) Line(document string) int {
	if e.Offset <= 0 {
		return 1
	}
	if e.Offset > len(document) {
		return strings.Count(document, "\n") + 1
	}
	return strings.Count(document[:e.Offset], "\n") + 1
}

// Column reports the 1-based column of the failure within its line.
func (e *FrontmatterError) Column(document string) int {
	if e.Offset <= 0 || e.Offset > len(document) {
		return 1
	}
	lastNewline := strings.LastIndexByte(document[:e.Offset], '\n')
	return e.Offset - lastNewline
}

// frontmatterError converts a yaml failure into one carrying a byte offset in the
// document. gopkg.in/yaml.v3 reports a line number but not a position, so the line
// is located inside the raw block and the block's own header offset is added.
func frontmatterError(frontmatterRaw string, err error) *FrontmatterError {
	line := yamlErrorLine(err.Error())
	offset := len("---\n") + lineOffset(frontmatterRaw, line)
	return &FrontmatterError{Offset: offset, Msg: err.Error()}
}

// lineOffset returns the byte offset of a 1-based line within text.
func lineOffset(text string, line int) int {
	if line <= 1 {
		return 0
	}
	offset := 0
	for i := 1; i < line; i++ {
		next := strings.IndexByte(text[offset:], '\n')
		if next < 0 {
			return len(text)
		}
		offset += next + 1
	}
	return offset
}

// yamlErrorLine extracts the 1-based line from a gopkg.in/yaml.v3 error, which
// reads "yaml: line 3: ...".
func yamlErrorLine(message string) int {
	const marker = "line "
	idx := strings.Index(message, marker)
	if idx < 0 {
		return 1
	}
	rest := message[idx+len(marker):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 1
	}
	line, err := strconv.Atoi(rest[:end])
	if err != nil || line < 1 {
		return 1
	}
	return line
}
```

Add `"strconv"` to the imports.

In `ParseMarkdownEntity`, replace the yaml failure branch:

```go
	var fm EntityFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatterRaw), &fm); err != nil {
		return nil, frontmatterError(frontmatterRaw, err)
	}
```

- [ ] **Step 4: Return the position from the save route**

In `pkg/gui/server.go`, add beside `writeGameError`:

```go
// writeSaveError answers a failed save. A frontmatter failure carries the line and
// column it happened on, so the editor can highlight it rather than showing a bare
// status; anything else falls back to the shared error mapping.
func writeSaveError(w http.ResponseWriter, err error, document string) {
	var fe *entity.FrontmatterError
	if !errors.As(err, &fe) {
		writeGameError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":  fe.Msg,
		"line":   fe.Line(document),
		"column": fe.Column(document),
	})
}
```

Use it in the entity `PUT` handler:

```go
			if err := s.service.SaveEntityInFolder(r.Context(), gameID, entityID, body.Folder, body.Markdown); err != nil {
				writeSaveError(w, err, body.Markdown)
				return
			}
```

Add the `entity` package import to `server.go` if it is not already there.

- [ ] **Step 5: Run the tests**

Run: `go test ./pkg/entity/ ./pkg/gui/ -v && go vet ./...`
Expected: PASS, vet clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/entity/entity.go pkg/entity/entity_test.go pkg/gui/server.go
git commit -m "feat(entity): report where frontmatter parsing failed"
```

---

### Task 4: Frontmatter key completion

**Files:**
- Create: `frontend/src/components/editor/frontmatter.ts`

**Interfaces:**
- Consumes: `FrontmatterSchema` (Task 2)
- Produces: `frontmatterRange(doc)`, `frontmatterKeysPresent(doc, range)`, `isKeyPosition(doc, pos, range)`, `frontmatterKeyCompletion(schema): CompletionSource`

- [ ] **Step 1: Write the range helpers and the source**

Create `frontend/src/components/editor/frontmatter.ts`:

```ts
import type { CompletionSource } from '@codemirror/autocomplete';
import type { FrontmatterSchema } from '../../types';

export interface DocRange {
  from: number;
  to: number;
}

// frontmatterRange returns the span of the YAML block a note starts with, or null
// when the document has no closed frontmatter. It is pure, so the block boundary
// rules are reviewable without a browser.
export function frontmatterRange(doc: string): DocRange | null {
  if (!doc.startsWith('---\n')) return null;
  const end = doc.indexOf('\n---\n', 4);
  if (end === -1) return null;
  return { from: 4, to: end + 1 };
}

// frontmatterKeysPresent lists the top-level keys the block already sets, so
// completion never offers a key that is already there.
export function frontmatterKeysPresent(doc: string, range: DocRange): Set<string> {
  const keys = new Set<string>();
  for (const line of doc.slice(range.from, range.to).split('\n')) {
    const match = /^([A-Za-z_][A-Za-z0-9_-]*)\s*:/.exec(line);
    if (match) keys.add(match[1]);
  }
  return keys;
}

// isKeyPosition reports whether the cursor sits where a key may be typed: inside
// the block, before any colon on its line, and not in a list item or a comment.
export function isKeyPosition(doc: string, pos: number, range: DocRange): boolean {
  if (pos < range.from || pos > range.to) return false;
  const lineStart = doc.lastIndexOf('\n', pos - 1) + 1;
  const before = doc.slice(lineStart, pos);
  if (before.includes(':')) return false;
  return !/^\s*[-#]/.test(before);
}

// frontmatterKeyCompletion offers the accepted keys at a key position, with the
// type and the description the schema carries.
export function frontmatterKeyCompletion(schema: FrontmatterSchema): CompletionSource {
  return (context) => {
    const doc = context.state.doc.toString();
    const range = frontmatterRange(doc);
    if (!range) return null;
    if (!isKeyPosition(doc, context.pos, range)) return null;

    const word = context.matchBefore(/[A-Za-z_][A-Za-z0-9_-]*/);
    if (!word && !context.explicit) return null;
    const from = word ? word.from : context.pos;

    const present = frontmatterKeysPresent(doc, range);
    const options = schema.keys
      .filter((key) => !present.has(key.name))
      .map((key) => ({
        label: key.name,
        type: 'property',
        detail: key.required ? `${key.type} (required)` : key.type,
        info: key.description,
        apply: `${key.name}: `,
      }));

    if (options.length === 0) return null;
    return { from, options, validFor: /^[A-Za-z_][A-Za-z0-9_-]*$/ };
  };
}
```

- [ ] **Step 2: Verify the type gate**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/editor/frontmatter.ts
git commit -m "feat(frontend): complete frontmatter keys from the schema"
```

---

### Task 5: Value completion, hover and lint

**Files:**
- Modify: `frontend/src/components/editor/frontmatter.ts`, `frontend/package.json`
- Create: `frontend/src/components/editor/frontmatterLint.ts`

**Interfaces:**
- Consumes: `frontmatterRange`, `frontmatterKeysPresent`, `isKeyPosition` (Task 4)
- Produces: `frontmatterValueCompletion(schema): CompletionSource`; `frontmatterHover(schema): Extension`; `frontmatterLinter(schema): Extension`; `frontmatterDiagnostics(schema, doc, range): Diagnostic[]`

- [ ] **Step 1: Add the YAML parser**

Run: `cd frontend && npm install yaml`

The `yaml` package is the client-side parser the linter needs. It is bundled, so the offline guarantee holds.

- [ ] **Step 2: Write the pure diagnostics function**

Create `frontend/src/components/editor/frontmatterLint.ts`:

```ts
import { parse as parseYaml } from 'yaml';
import type { Diagnostic } from '@codemirror/lint';
import type { FrontmatterSchema } from '../../types';
import { frontmatterKeysPresent, type DocRange } from './frontmatter';

// lineStartOffset returns the byte offset of a 1-based line within text.
export function lineStartOffset(text: string, line: number): number {
  if (line <= 1) return 0;
  let offset = 0;
  for (let i = 1; i < line; i++) {
    const next = text.indexOf('\n', offset);
    if (next === -1) return text.length;
    offset = next + 1;
  }
  return offset;
}

// keyLineOffset finds the offset of a top-level key's line, so a diagnostic about
// that key can underline it rather than the whole block.
export function keyLineOffset(block: string, key: string): number {
  const pattern = new RegExp(`^${key}\\s*:`, 'm');
  const match = pattern.exec(block);
  return match ? match.index : 0;
}

// frontmatterDiagnostics is pure: it takes the document and returns the problems
// the linter reports, so the rules are reviewable without an editor instance.
export function frontmatterDiagnostics(
  schema: FrontmatterSchema,
  doc: string,
  range: DocRange,
): Diagnostic[] {
  const block = doc.slice(range.from, range.to);
  const diagnostics: Diagnostic[] = [];

  let parsed: unknown;
  try {
    parsed = parseYaml(block);
  } catch (err) {
    const error = err as Error & { linePos?: { line: number }[] };
    const line = error.linePos?.[0]?.line ?? 1;
    const from = range.from + lineStartOffset(block, line);
    diagnostics.push({
      from,
      to: Math.min(from + 1, range.to),
      severity: 'error',
      message: `Invalid YAML: ${error.message}`,
    });
    // A block that will not parse has no trustworthy keys to check.
    return diagnostics;
  }

  if (parsed === null || typeof parsed !== 'object') {
    return diagnostics;
  }

  const known = new Set(schema.keys.map((key) => key.name));
  const present = frontmatterKeysPresent(doc, range);

  for (const key of Object.keys(parsed as Record<string, unknown>)) {
    if (known.has(key)) continue;
    const from = range.from + keyLineOffset(block, key);
    diagnostics.push({
      from,
      to: Math.min(from + key.length, range.to),
      severity: 'info',
      // The engine is schema-agnostic: an unknown key is carried in ExtraMeta, so
      // this is information, never a reason to refuse a save.
      message: `"${key}" is not a known key. The engine keeps it as extra metadata.`,
    });
  }

  // A known key listed twice is a real mistake: the second wins silently.
  const seen = new Set<string>();
  for (const line of block.split('\n')) {
    const match = /^([A-Za-z_][A-Za-z0-9_-]*)\s*:/.exec(line);
    if (!match) continue;
    const key = match[1];
    if (seen.has(key)) {
      const from = range.from + keyLineOffset(block, key);
      diagnostics.push({
        from,
        to: Math.min(from + key.length, range.to),
        severity: 'error',
        message: `"${key}" is set more than once; only the last value survives.`,
      });
    }
    seen.add(key);
  }

  void present;
  return diagnostics;
}
```

Remove the `void present;` line and the `present` variable if you do not use it; `noUnusedLocals` fails the build otherwise. The duplicate-key check above does its own scan, so delete the `frontmatterKeysPresent` import and call if they end up unused.

- [ ] **Step 3: Wire the diagnostics into a linter and add value completion and hover**

Append to `frontend/src/components/editor/frontmatter.ts`:

```ts
import { linter } from '@codemirror/lint';
import type { Extension } from '@codemirror/state';
import { hoverTooltip } from '@codemirror/view';
import { frontmatterDiagnostics } from './frontmatterLint';

// frontmatterLinter reports broken YAML, duplicate keys and unknown keys. It is an
// early warning: the save path stays enabled, because the client parser and the
// Go parser will not agree on every edge case.
export function frontmatterLinter(schema: FrontmatterSchema): Extension {
  return linter((view) => {
    const doc = view.state.doc.toString();
    const range = frontmatterRange(doc);
    if (!range) return [];
    return frontmatterDiagnostics(schema, doc, range);
  });
}

// frontmatterValueCompletion suggests the values a key's domain has, and hands
// [[wikilink]] completion to the link source for the keys that hold links.
export function frontmatterValueCompletion(schema: FrontmatterSchema): CompletionSource {
  return (context) => {
    const doc = context.state.doc.toString();
    const range = frontmatterRange(doc);
    if (!range || context.pos < range.from || context.pos > range.to) return null;

    const lineStart = doc.lastIndexOf('\n', context.pos - 1) + 1;
    const before = doc.slice(lineStart, context.pos);
    const keyMatch = /^([A-Za-z_][A-Za-z0-9_-]*)\s*:\s*/.exec(before);
    if (!keyMatch) return null;

    // A link-bearing field is the wikilink source's job, not this one's.
    if (/\[\[[^[\]]*$/.test(before)) return null;

    const key = schema.keys.find((candidate) => candidate.name === keyMatch[1]);
    if (!key?.values || key.values.length === 0) return null;

    const word = context.matchBefore(/[A-Za-z0-9_-]*/);
    if (!word && !context.explicit) return null;

    return {
      from: word ? word.from : context.pos,
      options: key.values.map((value) => ({
        label: value,
        type: 'enum',
        detail: key.name,
        info: key.description,
      })),
      validFor: /^[A-Za-z0-9_-]*$/,
    };
  };
}

// frontmatterHover explains a key: what it holds and what the engine does with it.
export function frontmatterHover(schema: FrontmatterSchema): Extension {
  return hoverTooltip((view, pos) => {
    const doc = view.state.doc.toString();
    const range = frontmatterRange(doc);
    if (!range || pos < range.from || pos > range.to) return null;

    const lineStart = doc.lastIndexOf('\n', pos - 1) + 1;
    const lineEndIdx = doc.indexOf('\n', lineStart);
    const line = doc.slice(lineStart, lineEndIdx === -1 ? doc.length : lineEndIdx);
    const match = /^([A-Za-z_][A-Za-z0-9_-]*)\s*:/.exec(line);
    if (!match) return null;

    const key = schema.keys.find((candidate) => candidate.name === match[1]);
    if (!key) return null;

    return {
      pos: lineStart,
      end: lineStart + match[1].length,
      create: () => {
        const dom = document.createElement('div');
        dom.className = 'px-3 py-2 text-xs max-w-xs';
        const title = document.createElement('div');
        title.className = 'font-mono text-purple-300';
        title.textContent = `${key.name}: ${key.type}${key.required ? ' (required)' : ''}`;
        const body = document.createElement('div');
        body.className = 'mt-1 text-stone-300';
        body.textContent = key.description;
        dom.append(title, body);
        return { dom };
      },
    };
  });
}
```

- [ ] **Step 4: Verify the type gate**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean. `noUnusedLocals` will flag any leftover helper, so delete unused ones rather than silencing them.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/editor/frontmatter.ts frontend/src/components/editor/frontmatterLint.ts frontend/package.json frontend/package-lock.json
git commit -m "feat(frontend): lint frontmatter and complete its values"
```

---

### Task 6: Wikilink completion

**Files:**
- Create: `frontend/src/components/editor/wikilink.ts`

**Interfaces:**
- Consumes: `EntitySummary` (organisation plan), `slugify` (`frontend/src/lib/slug.ts`)
- Produces: `smartWikilink(id, name): string`; `wikilinkCompletion(entities): CompletionSource`

- [ ] **Step 1: Write the insert form and the source**

Create `frontend/src/components/editor/wikilink.ts`:

```ts
import type { CompletionSource } from '@codemirror/autocomplete';
import type { EntitySummary } from '../../types';
import { slugify } from '../../lib/slug';

// smartWikilink inserts the bare id when the display name already slugifies to it,
// and adds a label only when it carries information the id does not. The engine
// unwraps [[id|label]] and preserves the label when it rewrites a link.
export function smartWikilink(id: string, name: string): string {
  return slugify(name) === id ? `[[${id}]]` : `[[${id}|${name}]]`;
}

// scoreEntity ranks a note against a query. A name hit beats an id hit, an alias
// beats a tag, and an empty query matches everything so [[ alone lists the codex.
export function scoreEntity(entity: EntitySummary, query: string): number {
  if (query === '') return 1;
  const name = entity.name.toLowerCase();
  const id = entity.id.toLowerCase();
  if (name.startsWith(query)) return 100;
  if (name.includes(query)) return 80;
  if (id.includes(query)) return 60;
  if ((entity.aliases ?? []).some((alias) => alias.toLowerCase().includes(query))) return 40;
  if ((entity.tags ?? []).some((tag) => tag.toLowerCase().includes(query))) return 20;
  return 0;
}

// wikilinkCompletion offers the notes that exist, so an author never has to know
// an id in advance. It fires in the body and inside the frontmatter's link-bearing
// values, because both hold links.
export function wikilinkCompletion(entities: EntitySummary[]): CompletionSource {
  return (context) => {
    const match = context.matchBefore(/\[\[[^[\]]*$/);
    if (!match) return null;

    const query = match.text.slice(2).trim().toLowerCase();
    const options = entities
      .map((entity) => ({ entity, score: scoreEntity(entity, query) }))
      .filter((entry) => entry.score > 0)
      .sort((a, b) => b.score - a.score || a.entity.name.localeCompare(b.entity.name))
      .slice(0, 50)
      .map(({ entity }) => ({
        label: entity.name,
        type: 'text',
        detail: entity.folder ? `${entity.folder}/${entity.id}` : entity.id,
        apply: smartWikilink(entity.id, entity.name),
      }));

    if (options.length === 0) return null;
    return { from: match.from, options, validFor: /^\[\[[^[\]]*$/ };
  };
}
```

`apply` replaces from the `[[`, so the inserted text includes the brackets. That is why `smartWikilink` returns them.

- [ ] **Step 2: Verify the type gate**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/editor/wikilink.ts
git commit -m "feat(frontend): complete wikilinks from the notes that exist"
```

---

### Task 7: Cache the entity index

**Files:**
- Create: `frontend/src/components/editor/entityIndex.ts`

**Interfaces:**
- Consumes: `APIClient.listEntities` (existing)
- Produces: `loadEntityIndex(client, gameID, force?): Promise<EntitySummary[]>`; `invalidateEntityIndex(): void`

- [ ] **Step 1: Write the cache**

Create `frontend/src/components/editor/entityIndex.ts`:

```ts
import type { APIClient } from '../../api/client';
import type { EntitySummary } from '../../types';

interface CachedIndex {
  gameID: string;
  entities: EntitySummary[];
  loadedAt: number;
}

let cache: CachedIndex | null = null;

// The index is refreshed on a timer rather than per keystroke: a campaign holds
// tens to low hundreds of notes, so one fetch filters locally and completion stays
// instant and offline.
const maxAgeMs = 30_000;

export async function loadEntityIndex(
  client: APIClient,
  gameID: string,
  force = false,
): Promise<EntitySummary[]> {
  const fresh = cache && cache.gameID === gameID && Date.now() - cache.loadedAt < maxAgeMs;
  if (fresh && !force) return cache!.entities;

  try {
    const entities = await client.listEntities();
    cache = { gameID, entities, loadedAt: Date.now() };
    return entities;
  } catch {
    // A failed refresh keeps the last good list rather than emptying completion.
    return cache?.entities ?? [];
  }
}

// invalidateEntityIndex drops the cache after a write, so a note saved in this
// session is immediately linkable.
export function invalidateEntityIndex(): void {
  cache = null;
}
```

- [ ] **Step 2: Verify the type gate**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/editor/entityIndex.ts
git commit -m "feat(frontend): cache the entity index for link completion"
```

---

### Task 8: Wire the intelligence into the editor

**Files:**
- Modify: `frontend/src/components/editor/MarkdownEditor.tsx`, `frontend/src/api/client.ts`, `frontend/src/components/CodexDrawer.tsx`, `frontend/src/components/ContentStudio.tsx`

**Interfaces:**
- Consumes: everything above
- Produces: `MarkdownEditor` accepts `frontmatterSchema?`, `linkTargets?`, `serverError?`; a save failure underlines the reported line

- [ ] **Step 1: Add the props and the extensions**

In `frontend/src/components/editor/MarkdownEditor.tsx`, extend the props:

```tsx
export interface MarkdownEditorProps {
  value: string;
  onChange: (next: string) => void;
  language: EditorLanguage;
  onSave?: () => void;
  readOnly?: boolean;
  placeholder?: string;
  minHeight?: string;
  ariaLabel: string;
  frontmatterSchema?: FrontmatterSchema;
  linkTargets?: EntitySummary[];
  serverError?: { line: number; message: string } | null;
}
```

Import the sources and add a field for the server-reported diagnostic:

```tsx
import { StateEffect, StateField, type Extension } from '@codemirror/state';
import { autocompletion, type CompletionSource } from '@codemirror/autocomplete';
import type { Diagnostic } from '@codemirror/lint';
import type { EntitySummary, FrontmatterSchema } from '../../types';
import { frontmatterKeyCompletion, frontmatterValueCompletion, frontmatterHover, frontmatterLinter } from './frontmatter';
import { wikilinkCompletion } from './wikilink';

// setServerDiagnostic carries the parse failure the server reported back into the
// editor, so a rejected save underlines the line it named.
const setServerDiagnostic = StateEffect.define<Diagnostic | null>();

const serverDiagnosticField = StateField.define<Diagnostic | null>({
  create: () => null,
  update(value, tr) {
    for (const effect of tr.effects) {
      if (effect.is(setServerDiagnostic)) return effect.value;
    }
    return value;
  },
});
```

Build the intelligence extensions inside the effect, before `EditorState.create`:

```tsx
    const intelligence: Extension[] = [serverDiagnosticField];
    if (frontmatterSchema) {
      const sources: CompletionSource[] = [
        frontmatterKeyCompletion(frontmatterSchema),
        frontmatterValueCompletion(frontmatterSchema),
      ];
      if (linkTargets && linkTargets.length > 0) {
        sources.push(wikilinkCompletion(linkTargets));
      } else {
        sources.push(wikilinkCompletion([]));
      }
      intelligence.push(
        autocompletion({ override: sources }),
        frontmatterLinter(frontmatterSchema),
        frontmatterHover(frontmatterSchema),
      );
    } else if (linkTargets && linkTargets.length > 0) {
      intelligence.push(autocompletion({ override: [wikilinkCompletion(linkTargets)] }));
    }
```

Add `...intelligence` to the `extensions` array, and read `frontmatterSchema` and `linkTargets` through refs the same way `onChange` is, so the editor is still created once per document.

- [ ] **Step 2: React to a server error**

Add an effect below the creation effect:

```tsx
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    if (!serverError) {
      view.dispatch({ effects: setServerDiagnostic.of(null) });
      return;
    }
    const lineNumber = Math.min(Math.max(serverError.line, 1), view.state.doc.lines);
    const line = view.state.doc.line(lineNumber);
    view.dispatch({
      effects: setServerDiagnostic.of({
        from: line.from,
        to: line.to,
        severity: 'error',
        message: serverError.message,
      }),
    });
  }, [serverError]);
```

- [ ] **Step 3: Surface the position from the client**

In `frontend/src/api/client.ts`, replace `saveEntity` so a rejection carries the position:

```ts
  async saveEntity(entityID: string, markdown: string, folder?: string): Promise<void> {
    const res = await fetch(`/api/game/${this.gameID}/entity/${entityID}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ markdown, folder: folder ?? '' }),
    });
    if (!res.ok) {
      const body = await res.text();
      let parsed: SaveErrorBody | null = null;
      try {
        parsed = JSON.parse(body) as SaveErrorBody;
      } catch {
        parsed = null;
      }
      const error = new Error(parsed?.error ?? body.trim() ?? `saveEntity: ${res.statusText}`);
      if (parsed?.line) {
        (error as Error & { line?: number }).line = parsed.line;
      }
      throw error;
    }
  }
```

Add `SaveErrorBody` to the type import at the top of the file.

- [ ] **Step 4: Load the schema and the index in the codex**

In `frontend/src/components/CodexDrawer.tsx`, load the schema once and the index per campaign:

```tsx
  const [frontmatterSchema, setFrontmatterSchema] = useState<FrontmatterSchema | undefined>();
  const [linkTargets, setLinkTargets] = useState<EntitySummary[]>([]);
  const [saveFailure, setSaveFailure] = useState<{ line: number; message: string } | null>(null);

  useEffect(() => {
    const client = new APIClient(gameID ?? '');
    void client.getEntityFrontmatterSchema().then(setFrontmatterSchema).catch(() => setFrontmatterSchema(undefined));
  }, []);

  useEffect(() => {
    if (!gameID) return;
    const client = new APIClient(gameID);
    void loadEntityIndex(client, gameID).then(setLinkTargets);
  }, [gameID]);
```

Pass them to the editor:

```tsx
            <MarkdownEditor
              key={entity?.id ?? 'no-note'}
              value={markdown}
              onChange={setMarkdown}
              language="markdown-frontmatter"
              onSave={() => {
                if (entity) void handleSave(entity.id);
              }}
              frontmatterSchema={frontmatterSchema}
              linkTargets={linkTargets}
              serverError={saveFailure}
              ariaLabel="Entity note markdown"
              minHeight="360px"
            />
```

In `handleSave`, catch the position, clear it on success, and invalidate the index so the note just saved is linkable:

```tsx
  const handleSave = async (id: string) => {
    try {
      setSaveError('');
      setSaveFailure(null);
      await onSave(id, markdown);
      setSavedMarkdown(markdown);
      invalidateEntityIndex();
      if (gameID) {
        setLinkTargets(await loadEntityIndex(new APIClient(gameID), gameID, true));
      }
    } catch (err) {
      const withLine = err as Error & { line?: number };
      if (withLine.line) {
        setSaveFailure({ line: withLine.line, message: withLine.message });
      }
      setSaveError(withLine.message);
    }
  };
```

- [ ] **Step 5: Do the same in the Content Studio**

In `frontend/src/components/ContentStudio.tsx`, load the schema and index alongside the folders, pass `frontmatterSchema`, `linkTargets` and `serverError` to its `MarkdownEditor`, and set `serverError` from the caught error's `line` in the existing `onSave`.

- [ ] **Step 6: Verify the whole thing by hand**

Run the dev servers and confirm, in the codex:
- typing a new key inside the frontmatter lists the known keys with their type and description;
- `type:` suggests the value set;
- an unknown key shows an info hint and still saves;
- breaking the YAML shows an error squiggle while typing;
- saving the broken document reports the same line, underlined;
- typing `[[` lists notes by name, and picking one inserts `[[id]]` or `[[id|Name]]`;
- adding a field to `entity.EntityFrontmatter`, running `go test ./pkg/gui -update-docs`, and rebuilding makes it appear in completion with no frontend change.

- [ ] **Step 7: Run the whole suite**

Run: `mise run test && mise run lint`
Expected: Go tests pass, `tsc --noEmit` is clean, `go vet` is clean.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/editor/MarkdownEditor.tsx frontend/src/api/client.ts frontend/src/components/CodexDrawer.tsx frontend/src/components/ContentStudio.tsx
git commit -m "feat(frontend): complete and lint frontmatter and wikilinks in the editor"
```

---

## Execution Notes

Recorded after the plan was executed, so the plan matches what shipped.

- **Task 5** dropped the hand-written duplicate-key scan. The `yaml` parser
  reports a repeated key as an error of its own, so the scan was a second
  implementation of a rule the parser already enforces; the plan's own step said to
  delete it if it ended up unused, and it did.
- **Task 8** attaches the intelligence through a CodeMirror `Compartment` rather
  than adding the extensions when the editor is created. The schema and the note
  list are both fetched after the first render, and the editor is deliberately
  created once, so a compartment is what lets late-arriving intelligence reach an
  editor that already exists.
- **Task 1** created `pkg/gui/schema/entity-frontmatter.json` as a committed
  artifact written by `go test ./pkg/gui -update-docs`, matching how the
  configuration reference and provider catalogue already work. The reflection
  helpers moved out of `docs_schema_test.go` into `pkg/gui/schema.go` so the
  production generator and the documentation generator share one walker.

## Self-Review

**Spec coverage.** §2.1 generated schema → Task 1, served in Task 2. §2.2 frontmatter intelligence → Tasks 4 (keys) and 5 (values, hover, lint). §2.3 legible save errors → Task 3 (Go) and Task 8 (client and editor). §2.4 wikilink intelligence → Task 6 (completion and the smart form) and Task 7 (the cached list with aliases). §5 testing → Task 1 (staleness, descriptions, required keys), Task 3 (offset and line mapping), Task 2 (route manifest), and `tsc --noEmit` in Tasks 4-8.

**Placeholder scan.** No `TBD` or "add validation". Task 5 flags two lines to delete if they end up unused, because `noUnusedLocals` would otherwise fail the build; that is a concrete instruction, not a placeholder.

**Type consistency.** `FrontmatterSchema`/`FrontmatterKeySchema` are declared in Go (Task 1) and mirrored in `frontend/src/types.ts` (Task 2) with matching JSON names (`allowUnknown`, `keys`, `name`, `type`, `required`, `values`, `description`). `DocRange` is declared in `frontmatter.ts` (Task 4) and imported by `frontmatterLint.ts` (Task 5). `lineStartOffset` and `keyLineOffset` live in `frontmatterLint.ts` (Task 5) and are used only there. `smartWikilink` and `scoreEntity` are declared in `wikilink.ts` (Task 6); `smartWikilink` is used by the completion source in the same file. `setServerDiagnostic` and `serverDiagnosticField` are declared and used in `MarkdownEditor.tsx` (Task 8). The `serverError` prop shape `{ line: number; message: string }` matches what the client sets and what `handleSave` builds.
