package content_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

func TestManifestRoundTrip(t *testing.T) {
	m := content.Manifest{
		ID:      "ashen_reach",
		Name:    "Ashen Reach",
		Version: "1.0.0",
		Type:    "world",
		Files:   []content.FileEntry{{Path: "world.yaml", SHA256: "ab", Size: 3}},
	}
	b, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := content.ParseManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != m.ID || len(got.Files) != 1 {
		t.Fatalf("manifest = %+v", got)
	}
}
