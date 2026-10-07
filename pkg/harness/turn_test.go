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
