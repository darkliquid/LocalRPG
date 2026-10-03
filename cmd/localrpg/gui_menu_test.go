package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestDefaultApplicationMenuIsConfigured(t *testing.T) {
	menu := application.DefaultApplicationMenu()
	if menu == nil {
		t.Fatal("expected non-nil default application menu")
	}
}
