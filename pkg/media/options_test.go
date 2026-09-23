package media

import (
	"reflect"
	"testing"
)

func optionSchema() []VoiceOption {
	return []VoiceOption{
		{Key: "stability", Label: "Stability", Kind: "float", Min: 0, Max: 1, Step: 0.05},
		{Key: "seed", Label: "Seed", Kind: "int", Min: 0, Max: 1000},
		{Key: "use_speaker_boost", Label: "Speaker boost", Kind: "bool"},
		{Key: "model", Label: "Model", Kind: "enum", Options: []string{"a", "b"}},
		{Key: "note", Label: "Note", Kind: "string"},
	}
}

func TestValidateVoiceOptions(t *testing.T) {
	t.Run("clamps a float to the declared range", func(t *testing.T) {
		got, warnings := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"stability": 1.7})
		if len(warnings) != 0 {
			t.Fatalf("warnings = %v", warnings)
		}
		if got["stability"] != 1.0 {
			t.Errorf("stability = %v, want 1.0", got["stability"])
		}
	})

	t.Run("rounds an int", func(t *testing.T) {
		got, _ := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"seed": 12.6})
		if got["seed"] != 13 {
			t.Errorf("seed = %v (%T), want 13", got["seed"], got["seed"])
		}
	})

	t.Run("accepts a bool and a string", func(t *testing.T) {
		got, _ := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"use_speaker_boost": true, "note": "warm"})
		if got["use_speaker_boost"] != true || got["note"] != "warm" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("rejects an enum value outside the list", func(t *testing.T) {
		got, warnings := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"model": "c"})
		if len(warnings) != 1 || got != nil {
			t.Errorf("got %v, warnings %v", got, warnings)
		}
	})

	t.Run("drops an unknown key with a warning", func(t *testing.T) {
		got, warnings := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"banana": 1})
		if got != nil || len(warnings) != 1 {
			t.Errorf("got %v, warnings %v", got, warnings)
		}
	})

	t.Run("an empty value map is omitted", func(t *testing.T) {
		got, warnings := ValidateVoiceOptions(optionSchema(), map[string]interface{}{})
		if got != nil || len(warnings) != 0 {
			t.Errorf("got %v, warnings %v", got, warnings)
		}
	})

	t.Run("keeps one good value and warns about the bad one", func(t *testing.T) {
		got, warnings := ValidateVoiceOptions(optionSchema(), map[string]interface{}{"stability": 0.4, "seed": "abc"})
		if len(warnings) != 1 {
			t.Errorf("warnings = %v", warnings)
		}
		want := map[string]interface{}{"stability": 0.4}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}
