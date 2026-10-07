package oracle

import (
	"math/rand"
	"strings"
	"testing"
)

func TestOpenerByTier(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	if opener(parts{Tier: "strong"}, rng) == "" || opener(parts{Tier: "miss"}, rng) == "" {
		t.Fatal("each tier should have an opener")
	}
	if opener(parts{Tier: ""}, rng) == "" {
		t.Fatal("neutral tier should have an opener")
	}
}

func TestConsequenceReferencesStakes(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	got := consequence(parts{Tier: "miss", Stakes: "the alarm sounds"}, rng)
	if !strings.Contains(got, "alarm") {
		t.Fatalf("consequence = %q", got)
	}
}

func TestHighStatSelectsConfidentVariants(t *testing.T) {
	rngHigh := rand.New(rand.NewSource(42))
	rngLow := rand.New(rand.NewSource(42))
	highParts := parts{
		Tier:  "strong",
		Stats: []statValue{{Name: "Edge", Value: 3}},
	}
	lowParts := parts{
		Tier:  "strong",
		Stats: []statValue{{Name: "Edge", Value: 0}},
	}
	highGot := opener(highParts, rngHigh)
	lowGot := opener(lowParts, rngLow)
	if highGot == "" || lowGot == "" {
		t.Fatal("openers should not be empty")
	}
	if highGot == lowGot {
		t.Fatalf("expected different opener variant sets for high stat vs normal/low stat, both got %q", highGot)
	}
}

func TestCastLineNamesAnEntity(t *testing.T) {
	got := castLine(parts{Entities: []string{"Garrick"}}, rand.New(rand.NewSource(1)))
	if !strings.Contains(got, "Garrick") {
		t.Fatalf("cast line = %q", got)
	}
}

func TestPlaceLineNamesTheLocation(t *testing.T) {
	got := placeLine(parts{Location: "Saltmarch"}, rand.New(rand.NewSource(1)))
	if !strings.Contains(got, "Saltmarch") {
		t.Fatalf("place line = %q", got)
	}
}

func TestEmptyPartsProduceNoLines(t *testing.T) {
	if castLine(parts{}, rand.New(rand.NewSource(1))) != "" || placeLine(parts{}, rand.New(rand.NewSource(1))) != "" {
		t.Fatal("empty parts should yield no lines")
	}
}
