package desktop

import "testing"

func TestSystemPayloadDefaultsSlugOnDraft(t *testing.T) {
	appState = &State{FormSysName: "Blades in the Dark", FormSysSlug: "", SystemSaved: false}
	req := systemPayload()
	if req.Name != "Blades in the Dark" {
		t.Fatalf("name = %q", req.Name)
	}
	if req.ID != "" {
		t.Fatalf("draft must omit an explicit ID, got %q", req.ID)
	}
}

func TestSystemPayloadKeepsSavedSlug(t *testing.T) {
	appState = &State{FormSysName: "Blades in the Dark", FormSysSlug: "blades", SystemSaved: true}
	req := systemPayload()
	if req.ID != "blades" {
		t.Fatalf("id = %q, want blades", req.ID)
	}
}
