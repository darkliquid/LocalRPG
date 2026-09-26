package ui

import (
	"testing"

	"go.hasen.dev/shirei"
)

// Snapshot renders view headlessly at w×h and compares it against the golden
// at testdata/snapshots/<name>.png. On a missing golden it writes one and
// passes; set UPDATE_SNAPSHOTS=1 to regenerate. It skips when the host has no
// usable system fonts, because shirei's text shaping cannot run there.
func Snapshot(t *testing.T, name string, w, h int, view func()) {
	t.Helper()
	res := shirei.Snapshot(t.Name(), name, w, h, shirei.FrameFn(view))
	switch res.Status {
	case shirei.SnapMatch, shirei.SnapCreated, shirei.SnapUpdated:
		if res.Status != shirei.SnapMatch {
			t.Logf("snapshot %s: %s (golden=%s)", name, res.Status, res.Golden)
		}
	case shirei.SnapSkip:
		t.Skipf("snapshot %s skipped: %s", name, res.Reason)
	default:
		t.Fatalf("snapshot %s: %s (golden=%s actual=%s) err=%v",
			name, res.Status, res.Golden, res.Actual, res.Err)
	}
}
