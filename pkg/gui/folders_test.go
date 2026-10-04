package gui

import (
	"errors"
	"testing"
)

func TestValidateFolderPath(t *testing.T) {
	valid := map[string]string{
		"":                        "",
		"/":                       "",
		"factions":                "factions",
		"factions/orders":         "factions/orders",
		"Guilds & Orders":         "Guilds & Orders",
		"  factions/orders  ":     "factions/orders",
		"places/port-vel/taverns": "places/port-vel/taverns",
	}
	for input, want := range valid {
		got, err := ValidateFolderPath(input)
		if err != nil {
			t.Errorf("ValidateFolderPath(%q) errored: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ValidateFolderPath(%q) = %q, want %q", input, got, want)
		}
	}

	invalid := []string{
		"../escape",
		"factions/../../etc",
		"/absolute",
		"factions\\orders",
		"factions//orders",
		"factions/",
		".hidden",
		"factions/.hidden",
		"factions/./orders",
	}
	for _, input := range invalid {
		if _, err := ValidateFolderPath(input); !errors.Is(err, ErrInvalidFolderPath) {
			t.Errorf("ValidateFolderPath(%q) error = %v, want ErrInvalidFolderPath", input, err)
		}
	}

	long := ""
	for i := 0; i < 65; i++ {
		long += "a"
	}
	if _, err := ValidateFolderPath(long); !errors.Is(err, ErrInvalidFolderPath) {
		t.Errorf("an over-long segment must be rejected, got %v", err)
	}
}

func TestBuildFolderTree(t *testing.T) {
	tree := BuildFolderTree([]string{"factions/orders", "factions", "places", "factions/orders/inner"})
	if len(tree) != 2 {
		t.Fatalf("root children = %d, want 2", len(tree))
	}
	if tree[0].Path != "factions" || tree[1].Path != "places" {
		t.Fatalf("children not sorted: %q, %q", tree[0].Path, tree[1].Path)
	}
	if tree[0].Name != "factions" {
		t.Errorf("Name = %q, want the directory name", tree[0].Name)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].Path != "factions/orders" {
		t.Fatalf("factions children = %+v, want one orders child", tree[0].Children)
	}
	if len(tree[0].Children[0].Children) != 1 {
		t.Fatalf("orders children = %+v, want one inner child", tree[0].Children[0].Children)
	}
}

func TestBuildFolderTreeInventsMissingParents(t *testing.T) {
	// Only the deepest path is listed, so the parents have to be created to hold it.
	tree := BuildFolderTree([]string{"places/port-vel/taverns"})
	if len(tree) != 1 || tree[0].Path != "places" {
		t.Fatalf("tree = %+v, want a single places root", tree)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].Path != "places/port-vel" {
		t.Fatalf("places children = %+v, want places/port-vel", tree[0].Children)
	}
}

func TestBuildFolderTreeIgnoresTheRoot(t *testing.T) {
	if tree := BuildFolderTree([]string{""}); len(tree) != 0 {
		t.Fatalf("tree = %+v, want the root to contribute no node", tree)
	}
}
