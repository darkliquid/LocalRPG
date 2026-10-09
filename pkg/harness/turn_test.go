package harness

import "testing"

func TestCheckRequestCarriesProfile(t *testing.T) {
	req, err := ParseCheckRequest(`{"actor":"x","check_kind":"do","profile":"pbta","position":"risky"}`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Profile != "pbta" || req.Position != "risky" {
		t.Fatalf("decoded %+v", req)
	}
}

func TestCheckRequestCarriesOpposed(t *testing.T) {
	req, err := ParseCheckRequest(`{"actor":"a","check_kind":"do","target":"ogre","opposed":"might"}`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Opposed != "might" || req.Target != "ogre" {
		t.Fatalf("request = %+v", req)
	}
}

func TestCheckResultCarriesTheOpponentsSide(t *testing.T) {
	res := CheckResult{OpposedRoll: &RollSummary{Notation: "2d6", Total: 7}, OpposedTotal: 7, OpposedActor: "ogre"}
	if res.OpposedRoll == nil || res.OpposedTotal != 7 || res.OpposedActor != "ogre" {
		t.Fatalf("result = %+v", res)
	}
}
