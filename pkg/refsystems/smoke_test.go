package refsystems

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/systemtest"
)

// TestCorpusPassesTheSmokeScenario keeps the smoke definition honest: if it
// cannot run the reference corpus, the definition is wrong, not the corpus.
func TestCorpusPassesTheSmokeScenario(t *testing.T) {
	for _, s := range List() {
		sys := systemtest.System{ID: s.ID, Script: s.Script, Mechanics: s.Mechanics}
		if failures := systemtest.Run(sys, systemtest.SmokeScenario(s.Mechanics)); len(failures) > 0 {
			t.Errorf("%s failed the smoke scenario: %+v", s.ID, failures)
		}
	}
}
