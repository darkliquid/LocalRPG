package desktop

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"math"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"golang.org/x/image/vector"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// refreshGraph reloads the entity graph and renders it offscreen.
func refreshGraph() {
	svc := liveService
	if svc == nil {
		return
	}
	gameID := appState.OpenGame
	go func() {
		graph := loadGraph(context.Background(), svc, gameID)
		var img *image.RGBA
		if graph != nil {
			img = renderGraph(graph, 360)
		}
		WithFrameLock(func() {
			appState.Graph = graph
			appState.GraphImage = img
		})
		RequestNextFrame()
	}()
}

// layoutGraph places nodes evenly on a ring, with the first node at the top.
func layoutGraph(g *gui.GraphDTO, size int) map[string][2]float64 {
	pos := make(map[string][2]float64, len(g.Nodes))
	n := len(g.Nodes)
	if n == 0 {
		return pos
	}
	centre := float64(size) / 2
	radius := centre - 12
	for i, node := range g.Nodes {
		angle := (float64(i)/float64(n))*2*math.Pi - math.Pi/2
		pos[node.ID] = [2]float64{centre + radius*math.Cos(angle), centre + radius*math.Sin(angle)}
	}
	return pos
}

// renderGraph rasterises the graph into an image, because shirei has no canvas.
func renderGraph(g *gui.GraphDTO, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.RGBA{R: 12, G: 10, B: 9, A: 255}), image.Point{}, draw.Src)
	if g == nil {
		return dst
	}
	pos := layoutGraph(g, size)
	for _, link := range g.Links {
		a, okA := pos[link.Source]
		b, okB := pos[link.Target]
		if !okA || !okB {
			continue
		}
		drawLine(dst, a, b, color.RGBA{R: 120, G: 110, B: 100, A: 255})
	}
	for _, node := range g.Nodes {
		centre, ok := pos[node.ID]
		if !ok {
			continue
		}
		drawDisc(dst, centre, 6, nodeColor(node.Type))
	}
	return dst
}

func nodeColor(kind string) color.RGBA {
	switch kind {
	case "character":
		return color.RGBA{R: 245, G: 158, B: 11, A: 255}
	case "npc":
		return color.RGBA{R: 56, G: 189, B: 248, A: 255}
	case "location":
		return color.RGBA{R: 168, G: 85, B: 247, A: 255}
	default:
		return color.RGBA{R: 120, G: 113, B: 108, A: 255}
	}
}

func drawDisc(dst *image.RGBA, centre [2]float64, radius float64, col color.RGBA) {
	cx, cy, r := float32(centre[0]), float32(centre[1]), float32(radius)
	var ras vector.Rasterizer
	ras.Reset(dst.Bounds().Dx(), dst.Bounds().Dy())
	ras.MoveTo(cx+r, cy)
	ras.QuadTo(cx+r, cy+r, cx, cy+r)
	ras.QuadTo(cx-r, cy+r, cx-r, cy)
	ras.QuadTo(cx-r, cy-r, cx, cy-r)
	ras.QuadTo(cx+r, cy-r, cx+r, cy)
	ras.ClosePath()
	ras.Draw(dst, dst.Bounds(), image.NewUniform(col), image.Point{})
}

func drawLine(dst *image.RGBA, a, b [2]float64, col color.RGBA) {
	const halfWidth = 1.0
	dx, dy := b[0]-a[0], b[1]-a[1]
	length := math.Hypot(dx, dy)
	if length == 0 {
		return
	}
	nx, ny := float32(-dy/length*halfWidth), float32(dx/length*halfWidth)
	ax, ay, bx, by := float32(a[0]), float32(a[1]), float32(b[0]), float32(b[1])
	var ras vector.Rasterizer
	ras.Reset(dst.Bounds().Dx(), dst.Bounds().Dy())
	ras.MoveTo(ax+nx, ay+ny)
	ras.LineTo(bx+nx, by+ny)
	ras.LineTo(bx-nx, by-ny)
	ras.LineTo(ax-nx, ay-ny)
	ras.ClosePath()
	ras.Draw(dst, dst.Bounds(), image.NewUniform(col), image.Point{})
}

func graphDrawer(p ui.Palette) {
	NextAccessName("graph.refresh")
	if Button(NoIcon, "Refresh") {
		refreshGraph()
	}
	AssignAccess()

	if appState.GraphImage == nil {
		Label("No graph yet.", FontSize(12), TextColorVec(p.Muted))
		return
	}

	id := UseImage("lore-graph", appState.GraphImage)
	ImageView(id, Vec2{GetContentWidth(), GetContentWidth()})

	if appState.Graph != nil {
		for _, node := range appState.Graph.Nodes {
			id := node.ID
			NextAccessName("graph.node." + id)
			if Button(NoIcon, node.Label) {
				openEntity(id)
			}
			AssignAccess()
		}
	}
}
