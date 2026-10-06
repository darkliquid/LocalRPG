package refsystems

import "testing"

func TestListHasTheThreeSystems(t *testing.T) {
	got := List()
	ids := map[string]bool{}
	for _, s := range got {
		ids[s.ID] = true
		if s.Script == "" || s.RulesPrompt == "" || s.Mechanics == nil {
			t.Errorf("%s is incomplete", s.ID)
		}
	}
	for _, want := range []string{"narrative_2d6", "d20_dc", "dice_pool"} {
		if !ids[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestEverySystemMechanicsValidate(t *testing.T) {
	for _, s := range List() {
		if problems := s.Mechanics.Checks.Validate(); len(problems) > 0 {
			t.Errorf("%s: %v", s.ID, problems)
		}
	}
}

func TestGet(t *testing.T) {
	if _, ok := Get("d20_dc"); !ok {
		t.Error("d20_dc not found")
	}
	if _, ok := Get("nope"); ok {
		t.Error("an unknown id should not be found")
	}
}
