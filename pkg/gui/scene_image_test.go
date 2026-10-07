package gui

import (
	"context"
	"testing"
)

func TestGenerateTurnSceneImageRequiresAProvider(t *testing.T) {
	gameID, svc := setupTestGame(t)
	if err := svc.GenerateTurnSceneImage(context.Background(), gameID, 1); err == nil {
		t.Fatal("image generation should be unavailable with no provider configured")
	}
}

func TestGenerateTurnSceneImageRejectsInvalidGame(t *testing.T) {
	_, svc := setupTestGame(t)
	if err := svc.GenerateTurnSceneImage(context.Background(), "bad/id", 1); err == nil {
		t.Fatal("an invalid game id should error")
	}
}
