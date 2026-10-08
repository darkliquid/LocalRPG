// Package ingest turns a folder of text or a set of URLs into a draft world.
// Folder ingestion is entirely local; URL ingestion is a bounded, polite network
// action the user opts into. Neither commits anything.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/jsonrepair"
	"github.com/darkliquid/localrpg/pkg/worldgen"
)

// ChunkLimit is the largest text one chunk may carry, so a large source is split
// into many small inputs a model can handle.
const ChunkLimit = 4096

// ChunksPerCall is how many chunks one model call is given. It is the pipeline's
// batch size, so an estimate and the work agree on it.
const ChunksPerCall = worldgen.ChunksPerCall

// Source is one thing to ingest: a folder or a set of URLs.
type Source struct {
	Kind string   `json:"kind"` // "folder" | "url"
	Path string   `json:"path,omitempty"`
	URLs []string `json:"urls,omitempty"`
}

// Chunk is one bounded piece of extracted text, with the source it came from.
type Chunk struct {
	Source string `json:"source"`
	Title  string `json:"title"`
	Text   string `json:"text"`
}

// Extract reads a source into bounded text chunks.
func Extract(ctx context.Context, src Source) ([]Chunk, error) {
	switch strings.ToLower(strings.TrimSpace(src.Kind)) {
	case "folder":
		return ExtractFolder(src.Path)
	case "url", "urls":
		chunks, errs := ExtractURLs(ctx, src.URLs, FetchOptions{})
		if len(chunks) == 0 && len(errs) > 0 {
			return nil, fmt.Errorf("ingest: every URL failed: %w", errs[0])
		}
		return chunks, nil
	default:
		return nil, fmt.Errorf("ingest: unknown source kind %q", src.Kind)
	}
}

// textExtensions are the file types folder ingestion reads.
var textExtensions = map[string]bool{".md": true, ".markdown": true, ".txt": true, ".text": true}

