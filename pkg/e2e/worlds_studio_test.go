//go:build e2e

package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestDeletingAWorldEntityKeepsTheStudioUsable drives the real studio: it opens a
// world with two templates, deletes one, and asserts the app is still rendered. A
// React error here unmounts the tree and leaves a blank page, which is what the
// report described.
func TestDeletingAWorldEntityKeepsTheStudioUsable(t *testing.T) {
	f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")
	f.WriteWorld(t, "ember-peak", "Ember Peak", nil)
	dir := filepath.Join(f.Service.GetResolver().WorldDir("ember-peak"), "entities")
	writeFile(t, filepath.Join(dir, "saltmarch.md"),
		"---\nid: saltmarch\nname: Saltmarch\ntype: location\n---\n\nA port.\n")
	writeFile(t, filepath.Join(dir, "the-tidewatch.md"),
		"---\nid: the-tidewatch\nname: The Tidewatch\ntype: faction\n---\n\nA crew.\n")

	b := f.Launch(t)
	b.Navigate("/")
	b.Click(`//button[@aria-label="Worlds Studio"]`)
	b.WaitFor("Worlds Studio")

	b.Click(`//h4[normalize-space()='Ember Peak']`)
	b.WaitFor("Starter Entities (2)")

	b.Click(`//button[.//span[contains(normalize-space(), 'Starter Entities')]]`)
	b.WaitFor("saltmarch.md")

	b.Click(`//button[@title='Delete entity template']`)
	b.WaitFor("Starter Entities (1)")

	// The studio is still mounted, and the surviving template is selectable.
	body := b.BodyText()
	if !strings.Contains(body, "The Tidewatch") {
		t.Fatalf("the studio blanked after deleting an entity:\n%s", body)
	}
	for _, entry := range b.Console() {
		if strings.Contains(entry, "Uncaught") || strings.Contains(entry, "Minified React error") {
			t.Fatalf("the browser reported an error after deletion: %s", entry)
		}
	}
}

// TestDeletingTheLastWorldEntityKeepsTheStudioUsable covers the other shape of
// the same bug: the delete empties the list, so the tree and the editor both
// unmount at once.
func TestDeletingTheLastWorldEntityKeepsTheStudioUsable(t *testing.T) {
	f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")
	f.WriteWorld(t, "ember-peak", "Ember Peak", nil)
	dir := filepath.Join(f.Service.GetResolver().WorldDir("ember-peak"), "entities")
	writeFile(t, filepath.Join(dir, "saltmarch.md"),
		"---\nid: saltmarch\nname: Saltmarch\ntype: location\n---\n\nA port.\n")

	b := f.Launch(t)
	b.Navigate("/")
	b.Click(`//button[@aria-label="Worlds Studio"]`)
	b.WaitFor("Worlds Studio")

	b.Click(`//h4[normalize-space()='Ember Peak']`)
	b.WaitFor("Starter Entities (1)")

	b.Click(`//button[.//span[contains(normalize-space(), 'Starter Entities')]]`)
	b.WaitFor("saltmarch.md")

	b.Click(`//button[@title='Delete entity template']`)
	b.WaitFor("Starter Entities (0)")
	b.WaitFor("No starter templates")

	for _, entry := range b.Console() {
		if strings.Contains(entry, "Uncaught") || strings.Contains(entry, "Minified React error") {
			t.Fatalf("the browser reported an error after deletion: %s", entry)
		}
	}
}
