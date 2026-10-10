package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/scene"
)

// bundleSizeTarget is the pinned upper bound for the size fixture's bundle. It
// exists so a change that stops downscaling, or adds weight, fails here rather
// than in a user's download. Re-measure with the benchmark before moving it.
const bundleSizeTarget = 400 * 1024

// demoBundleScript builds a script with one large illustration and one clip, the
// shape that grows a bundle fastest.
func demoBundleScript(tb testing.TB) (*scene.Script, string) {
	tb.Helper()
	dir := tb.TempDir()
	art := bigPNG(tb, 4000, 3000)
	clip := filepath.Join(dir, "welcome.wav")
	if err := os.WriteFile(clip, []byte("RIFF....WAVEfmt ....data"), 0644); err != nil {
		tb.Fatal(err)
	}
	return &scene.Script{
		GameName: "Campaign One",
		Scenes: []scene.Scene{{
			LocationName: "Alden Tavern",
			ArtPath:      art,
			Duration:     4 * time.Second,
			Beats: []scene.Beat{
				{Kind: scene.BeatSceneCard, Text: "Alden Tavern", ArtPath: art, Duration: 2 * time.Second},
				{Kind: scene.BeatSpeech, Speaker: "Garrick", Text: "Welcome.", ArtPath: art,
					AudioPaths: []string{clip}, AudioDuration: time.Second, Duration: 2 * time.Second},
			},
		}},
		TotalDuration: 4 * time.Second,
	}, dir
}

func exportDemoBundle(tb testing.TB, script *scene.Script, dir string) int64 {
	tb.Helper()
	out := filepath.Join(dir, "bundle.html")
	if _, err := testExporter().Export(context.Background(), script, out); err != nil {
		tb.Fatalf("Export: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		tb.Fatal(err)
	}
	return info.Size()
}

// TestBundleSizeWithinTarget pins the bundle size so a regression is visible
// without running the benchmark.
func TestBundleSizeWithinTarget(t *testing.T) {
	script, dir := demoBundleScript(t)
	size := exportDemoBundle(t, script, dir)
	t.Logf("bundle size: %d bytes (target %d)", size, bundleSizeTarget)
	if size > bundleSizeTarget {
		t.Fatalf("bundle is %d bytes, over the %d-byte target", size, bundleSizeTarget)
	}
}

// BenchmarkDemoBundleSize records the bundle size and fails above the pinned
// target, so a regression shows up in a benchmark run too.
func BenchmarkDemoBundleSize(b *testing.B) {
	script, dir := demoBundleScript(b)
	for i := 0; i < b.N; i++ {
		if size := exportDemoBundle(b, script, dir); size > bundleSizeTarget {
			b.Fatalf("bundle is %d bytes, over the %d-byte target", size, bundleSizeTarget)
		}
	}
}
