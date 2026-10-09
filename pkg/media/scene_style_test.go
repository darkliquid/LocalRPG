package media

import (
	"strings"
	"testing"
)

func TestSceneStyleIsStable(t *testing.T) {
	a := NewSceneStyle("saltmarch", "grim fantasy", "A salt-crusted port.")
	b := NewSceneStyle("saltmarch", "grim fantasy", "A salt-crusted port.")
	if a != b {
		t.Fatalf("style differs: %+v %+v", a, b)
	}
	if a.Clause() == "" {
		t.Fatal("a scene style should name a palette and a lighting")
	}
}

func TestSceneStyleChangesWithScene(t *testing.T) {
	if NewSceneStyle("a", "s", "").Seed == NewSceneStyle("b", "s", "").Seed {
		t.Fatal("a different scene should change the seed")
	}
	if NewSceneStyle("a", "s", "").Seed == NewSceneStyle("a", "other", "").Seed {
		t.Fatal("a different world style should change the seed")
	}
}

func TestSceneStyleClauseNamesTheLook(t *testing.T) {
	style := NewSceneStyle("hall", "oil painting", "A vaulted stone hall.")
	clause := style.Clause()
	for _, want := range []string{"palette:", "lighting:", "A vaulted stone hall."} {
		if !strings.Contains(clause, want) {
			t.Fatalf("clause omits %q: %s", want, clause)
		}
	}
	if NewSceneStyle("", "", "").Empty() != true {
		t.Fatal("an empty style should report itself empty")
	}
}

// TestSceneStyleVariesAcrossScenes guards that two scenes do not always share a
// look: the palette or the lighting must differ for at least some pairs.
func TestSceneStyleVariesAcrossScenes(t *testing.T) {
	seen := make(map[string]bool, 8)
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		style := NewSceneStyle(id, "fantasy", "")
		seen[style.Palette+"|"+style.Lighting] = true
	}
	if len(seen) < 2 {
		t.Fatalf("every scene resolved to the same look: %v", seen)
	}
}
