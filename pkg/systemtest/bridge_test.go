package systemtest

import "testing"

func TestBridgeRollIsDeterministic(t *testing.T) {
	b := NewBridge(nil, 42, SetupEntity{})
	a1, err := b.Roll("2d6")
	if err != nil {
		t.Fatal(err)
	}
	b2 := NewBridge(nil, 42, SetupEntity{})
	a2, err := b2.Roll("2d6")
	if err != nil {
		t.Fatal(err)
	}
	if a1.Total != a2.Total {
		t.Fatalf("same seed produced %d and %d", a1.Total, a2.Total)
	}
}

func TestBridgeRollCountsSuccesses(t *testing.T) {
	b := NewBridge(nil, 1, SetupEntity{})
	r, err := b.Roll("5d10>=8")
	if err != nil {
		t.Fatal(err)
	}
	if r.Successes < 0 || r.Successes > 5 {
		t.Fatalf("successes = %d", r.Successes)
	}
	if r.Total != r.Successes {
		t.Fatalf("total %d should equal the success count %d", r.Total, r.Successes)
	}
}

func TestBridgeReadsAndWritesState(t *testing.T) {
	b := NewBridge(nil, 1, SetupEntity{Stats: map[string]interface{}{"grit": 2}})
	if got, _ := b.GetStat(PlayerID, "grit"); toInt(got) != 2 {
		t.Fatalf("grit = %v, want 2", got)
	}
	if err := b.SetStat(PlayerID, "grit", 1); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.GetStat(PlayerID, "grit"); toInt(got) != 1 {
		t.Fatalf("grit = %v, want 1", got)
	}
}

func toInt(value interface{}) int {
	if n, ok := toFloat(value); ok {
		return int(n)
	}
	return -1
}
