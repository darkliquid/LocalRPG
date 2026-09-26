package desktop

import "testing"

func TestWorldPayloadSplitsTags(t *testing.T) {
	appState = &State{FormWorldName: "The Sundered Realm", FormWorldTags: "fantasy, dark , ,low-magic"}
	req := worldPayload()
	if len(req.Tags) != 3 {
		t.Fatalf("tags = %#v", req.Tags)
	}
	if req.Tags[1] != "dark" {
		t.Fatalf("tags trimmed wrong: %#v", req.Tags)
	}
	if req.ID != "" {
		t.Fatalf("draft must omit an ID, got %q", req.ID)
	}
}
