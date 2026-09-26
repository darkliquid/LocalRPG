package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestLayoutGraphIsOnTheRing(t *testing.T) {
	g := &gui.GraphDTO{
		Nodes: []gui.GraphNodeDTO{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}, {ID: "c", Label: "C"}},
		Links: []gui.GraphLinkDTO{{Source: "a", Target: "b"}},
	}
	pos := layoutGraph(g, 360)
	if len(pos) != 3 {
		t.Fatalf("positions = %d, want 3", len(pos))
	}
	for id, xy := range pos {
		if xy[0] < 0 || xy[0] > 360 || xy[1] < 0 || xy[1] > 360 {
			t.Errorf("node %s off canvas: %v", id, xy)
		}
	}
}

func TestRenderGraphProducesRGBA(t *testing.T) {
	g := &gui.GraphDTO{Nodes: []gui.GraphNodeDTO{{ID: "a", Label: "A"}}}
	img := renderGraph(g, 200)
	if img == nil || img.Bounds().Dx() != 200 {
		t.Fatalf("expected a 200px image, got %+v", img)
	}
}
