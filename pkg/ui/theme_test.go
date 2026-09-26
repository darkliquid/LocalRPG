package ui

import "testing"

func TestDefaultPaletteChannelsAreInRange(t *testing.T) {
	p := DefaultPalette()
	colors := map[string][4]float32{
		"Bg": p.Bg, "Panel": p.Panel, "Text": p.Text, "Muted": p.Muted,
		"Border": p.Border, "Accent": p.Accent, "Danger": p.Danger,
	}
	for name, c := range colors {
		if c[0] < 0 || c[0] > 360 {
			t.Errorf("%s hue out of range: %v", name, c[0])
		}
		for i := 1; i < 3; i++ {
			if c[i] < 0 || c[i] > 100 {
				t.Errorf("%s channel %d out of range: %v", name, i, c[i])
			}
		}
		if c[3] < 0 || c[3] > 1 {
			t.Errorf("%s alpha out of range: %v", name, c[3])
		}
	}
}
