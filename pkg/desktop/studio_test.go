package desktop

import "testing"

func TestApplySelectionKeepsDirtyDraft(t *testing.T) {
	appState = &State{Studio: Selection{Kind: "draft"}, StudioDirty: true}
	applySelection(Selection{Kind: "saved", ID: "existing"}, func() bool { return true })
	if appState.Studio.Kind != "draft" || !appState.ConfirmDiscard {
		t.Fatalf("studio = %+v confirm = %v", appState.Studio, appState.ConfirmDiscard)
	}
}

func TestApplySelectionSwitchesWhenClean(t *testing.T) {
	appState = &State{Studio: Selection{Kind: "draft"}, StudioDirty: false}
	applySelection(Selection{Kind: "saved", ID: "existing"}, func() bool { return false })
	if appState.Studio.ID != "existing" {
		t.Fatalf("studio = %+v", appState.Studio)
	}
}
