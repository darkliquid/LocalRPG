package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

type stubRouter struct {
	response string
}

func (s *stubRouter) GenerateForRole(ctx context.Context, role string, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: s.response}, nil
}

func TestCharacterEnricher_EnrichesMissingFields(t *testing.T) {
	router := &stubRouter{
		response: `{"gender":"female","age":"29","pronouns":"she/her","appearance":"A sharp-eyed scout with braided auburn hair and leather flight gear."}`,
	}
	enricher := NewCharacterEnricher(router)

	ent := &entity.Entity{
		ID:   "mara-jade",
		Name: "Mara Jade",
		Type: "character",
		Body: "A smuggler spotted in the lower cantina.",
	}

	enriched, err := enricher.Enrich(context.Background(), ent, "Space Opera Sci-Fi")
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if enriched.Gender != "female" {
		t.Errorf("Gender = %q, want female", enriched.Gender)
	}
	if enriched.Age != "29" {
		t.Errorf("Age = %q, want 29", enriched.Age)
	}
	if !strings.Contains(enriched.Appearance, "sharp-eyed scout") {
		t.Errorf("Appearance = %q, want generated appearance", enriched.Appearance)
	}
}

func TestCharacterEnricher_SkipsWhenAlreadyComplete(t *testing.T) {
	router := &stubRouter{
		response: `{"gender":"other","age":"99","appearance":"Different appearance"}`,
	}
	enricher := NewCharacterEnricher(router)

	ent := &entity.Entity{
		ID:         "mara-jade",
		Name:       "Mara Jade",
		Type:       "character",
		Gender:     "female",
		Age:        "29",
		Appearance: "Original appearance",
	}

	enriched, err := enricher.Enrich(context.Background(), ent, "Space Opera Sci-Fi")
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if enriched.Appearance != "Original appearance" {
		t.Errorf("Appearance modified unexpectedly: %q", enriched.Appearance)
	}
}
