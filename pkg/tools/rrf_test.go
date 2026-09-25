package tools

import (
	"testing"
)

func TestReciprocalRankFusion(t *testing.T) {
	// FTS hits: A (rank 1), B (rank 2), C (rank 3)
	ftsIDs := []string{"docA", "docB", "docC"}
	// Vector hits: C (rank 1), B (rank 2), D (rank 3)
	vecIDs := []string{"docC", "docB", "docD"}

	fused := FuseRankings(ftsIDs, vecIDs, 10)
	if len(fused) != 4 {
		t.Fatalf("expected 4 unique items fused, got %d", len(fused))
	}

	// docB appears at rank 2 in both, docC at rank 3 and rank 1.
	// docC score: 0.5/(60+3) + 0.5/(60+1) = 0.5/63 + 0.5/61 = 0.007936 + 0.008196 = 0.016132
	// docB score: 0.5/(60+2) + 0.5/(60+2) = 0.5/62 + 0.5/62 = 0.016129
	// docA score: 0.5/(60+1) = 0.008196
	// docD score: 0.5/(60+3) = 0.007936
	if fused[0] != "docC" {
		t.Errorf("expected top docC, got %s", fused[0])
	}
	if fused[1] != "docB" {
		t.Errorf("expected second docB, got %s", fused[1])
	}
	if fused[2] != "docA" {
		t.Errorf("expected third docA, got %s", fused[2])
	}
	if fused[3] != "docD" {
		t.Errorf("expected fourth docD, got %s", fused[3])
	}
}

func TestReciprocalRankFusionEmptyLists(t *testing.T) {
	// Only FTS hits
	fusedFTS := FuseRankings([]string{"a", "b"}, nil, 5)
	if len(fusedFTS) != 2 || fusedFTS[0] != "a" || fusedFTS[1] != "b" {
		t.Errorf("unexpected FTS-only fusion: %+v", fusedFTS)
	}

	// Only Vec hits
	fusedVec := FuseRankings(nil, []string{"x", "y"}, 5)
	if len(fusedVec) != 2 || fusedVec[0] != "x" || fusedVec[1] != "y" {
		t.Errorf("unexpected Vec-only fusion: %+v", fusedVec)
	}

	// Empty
	fusedEmpty := FuseRankings(nil, nil, 5)
	if len(fusedEmpty) != 0 {
		t.Errorf("expected empty fusion, got %+v", fusedEmpty)
	}
}
