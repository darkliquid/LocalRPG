//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

// packWorld builds a .lrpgpack from a world directory written outside the
// fixture, so importing it lands as a new world rather than clashing with one the
// fixture already has.
func packWorld(t *testing.T, dir, id, name string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "entities"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "world.yaml"),
		"id: "+id+"\nname: "+name+"\ndescription: A frontier the player imports.\ngenre: dark fantasy\n")
	writeFile(t, filepath.Join(dir, "entities", "saltmarch.md"),
		"---\nid: saltmarch\nname: Saltmarch\ntype: location\n---\n\nA port.\n")

	packPath := filepath.Join(t.TempDir(), id+".lrpgpack")
	file, err := os.Create(packPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := content.Pack(dir, "world", content.ManifestMeta{ID: id, Name: name}, file); err != nil {
		t.Fatalf("pack %s: %v", id, err)
	}
	return packPath
}

// TestImportingAPackagePutsItInTheStudio is TC-12 end to end: a package is chosen
// through the launcher's import input, the dialog reviews it, the import reports
// what it did, and the world it carried is in the studio afterwards. The picker
// itself cannot be driven, so the input is given the file the way a chooser would.
func TestImportingAPackagePutsItInTheStudio(t *testing.T) {
	f := NewFixture(t, "")
	packPath := packWorld(t, filepath.Join(t.TempDir(), "ashen-reach"), "ashen-reach", "Ashen Reach")

	b := f.Launch(t)
	b.Navigate("/")

	// The launcher has nothing to show until the import lands.
	if body := b.BodyText(); strings.Contains(body, "Ashen Reach") {
		t.Fatalf("the fixture already carries the world: %s", body)
	}

	b.SetFileInput(`//input[@aria-label="Choose a package to import"]`, packPath)

	// The dialog reviews what the package is before anything is written.
	b.WaitVisible(`//h3[normalize-space()='Import Content Package']`)
	body := b.BodyText()
	if !strings.Contains(body, "Ashen Reach") || !strings.Contains(body, "ashen-reach") {
		t.Fatalf("the dialog does not name the package: %s", body)
	}

	b.Click(`//button[normalize-space()='Install Package']`)

	// The dialog goes when the import is done, and the studio lists what it carried.
	b.WaitForGone("Review package contents and trust settings")
	b.Click(`//button[@aria-label="Worlds Studio"]`)
	b.WaitFor("Worlds Studio")
	b.WaitFor("Ashen Reach")
}

// TestGeneratingAWorldLandsItInTheStudio is the other half of TC-12: the work a
// generation does end to end, and the entry it leaves in the studio. With no model
// configured the built-in template generator answers, so the flow is deterministic
// and needs no provider. The review names itself after the draft it holds, which is
// why its landmark is the accepted count rather than a caption.
func TestGeneratingAWorldLandsItInTheStudio(t *testing.T) {
	f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: disabled\n")

	b := f.Launch(t)
	b.Navigate("/")
	b.Click(`//button[@aria-label="Worlds Studio"]`)
	b.WaitFor("Worlds Studio")

	b.Click(`//button[@title='Generate a world with AI']`)
	b.WaitFor("Generate a world")

	b.SetInputValue(`//textarea[@placeholder='A drowned kingdom where the tides never recede.']`,
		"A drowned kingdom where the tides never recede.")
	// The studio's own Generate button is earlier in the document and carries the
	// same word, so the click is scoped to the dialog.
	b.Click(`//div[@role='dialog']//button[contains(normalize-space(.), 'Generate')]`)

	// The review names itself after the draft, so its landmark is the accept count.
	b.WaitFor("accepted")
	body := b.BodyText()
	if !strings.Contains(body, "accepted") {
		t.Fatalf("the review does not report what it holds: %s", body)
	}

	b.Click(`//button[contains(normalize-space(.), 'Create world')]`)
	b.WaitFor("Worlds Studio")
	b.WaitFor("A drowned kingdom")
}
