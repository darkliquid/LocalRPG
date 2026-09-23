package media

import (
	"reflect"
	"testing"
)

func TestNormaliseVoiceTags(t *testing.T) {
	cases := []struct {
		name string
		raw  []string
		want []string
	}{
		{"lowercases and trims", []string{" Elder ", "MALE"}, []string{"elder", "male"}},
		{"synonyms collapse", []string{"Masculine", "UK"}, []string{"male", "british"}},
		{"middle aged is hyphenated", []string{"middle aged"}, []string{"middle-aged"}},
		{"duplicates collapse", []string{"male", "Male", "male"}, []string{"male"}},
		{"empties drop", []string{"", "  ", "eerie"}, []string{"eerie"}},
		{"no tags", nil, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormaliseVoiceTags(tc.raw...)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NormaliseVoiceTags(%v) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
