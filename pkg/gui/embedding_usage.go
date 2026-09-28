package gui

import "github.com/darkliquid/localrpg/pkg/harness"

// RecordEmbeddingUsage files an embedding call's spend against the shared
// ledger. Embeddings run outside a turn, so the row carries turn 0 and the
// global scope, the same convention asset previews use.
func (s *Service) RecordEmbeddingUsage(providerKey, model string, inputTokens, requests int) {
	if providerKey == "" {
		return
	}
	s.RecordUsageGlobal("embedding", harness.Usage{
		Provider:    providerKey,
		Model:       model,
		InputTokens: inputTokens,
		Requests:    requests,
	})
}
