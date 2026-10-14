// Package ingest parses external documents, local text folders, and web pages into
// candidate world drafts for LocalRPG campaigns.
//
// It provides polite HTTP crawling (fetch.go) with domain safety and size bounds,
// text chunking, and LLM-driven information extraction (ingest.go) to discover
// entities, factions, locations, and lore from unstructured sources. Extracted
// content is assembled into draft world models for user inspection and approval.
//
// What ingest is NOT:
//   - It does NOT commit or persist changes directly to an active campaign; all
//     ingestion results are non-destructive drafts that must be explicitly accepted.
//   - It does NOT generate tabletop RPG rulesets or mechanics; system generation
//     belongs to pkg/sysgen and system execution to pkg/rules.
//   - It does NOT manage live campaign entities or timeline records; that belongs
//     to pkg/entity and pkg/engine/timeline.go.
package ingest
