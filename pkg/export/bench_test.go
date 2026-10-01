package export

import (
	"context"
	"path/filepath"
	"testing"
)

// BenchmarkRenderVideo1080p measures the whole path: composite, encode, mux. The
// small script is four seconds of video, so the reported time is for roughly
// sixty 1080p frames.
func BenchmarkRenderVideo1080p(b *testing.B) {
	pipeline := NewVideoPipeline(".")
	pipeline.SetSize(1920, 1080)
	pipeline.SetFPS(15)
	script := smallScript()

	out := filepath.Join(b.TempDir(), "replay.webm")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := pipeline.RenderVideo(context.Background(), script, out); err != nil {
			b.Fatal(err)
		}
	}
}
