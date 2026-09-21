package scene

import (
	"testing"
	"time"
)

func TestScriptFlattensScenesInOrder(t *testing.T) {
	script := Script{
		GameID: "campaign-01",
		Scenes: []Scene{
			{LocationID: "alden-tavern", LocationName: "Alden Tavern", Beats: []Beat{
				{Kind: BeatNarration, Text: "Warm light."},
				{Kind: BeatSpeech, Text: "Welcome.", Speaker: "Garrick"},
			}},
			{LocationID: "aldon-harbour", LocationName: "Aldon Harbour", Beats: []Beat{
				{Kind: BeatNarration, Text: "Salt air."},
			}},
		},
	}

	beats := script.Beats()
	if len(beats) != 3 {
		t.Fatalf("expected 3 beats, got %d", len(beats))
	}
	if beats[0].Text != "Warm light." || beats[2].Text != "Salt air." {
		t.Errorf("beats are out of order: %+v", beats)
	}
}

func TestSceneCardCarriesTheLocationName(t *testing.T) {
	card := SceneCard(Scene{LocationID: "alden-tavern", LocationName: "Alden Tavern", ArtPath: "/tmp/tavern.svg"})

	if card.Kind != BeatSceneCard {
		t.Errorf("Kind = %q, want scene_card", card.Kind)
	}
	if card.Text != "Alden Tavern" || card.ArtPath != "/tmp/tavern.svg" {
		t.Errorf("unexpected card: %+v", card)
	}
	if card.Duration < MinimumBeatDuration {
		t.Errorf("Duration = %v, want at least the floor", card.Duration)
	}
}

func TestSceneCardFallsBackWhenANameIsMissing(t *testing.T) {
	card := SceneCard(Scene{LocationID: "opening-scene"})

	if card.Text == "" {
		t.Errorf("expected a placeholder name for an unnamed location")
	}
	if card.Duration < MinimumBeatDuration {
		t.Errorf("Duration = %v, want at least the floor", card.Duration)
	}
}

func TestBeatsForAnEmptyScript(t *testing.T) {
	script := Script{GameID: "campaign-01"}
	if got := script.Beats(); len(got) != 0 {
		t.Errorf("expected no beats, got %+v", got)
	}
	if script.TotalDuration != time.Duration(0) {
		t.Errorf("expected no duration, got %v", script.TotalDuration)
	}
}
