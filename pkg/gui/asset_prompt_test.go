package gui

import (
	"strings"
	"testing"
)

func TestBuildAssetPrompt(t *testing.T) {
	tests := []struct {
		name        string
		kind        string
		entityName  string
		description string
		artStyle    string
		genre       string
		wantSubstrs []string
	}{
		{
			name:        "game banner",
			kind:        "banner",
			entityName:  "Shadow over Hollowmere",
			description: "The Sunken Realm",
			artStyle:    "Dark Gothic Oil Painting",
			genre:       "",
			wantSubstrs: []string{"Dark Gothic Oil Painting", "Shadow over Hollowmere", "The Sunken Realm", "widescreen cinematic"},
		},
		{
			name:        "game icon",
			kind:        "icon",
			entityName:  "Shadow over Hollowmere",
			description: "The Sunken Realm",
			artStyle:    "Pixel Art",
			genre:       "",
			wantSubstrs: []string{"Pixel Art", "Shadow over Hollowmere", "The Sunken Realm", "app icon emblem"},
		},
		{
			name:        "world banner",
			kind:        "banner",
			entityName:  "Ember Peak",
			description: "A scorched volcanic crater",
			artStyle:    "Watercolour",
			genre:       "High Fantasy",
			wantSubstrs: []string{"Watercolour", "Ember Peak", "High Fantasy", "panoramic"},
		},
		{
			name:        "world icon",
			kind:        "icon",
			entityName:  "Ember Peak",
			description: "A scorched volcanic crater",
			artStyle:    "Vector Minimalist",
			genre:       "High Fantasy",
			wantSubstrs: []string{"Vector Minimalist", "Ember Peak", "High Fantasy", "badge"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAssetPrompt(tt.kind, tt.entityName, tt.description, tt.artStyle, tt.genre)
			for _, sub := range tt.wantSubstrs {
				if !strings.Contains(got, sub) {
					t.Errorf("buildAssetPrompt() missing substring %q in prompt: %q", sub, got)
				}
			}
		})
	}
}