// ExtractFolder walks a directory and reads its Markdown and text files into
// chunks. It skips dot-directories, non-text extensions, and files that look
// binary, and it makes no network call.
func ExtractFolder(dir string) ([]Chunk, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("ingest: no folder given")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("ingest: read folder: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("ingest: %q is not a folder", dir)
	}

	var chunks []Chunk
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !textExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if isBinary(data) {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		for _, part := range SplitText(string(data)) {
			chunks = append(chunks, Chunk{Source: rel, Title: rel, Text: part})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ingest: walk folder: %w", err)
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("ingest: no readable text found in %q", dir)
	}
	return chunks, nil
}

// isBinary reports whether a file's leading bytes look like anything but text.
func isBinary(data []byte) bool {
	limit := min(len(data), 512)
	for i := 0; i < limit; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// SplitText splits a document at heading and paragraph boundaries into chunks no
// larger than ChunkLimit, so one file never becomes one oversized prompt. Page
// furniture is removed first: frontmatter and link lists are metadata and
// navigation, and a model given them as prose treats ids as content and wastes
// its attention on a table of contents.
func SplitText(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = stripNavigation(stripFrontmatter(text))
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) <= ChunkLimit {
		return []string{text}
	}

	var chunks []string
	var current strings.Builder
	flush := func() {
		if body := strings.TrimSpace(current.String()); body != "" {
			chunks = append(chunks, body)
		}
		current.Reset()
	}

	for _, block := range splitBlocks(text) {
		if current.Len() > 0 && current.Len()+len(block)+2 > ChunkLimit {
			flush()
		}
		if len(block) > ChunkLimit {
			flush()
			chunks = append(chunks, hardSplit(block)...)
			continue
		}
		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(block)
	}
	flush()
	return chunks
}

// splitBlocks breaks a document at headings and blank lines, keeping each
// heading with the prose that follows it.
func splitBlocks(text string) []string {
	lines := strings.Split(text, "\n")
	var blocks []string
	var current []string
	flush := func() {
		if block := strings.TrimSpace(strings.Join(current, "\n")); block != "" {
			blocks = append(blocks, block)
		}
		current = nil
	}

	for _, line := range lines {
		if isHeading(line) {
			flush()
		}
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return blocks
}

// navLinkRe matches a Markdown list item that is nothing but a link. A
// wiki-style page uses those for its table of contents rather than for prose.
var navLinkRe = regexp.MustCompile(`^\s*[-*+]\s*\[[^\]]*\]\([^)]*\)\s*$`)

// stripFrontmatter removes a leading YAML frontmatter block. It describes the
// page, not the world: ids, parent ids, and sort orders are noise a model will
// otherwise try to read as lore.
func stripFrontmatter(text string) string {
	if !strings.HasPrefix(text, "---\n") {
		return text
	}
	rest := text[4:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return text
	}
	return strings.TrimLeft(rest[idx+4:], "\n")
}

// stripNavigation drops link-only list lines, which are a page's contents rather
// than its content.
func stripNavigation(text string) string {
	if !strings.Contains(text, "](") {
		return text
	}
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if navLinkRe.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// isHeading reports whether a line is an ATX Markdown heading.
func isHeading(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	hashes := 0
	for hashes < len(trimmed) && trimmed[hashes] == '#' {
		hashes++
	}
	if hashes == 0 || hashes > 6 {
		return false
	}
	rest := trimmed[hashes:]
	return rest == "" || strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t")
}

// hardSplit cuts an oversized block into ChunkLimit-sized pieces at a line
// boundary where possible.
func hardSplit(block string) []string {
	var out []string
	for len(block) > ChunkLimit {
		cut := strings.LastIndex(block[:ChunkLimit], "\n")
		if cut <= 0 {
			cut = ChunkLimit
		}
		out = append(out, strings.TrimSpace(block[:cut]))
		block = strings.TrimSpace(block[cut:])
	}
	if block != "" {
		out = append(out, block)
	}
	return out
}

// Batch groups chunks so each model call sees a bounded amount of text.
func Batch(chunks []Chunk, perCall int) [][]Chunk {
	if perCall <= 0 {
		perCall = ChunksPerCall
	}
	var batches [][]Chunk
	for i := 0; i < len(chunks); i += perCall {
		end := min(i+perCall, len(chunks))
		batches = append(batches, chunks[i:end])
	}
	return batches
}

// ingestSchema is the reply shape: an optional outline plus entities.
const ingestSchema = `{"type":"object","properties":{` +
	`"name":{"type":"string"},"genre":{"type":"string"},"description":{"type":"string"},` +
	`"lore":{"type":"string"},"entities":{"type":"array","items":{"type":"object","properties":{` +
	`"name":{"type":"string"},"type":{"type":"string"},"description":{"type":"string"},` +
	`"tags":{"type":"array","items":{"type":"string"}}}}}}}`

const ingestSystem = `You read source material and extract a tabletop roleplaying world from it.
You are exhaustive: every named thing in the source becomes an entity.
You keep every name exactly as the source writes it, and you invent nothing.
You return one JSON object and nothing else.`

// entityKinds tells the model what counts as an entity. Without this it answers
// with characters and stops, because "entity" on its own reads as "person".
const entityKinds = `Each distinct thing the source names becomes one entity, typed by what it is:

- a place, district, region, building, or room is a "location"
- a named person, or a group of unnamed people in a role, is a "character"
- an organisation, guild, order, house, crew, or cult is a "faction"
- a kind of being, people, ancestry, or creature is a "species"
- an object, material, technology, or artefact is an "item"
- a dated happening, battle, or era is an "event"
- a custom, ritual, holiday, belief, language, trade, office, or idea is a "concept"`

const ingestInstructions = `Work through the source and list everything it names. A page that names ten things yields ten entities: do not summarise a page into one entity, and do not skip a thing because the source mentions it only once. A glossary entry, a heading with a definition, a bolded term in a list, and a passing mention all count. A heading such as "Customs & Rituals" is not itself an entity; the customs beneath it are.

Write two to four sentences for each entity, in the source's own terms, with enough detail to run a scene from it. A name on its own is a failure.

Also write lore: one short paragraph of markdown prose covering the world's history, peoples, customs, places, or conflicts, drawn only from this source material. Include lore in every reply, because it is collected from every part of the source rather than only the first. Keep the whole reply compact: it is one part of a longer reading, not a summary of everything.`

// ingestEntity is one entity as the model returns it.
type ingestEntity struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type ingestReply struct {
	Name        string         `json:"name"`
	Genre       string         `json:"genre"`
	Description string         `json:"description"`
	Lore        string         `json:"lore"`
	Entities    []ingestEntity `json:"entities"`
}

// Progress reports one batch of an ingestion, so a long import can say what it
// is reading and how much is left.
type Progress struct {
	// Batch is the one-based index of this batch, and Batches is how many the
	// whole source needs, so a caller can estimate the time remaining.
	Batch   int
	Batches int
	// Sources names the files or pages this batch read.
	Sources []string
	// Found is how many entities this batch added or improved, Total is how many
	// the import has produced so far, and Names is what this batch touched.
	Found int
	Total int
	Names []string
	// CutOff counts the batches whose reply ran out before it finished. What they
	// wrote before the cut was kept, and the count is reported so the user knows
	// the import is missing the tail of a page rather than believing it complete.
	CutOff int
}

// BuildContext names the world an ingestion is adding to. An empty context means
// the chunks describe a world of their own, which is what a whole-world import
// wants; a filled one steers the extraction to fit a world that already exists.
type BuildContext struct {
	Name        string
	Genre       string
	Description string
	Lore        string
	Entities    []worldgen.EntitySummary
}

// Build turns chunks into a draft world, reusing WG-1's generator, and records
// each entity's source so provenance survives into the review.
func Build(ctx context.Context, gen worldgen.Generator, chunks []Chunk, brief worldgen.Brief) (worldgen.Draft, error) {
	return BuildInto(ctx, gen, chunks, brief, BuildContext{}, nil)
}

// BuildInto is Build for chunks that extend an existing world: the world's
// identity, lore, and entities are given to the model so the extracted notes fit
// it, and every link is validated against that world as well as the new notes.
func BuildInto(ctx context.Context, gen worldgen.Generator, chunks []Chunk, brief worldgen.Brief, into BuildContext, onProgress func(Progress)) (worldgen.Draft, error) {
	if len(chunks) == 0 {
		return worldgen.Draft{}, fmt.Errorf("ingest: nothing to build from")
	}
	if gen == nil {
		return worldgen.Draft{}, fmt.Errorf("ingest: no generator configured")
	}

	draft := worldgen.Draft{}
	seen := map[string]struct{}{}
	at := map[string]int{}
	for _, e := range into.Entities {
		seen[e.ID] = struct{}{}
	}

	batches := Batch(chunks, ChunksPerCall)
	// The batch count is known before the first call, so a caller can show how
	// much work is coming rather than only how much is done.
	if onProgress != nil {
		onProgress(Progress{Batch: 0, Batches: len(batches)})
	}

	cutOff := 0

	// readBatch reads one batch, halving it when the reply cannot be parsed at
	// all. A model that runs out of room on four pages still answers for two, and
	// losing a whole import to one oversized call is not a trade worth making.
	var readBatch func(batch []Chunk, index int, first bool, touched *[]string, cut *bool) error
	readBatch = func(batch []Chunk, index int, first bool, touched *[]string, cut *bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Each call is told what the calls before it already produced. Without
		// that it re-names the same handful of salient things every time, and the
		// duplicates are the ones a reader most wanted to keep.
		raw, err := gen.GenerateJSON(ctx, buildPrompt(brief, batch, first, into, draft.Entities), ingestSchema)
		if err != nil {
			return fmt.Errorf("ingest: batch %d: %w", index, err)
		}

		reply, salvaged, parseErr := decodeReply(raw)
		if parseErr != nil {
			if len(batch) > 1 {
				half := len(batch) / 2
				if err := readBatch(batch[:half], index, first, touched, cut); err != nil {
					return err
				}
				return readBatch(batch[half:], index, false, touched, cut)
			}
			// One page and no reply: nothing to read, so the import carries on
			// rather than losing the fifty batches around it.
			*cut = true
			return nil
		}
		if salvaged {
			*cut = true
		}
		*touched = append(*touched, applyReply(&draft, reply, batch, first, seen, at)...)
		return nil
	}

	for i, batch := range batches {
		var touched []string
		cut := false
		if err := readBatch(batch, i+1, i == 0, &touched, &cut); err != nil {
			return draft, err
		}
		if cut {
			cutOff++
		}
		if onProgress != nil {
			onProgress(Progress{
				Batch:   i + 1,
				Batches: len(batches),
				Sources: chunkSourceList(batch),
				Found:   len(touched),
				Total:   len(draft.Entities),
				Names:   touched,
				CutOff:  cutOff,
			})
		}
	}

	if into.Name != "" {
		// Extending a world: the draft borrows its identity, and the caller keeps
		// only the notes.
		draft.World.ID = ""
		draft.World.Name = into.Name
		draft.World.Genre = firstNonEmpty(draft.World.Genre, into.Genre)
		draft.World.Description = firstNonEmpty(draft.World.Description, into.Description)
	}
	if draft.World.Name == "" {
		draft.World.Name = firstNonEmpty(brief.Name, "Imported World")
	}
	draft.World.ID = entity.Slugify(draft.World.Name)
	if draft.World.ID == "" {
		draft.World.ID = "imported-world"
	}
	draft.ID = draft.World.ID
	// Links resolve against the notes this batch produced and, when the chunks
	// extend a world, against that world's own entities too. Linking against the
	// batch alone first would flatten a link to an existing note before the world
	// ever saw it.
	if len(into.Entities) > 0 {
		draft.Entities = worldgen.LinkBatch(draft.Entities, into.Entities)
	} else {
		draft = worldgen.LinkDraft(draft)
	}
	draft.Sections = worldgen.SplitLoreSections(draft.Lore)
	return draft, nil
}

// applyReply merges one model reply into the draft, tagging every new entity
// with the source chunks the batch came from. A name that appears again is
// merged rather than dropped: the later mention often carries the description
// the first one lacked, and losing it is how an import ends up with names and
// nothing else.
func applyReply(draft *worldgen.Draft, reply ingestReply, batch []Chunk, first bool, seen map[string]struct{}, at map[string]int) []string {
	var touched []string
	if first {
		draft.World.Name = firstNonEmpty(reply.Name, draft.World.Name)
		draft.World.Genre = firstNonEmpty(reply.Genre, draft.World.Genre)
		draft.World.Description = firstNonEmpty(reply.Description, draft.World.Description)
	} else if draft.World.Description == "" {
		draft.World.Description = strings.TrimSpace(reply.Description)
	}
	if lore := strings.TrimSpace(reply.Lore); lore != "" {
		draft.Lore = joinLore(draft.Lore, lore)
	}

	source := chunkSources(batch)
	for _, spec := range reply.Entities {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		id := entity.Slugify(name)
		if id == "" {
			continue
		}
		incoming := worldgen.DraftEntity{
			ID:     id,
			Name:   name,
			Type:   firstNonEmpty(spec.Type, "concept"),
			Tags:   cleanTags(spec.Tags),
			Body:   strings.TrimSpace(spec.Description),
			Source: source,
		}
		if _, exists := seen[id]; exists {
			if position, ok := at[id]; ok {
				mergeEntity(&draft.Entities[position], incoming)
				touched = append(touched, name)
				continue
			}
			// The world already has this note; adding entities must not touch it.
			continue
		}
		if incoming.Body == "" {
			incoming.Body = name + "."
		}
		incoming.Folder = entityFolderFor(incoming.Type)
		seen[id] = struct{}{}
		at[id] = len(draft.Entities)
		draft.Entities = append(draft.Entities, incoming)
		touched = append(touched, name)
	}
	return touched
}

// mergeEntity folds a repeated mention into the entity it repeats, keeping the
// fuller description and the union of the tags and sources.
func mergeEntity(into *worldgen.DraftEntity, from worldgen.DraftEntity) {
	if len(from.Body) > len(into.Body) {
		into.Body = from.Body
	}
	if into.Type == "concept" && from.Type != "concept" {
		into.Type = from.Type
		into.Folder = entityFolderFor(from.Type)
	}
	for _, tag := range from.Tags {
		into.Tags = appendUnique(into.Tags, tag)
	}
	into.Source = joinDistinct(into.Source, from.Source)
}

// joinLore appends a lore section, keeping one blank line between them.
func joinLore(existing, addition string) string {
	existing = strings.TrimRight(existing, "\n")
	if existing == "" {
		return addition + "\n"
	}
	return existing + "\n\n" + addition + "\n"
}

// joinDistinct joins two comma-separated source lists without repeating a name.
func joinDistinct(existing, addition string) string {
	if addition == "" || existing == addition {
		return existing
	}
	if existing == "" {
		return addition
	}
	parts := strings.Split(existing, ", ")
	for _, part := range strings.Split(addition, ", ") {
		parts = appendUnique(parts, part)
	}
	return strings.Join(parts, ", ")
}

// entityFolders groups an imported world's notes by kind, so a source that yields
// sixty entities does not land as one flat list in the studio.
var entityFolders = map[string]string{
	"location":  "locations",
	"character": "characters",
	"faction":   "factions",
	"species":   "species",
	"item":      "items",
	"event":     "events",
	"concept":   "concepts",
}

// entityFolderFor is the folder an entity of this kind belongs in, or "" when the
// kind is one the source invented.
func entityFolderFor(kind string) string {
	return entityFolders[strings.ToLower(strings.TrimSpace(kind))]
}

func appendUnique(list []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return list
	}
	for _, existing := range list {
		if strings.EqualFold(existing, value) {
			return list
		}
	}
	return append(list, value)
}

// chunkSources names where a batch's text came from, so an entity can be traced.
func chunkSources(batch []Chunk) string {
	return strings.Join(chunkSourceList(batch), ", ")
}

// chunkSourceList is the distinct sources a batch read, in order.
func chunkSourceList(batch []Chunk) []string {
	seen := map[string]struct{}{}
	var sources []string
	for _, c := range batch {
		if c.Source == "" {
			continue
		}
		if _, dup := seen[c.Source]; dup {
			continue
		}
		seen[c.Source] = struct{}{}
		sources = append(sources, c.Source)
	}
	return sources
}

// buildPrompt assembles a bounded extraction prompt for one batch of chunks. It
// asks for every entity the source describes rather than a count: an ingestion
// reports what is in the source, and inventing entities to reach a target is the
// opposite of what a source is for. extracted is what the earlier batches of this
// same import already produced, so a batch adds rather than repeats.
func buildPrompt(brief worldgen.Brief, batch []Chunk, first bool, into BuildContext, extracted []worldgen.DraftEntity) string {
	var b strings.Builder
	b.WriteString(ingestSystem)
	if into.Name != "" {
		b.WriteString("\n\nExtract everything the source material below says about the world \"" +
			into.Name + "\".\n")
	} else {
		b.WriteString("\n\nExtract the world described by the source material below.\n")
	}
	b.WriteString("\n" + entityKinds + "\n")
	b.WriteString("\n" + ingestInstructions + "\n")

	if brief.Premise != "" {
		b.WriteString("\nAdditional instruction: " + truncate(brief.Premise, 500) + "\n")
	}
	if into.Name != "" {
		if into.Genre != "" {
			b.WriteString("\nThe world's genre is " + into.Genre + ".\n")
		}
		if into.Description != "" {
			b.WriteString("The world's premise: " + truncate(into.Description, 400) + "\n")
		}
		if lore := truncate(into.Lore, 1200); lore != "" {
			b.WriteString("\nExisting lore, for tone and continuity:\n" + lore + "\n")
		}
		if existing := summaryList(into.Entities, 60); existing != "" {
			b.WriteString("\nEntities this world already has. Do not repeat them, and link to them " +
				"with [[Name]] where the source supports it:\n" + existing)
		}
		b.WriteString("\nDo not propose a new world name, genre, or description; the world already exists.\n")
	} else if first {
		b.WriteString("\nAlso give the world a name, a genre, and a one-line description drawn from the " +
			"source material.\n")
	}

	// The running inventory is what makes the calls build on each other rather
	// than each rediscovering the same handful of headline names.
	if already := extractedList(extracted, extractedLimit); already != "" {
		b.WriteString("\nAlready extracted from earlier parts of this same source. Do not repeat " +
			"these; add only what is missing from them:\n" + already)
	}
	if !first {
		b.WriteString("\nThis is a later part of the source.\n")
	}

	b.WriteString("\nSource material:\n")
	for _, c := range batch {
		b.WriteString("\n--- " + firstNonEmpty(c.Title, c.Source) + " ---\n")
		b.WriteString(c.Text + "\n")
	}
	b.WriteString(`\nReturn {"name":...,"genre":...,"description":...,"lore":...,"entities":[{"name":...,"type":...,"description":...,"tags":[...]}]}.`)
	return b.String()
}

// extractedLimit bounds the running inventory in the prompt. A long import would
// otherwise grow its own prompt past the source it is reading.
const extractedLimit = 200

// extractedList renders the names already collected, bounded so the prompt stays
// smaller than the material it describes.
func extractedList(entities []worldgen.DraftEntity, limit int) string {
	var b strings.Builder
	shown := 0
	for _, e := range entities {
		if shown >= limit {
			b.WriteString("- (and more)\n")
			break
		}
		if e.Name == "" {
			continue
		}
		b.WriteString("- " + e.Name + " (" + firstNonEmpty(e.Type, "concept") + ")\n")
		shown++
	}
	return b.String()
}

// summaryList renders a bounded list of a world's existing entities, so a
// model can link to them by name.
func summaryList(entities []worldgen.EntitySummary, limit int) string {
	var b strings.Builder
	count := 0
	for _, e := range entities {
		if count >= limit {
			break
		}
		if e.Name == "" {
			continue
		}
		b.WriteString("- " + e.Name + " (" + firstNonEmpty(e.Type, "concept") + ")\n")
		count++
	}
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cleanTags(tags []string) []string {
	var out []string
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		seen := false
		for _, existing := range out {
			if strings.EqualFold(existing, trimmed) {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, trimmed)
		}
	}
	return out
}

func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

// decodeReply parses one batch's reply, salvaging the entities from a reply that
// ran out of room. A response cut off mid-entity is not a failed response: the
// entities before the cut are whole, and a long import cannot afford to lose a
// batch, or to fail outright, because one call wrote more than the model had room
// for. salvaged reports that the reply was short, so the caller can say so.
func decodeReply(raw []byte) (ingestReply, bool, error) {
	payload := raw
	if res := jsonrepair.Repair(raw); res.OK {
		payload = res.Payload
	}

	var reply ingestReply
	parseErr := json.Unmarshal(payload, &reply)
	if parseErr == nil {
		return reply, false, nil
	}

	// The reply did not parse as a whole. It may still hold the entities written
	// before it was cut off, so keep those rather than losing the batch.
	salvaged := ingestReply{}
	for _, element := range jsonrepair.ArrayElements(payload) {
		var entity ingestEntity
		if err := json.Unmarshal(element, &entity); err == nil && entity.Name != "" {
			salvaged.Entities = append(salvaged.Entities, entity)
		}
	}
	if len(salvaged.Entities) == 0 {
		return ingestReply{}, false, fmt.Errorf("ingest: parse model reply: %w", parseErr)
	}
	return salvaged, true, nil
}
