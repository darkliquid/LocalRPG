package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestRootViewSnapshot(t *testing.T) {
	ui.Snapshot(t, "root", 800, 600, RootView)
}
