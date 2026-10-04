package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The native menu is macOS-only, and it deliberately omits the Help menu the
// Wails default ships: its sole entry opens wails.io, which tells the user
// nothing about LocalRPG. The application's own Help and About live in the
// frontend menu instead.
func TestLocalRPGApplicationMenuOmitsWailsHelp(t *testing.T) {
	menu := localRPGApplicationMenu()
	if menu == nil {
		t.Fatal("expected non-nil application menu")
	}
	if item := menu.FindByRole(application.HelpMenu); item != nil {
		t.Errorf("application menu should not carry a Help menu, got %q", item.Label())
	}
	if item := menu.FindByLabel("Help"); item != nil {
		t.Errorf("application menu should not carry a Help menu, got %q", item.Label())
	}
	// The AppMenu role is macOS-only (NewAppMenu returns nil elsewhere), so the
	// menu cannot be asserted to carry it. It must still carry the standard
	// File/Edit/View/Window roles.
	if menu.ItemAt(0) == nil {
		t.Error("expected the application menu to carry at least one item")
	}
	if item := menu.FindByLabel("Edit"); item == nil {
		t.Error("expected the standard Edit menu to be present")
	}
}
