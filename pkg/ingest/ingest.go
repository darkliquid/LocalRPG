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
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/jsonrepair"
	"github.com/darkliquid/localrpg/pkg/worldgen"
)

// ChunkLimit is the largest text one chunk may carry, so a large source is split
// into many small inputs a model can handle.
const ChunkLimit = 4096

// ChunksPerCall is how many chunks one model call is given.
const ChunksPerCall = 4

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
// larger than ChunkLimit, so one file never becomes one oversized prompt.
func SplitText(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
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

const ingestSystem = `You read source material and extract a tabletop roleplaying world from it. ` +
	`Keep names as they appear in the source. Return one JSON object and nothing else.`

type ingestReply struct {
	Name        string `json:"name"`
	Genre       string `json:"genre"`
	Description string `json:"description"`
	Lore        string `json:"lore"`
	Entities    []struct {
		Name        string   `json:"name"`
		Type        string   `json:"type"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	} `json:"entities"`
}

// Build turns chunks into a draft world, reusing WG-1's generator, and records
// each entity's source so provenance survives into the review.
func Build(ctx context.Context, gen worldgen.Generator, chunks []Chunk, brief worldgen.Brief) (worldgen.Draft, error) {
	if len(chunks) == 0 {
		return worldgen.Draft{}, fmt.Errorf("ingest: nothing to build from")
	}
	if gen == nil {
		return worldgen.Draft{}, fmt.Errorf("ingest: no generator configured")
	}

	draft := worldgen.Draft{}
	seen := map[string]struct{}{}
	for i, batch := range Batch(chunks, ChunksPerCall) {
		if err := ctx.Err(); err != nil {
			return draft, err
		}
		raw, err := gen.GenerateJSON(ctx, buildPrompt(brief, batch, i == 0), ingestSchema)
		if err != nil {
			return draft, fmt.Errorf("ingest: batch %d: %w", i+1, err)
		}
		var reply ingestReply
		if err := decodeJSON(raw, &reply); err != nil {
			return draft, err
		}
		applyReply(&draft, reply, batch, i == 0, seen)
	}

	if draft.World.Name == "" {
		draft.World.Name = firstNonEmpty(brief.Name, "Imported World")
	}
	draft.World.ID = entity.Slugify(draft.World.Name)
	if draft.World.ID == "" {
		draft.World.ID = "imported-world"
	}
	draft.ID = draft.World.ID
	draft = worldgen.LinkDraft(draft)
	draft.Sections = worldgen.SplitLoreSections(draft.Lore)
	return draft, nil
}

// applyReply merges one model reply into the draft, tagging every new entity
// with the source chunks the batch came from.
func applyReply(draft *worldgen.Draft, reply ingestReply, batch []Chunk, first bool, seen map[string]struct{}) {
	if first {
		draft.World.Name = firstNonEmpty(reply.Name, draft.World.Name)
		draft.World.Genre = firstNonEmpty(reply.Genre, draft.World.Genre)
		draft.World.Description = firstNonEmpty(reply.Description, draft.World.Description)
		draft.Lore = firstNonEmpty(reply.Lore, draft.Lore)
	} else {
		if reply.Lore != "" {
			draft.Lore = strings.TrimRight(draft.Lore, "\n") + "\n\n" + strings.TrimSpace(reply.Lore) + "\n"
		}
		if draft.World.Description == "" {
			draft.World.Description = strings.TrimSpace(reply.Description)
		}
	}

	source := chunkSources(batch)
	for _, spec := range reply.Entities {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		id := entity.Slugify(name)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		kind := firstNonEmpty(spec.Type, "concept")
		draft.Entities = append(draft.Entities, worldgen.DraftEntity{
			ID:     id,
			Name:   name,
			Type:   kind,
			Tags:   cleanTags(spec.Tags),
			Body:   firstNonEmpty(spec.Description, name+"."),
			Source: source,
		})
	}
}

// chunkSources names where a batch's text came from, so an entity can be traced.
func chunkSources(batch []Chunk) string {
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
	return strings.Join(sources, ", ")
}

// buildPrompt assembles a bounded extraction prompt for one batch of chunks.
func buildPrompt(brief worldgen.Brief, batch []Chunk, first bool) string {
	var b strings.Builder
	b.WriteString(ingestSystem)
	b.WriteString("\n\nExtract the world described by the source material below.\n")
	if brief.Premise != "" {
		b.WriteString("\nAdditional instruction: " + truncate(brief.Premise, 500) + "\n")
	}
	if first {
		b.WriteString("\nAlso give the world a name, a genre, a one-line description, and a short lore " +
			"document assembled from the source.\n")
	} else {
		b.WriteString("\nThis is a later part of the source; add any further lore and entities it reveals.\n")
	}
	b.WriteString("\nSource material:\n")
	for _, c := range batch {
		b.WriteString("\n--- " + firstNonEmpty(c.Title, c.Source) + " ---\n")
		b.WriteString(c.Text + "\n")
	}
	b.WriteString(`\nReturn {"name":...,"genre":...,"description":...,"lore":...,"entities":[...]}.`)
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

// decodeJSON repairs a model reply that is fenced, padded with prose, or
// unterminated, then unmarshals it.
func decodeJSON(raw []byte, v any) error {
	payload := raw
	if res := jsonrepair.Repair(raw); res.OK {
		payload = res.Payload
	}
	if err := json.Unmarshal(payload, v); err != nil {
		return fmt.Errorf("ingest: parse model reply: %w", err)
	}
	return nil
}
