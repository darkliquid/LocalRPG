package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestRosterResolvesNamesSlugsAndAliases(t *testing.T) {
	r := &roster{byKey: map[string]string{}}
	r.Declare("Lady Evelyn Vance", "lady-evelyn-vance")
	r.Declare("player", "sean")

	if id, ok := r.Resolve("Lady Evelyn Vance"); !ok || id != "lady-evelyn-vance" {
		t.Fatalf("name resolve = %q, %v", id, ok)
	}
	if id, ok := r.Resolve("lady evelyn vance"); !ok || id != "lady-evelyn-vance" {
		t.Fatalf("case-insensitive resolve = %q, %v", id, ok)
	}
	if id, ok := r.Resolve("lady-evelyn-vance"); !ok || id != "lady-evelyn-vance" {
		t.Fatalf("slug resolve = %q, %v", id, ok)
	}
	if id, ok := r.Resolve("Evelyn"); ok {
		t.Fatalf("a partial name must not resolve, got %q", id)
	}
}

func TestRosterVoiceIsNilWithoutAStore(t *testing.T) {
	r := &roster{byKey: map[string]string{}}
	if voice := r.Voice("sean"); voice != nil {
		t.Fatalf("Voice without a store = %#v, want nil", voice)
	}
	var nilRoster *roster
	if _, ok := nilRoster.Resolve("anything"); ok {
		t.Fatal("a nil roster must resolve nothing")
	}
	if voice := nilRoster.Voice("anything"); voice != nil {
		t.Fatalf("nil roster voice = %#v", voice)
	}
	_ = entity.VoiceConfig{}
}

func TestRosterResolvesDeclaredPersonaVoiceWithGender(t *testing.T) {
	profiles := []config.VoiceProfile{
		{ID: "af_female", VoiceID: "af_female", Tags: []string{"female"}},
		{ID: "am_male", VoiceID: "am_male", Tags: []string{"male"}},
	}
	r := newRoster(nil, "", "", profiles)
	r.DeclarePersona("kaelen", harness.PersonaDecl{
		Name:   "Kaelen",
		Type:   "character",
		Gender: "male",
	})
	voice := r.Voice("kaelen")
	if voice == nil || voice.VoiceID != "am_male" {
		t.Fatalf("expected am_male for declared male persona, got %#v", voice)
	}
}
