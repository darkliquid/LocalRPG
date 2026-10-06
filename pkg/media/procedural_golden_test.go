package media

import (
	"bytes"
	"testing"
)

func TestProceduralGoldenMatrix(t *testing.T) {
	genres := []string{"fantasy", "cyberpunk", "horror", "scifi", "wildwest", "unknown"}
	times := []string{"dawn", "day", "dusk", "night"}
	weathers := []string{"clear", "rain", "fog", "snow"}
	for _, g := range genres {
		for _, tod := range times {
			for _, wx := range weathers {
				svg := GenerateSceneSVG(SceneRequest{Genre: g, TimeOfDay: tod, Weather: wx, Seed: "golden"})
				if len(svg) == 0 || !bytes.Contains(svg, []byte("<svg")) {
					t.Fatalf("%s/%s/%s produced no SVG", g, tod, wx)
				}
			}
		}
	}
}
