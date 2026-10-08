package worldgen

import "testing"

func TestEstimatePlan(t *testing.T) {
	if got := EstimatePlan("world", Brief{Counts: Counts{Locations: 2, Factions: 2, Characters: 3}}, 0, nil); got.Calls != 4 {
		t.Fatalf("world calls = %d, want 4", got.Calls)
	}
	if got := EstimatePlan("entities", Brief{Counts: Counts{Characters: 25}}, 0, nil); got.Calls < 2 {
		t.Fatalf("a large batch should span calls: %d", got.Calls)
	}
	if got := EstimatePlan("ingest", Brief{}, 10, nil); got.Calls < 10 {
		t.Fatalf("ingest calls = %d", got.Calls)
	}
}

func TestEstimatePlanPricesWhenATableIsGiven(t *testing.T) {
	table := func(calls int) (int64, bool) { return int64(calls) * 1000, true }
	got := EstimatePlan("world", Brief{Premise: "x"}, 0, table)
	if !got.Priced || got.CostMicros != int64(got.Calls)*1000 {
		t.Fatalf("estimate = %+v", got)
	}
}

func TestEstimatePlanIsUnpricedByDefault(t *testing.T) {
	got := EstimatePlan("world", Brief{Premise: "x"}, 0, nil)
	if got.Priced || got.CostMicros != 0 {
		t.Fatalf("estimate = %+v, want unpriced", got)
	}
}

func TestEstimatePlanWorldSpansBatches(t *testing.T) {
	// 25 characters needs three calls, two more than the single characters step.
	got := EstimatePlan("world", Brief{Counts: Counts{Characters: 25}}, 0, nil)
	if got.Calls != 6 {
		t.Fatalf("calls = %d, want 6", got.Calls)
	}
}
