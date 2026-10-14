// Package entity defines the core domain model for campaign entities, including
// characters, locations, factions, items, and lore notes.
//
// Entities are represented as schema-agnostic markdown files with structured YAML
// frontmatter (EntityFrontmatter, Entity in entity.go). This package provides
// serialization and parsing (ParseMarkdownEntity, SerializeMarkdown), wikilink
// mention extraction ([[Entity Name]] in mention.go), and helpers for entity
// appearances, portraits, voice profile associations, and aliases.
//
// What entity is NOT:
//   - It does NOT perform durable disk I/O or coordinate transaction boundaries;
//     writing and saving entity notes is managed by pkg/engine/timeline.go.
//   - It does NOT query or search entities across a campaign; indexing, full-text
//     search, and semantic vector similarity belong to pkg/storage.
//   - It does NOT manage dynamic game mechanics state (such as hit points or gold);
//     hierarchical in-memory mechanics state belongs to pkg/state.
package entity
