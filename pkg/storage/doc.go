// Package storage provides a derived, fast query index and search cache backed by
// SQLite.
//
// The Store (store.go) maintains an indexed representation of campaign entities,
// turns, full-text search tables (FTS5), vector embeddings for semantic memory
// recall, and background TTS generation queues. Sync (sync.go) reconciles the
// database against the source files on disk.
//
// What storage is NOT:
//   - It is NOT the primary, durable source of truth. The SQLite database is a
//     disposable, derived index that can be dropped and entirely reconstructed from
//     the campaign's durable source files.
//   - It does NOT manage durable file persistence; append-only turn records
//     (history.jsonl) and entity markdown files are owned and written exclusively
//     by pkg/engine (specifically timeline.go).
//   - It does NOT parse entity markdown formats or schema frontmatter; that is the
//     responsibility of pkg/entity.
package storage
