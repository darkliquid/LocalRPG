package export

import (
	"os"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// layerArtResolver is an art resolver that can also layer a scene, as the built-in
// generator does. The embedded interface is nil: only SceneLayers is called.
type layerArtResolver struct {
	scene.ArtResolver
	layers media.LayeredScene
	ok     bool
}

func (l layerArtResolver) SceneLayers(*entity.Entity) (media.LayeredScene, bool) {
	return l.layers, l.ok
}

func TestAttachSceneLayersWritesTheLayers(t *testing.T) {
	root := t.TempDir()
	store, err := storage.OpenGameStore(core.NewPathResolver(root), "layered")
	if err != nil {
		t.Fatalf("open game store: %v", err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "clearing", Name: "Clearing", Type: "location", Hash: "h1"}); err != nil {
		t.Fatal(err)
	}

	compiler := &ScriptCompiler{resolver: core.NewPathResolver(root)}
	script := &scene.Script{Scenes: []scene.Scene{{LocationID: "clearing"}, {LocationID: ""}}}
	resolver := layerArtResolver{ok: true, layers: media.LayeredScene{Width: 8, Height: 8, Layers: []media.Layer{
		{Depth: 0, SVG: []byte(`<svg><rect/></svg>`)},
		{Depth: 1, SVG: []byte(`<svg><circle/></svg>`)},
	}}}

	compiler.attachSceneLayers(script, store, resolver)

	if len(script.Scenes[0].Layers) != 2 {
		t.Fatalf("layers = %+v", script.Scenes[0].Layers)
	}
	if script.Scenes[0].Layers[0].Depth != 0 || script.Scenes[0].Layers[1].Depth != 1 {
		t.Fatalf("depths are not preserved: %+v", script.Scenes[0].Layers)
	}
	for _, layer := range script.Scenes[0].Layers {
		if _, err := os.Stat(layer.Art); err != nil {
			t.Fatalf("layer art was not written: %v", err)
		}
	}
	if len(script.Scenes[1].Layers) != 0 {
		t.Fatal("an unlocated scene should have no layers")
	}
}

func TestAttachSceneLayersSkipsAFlatResolver(t *testing.T) {
	root := t.TempDir()
	store, err := storage.OpenGameStore(core.NewPathResolver(root), "flat")
	if err != nil {
		t.Fatalf("open game store: %v", err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "clearing", Name: "Clearing", Type: "location", Hash: "h1"}); err != nil {
		t.Fatal(err)
	}

	compiler := &ScriptCompiler{resolver: core.NewPathResolver(root)}
	script := &scene.Script{Scenes: []scene.Scene{{LocationID: "clearing"}}}

	// A resolver that only makes flat images reports no layers, and the scene is
	// left exactly as it was.
	compiler.attachSceneLayers(script, store, layerArtResolver{ok: false})
	if len(script.Scenes[0].Layers) != 0 {
		t.Fatalf("a flat resolver should add no layers: %+v", script.Scenes[0].Layers)
	}
}
