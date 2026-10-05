package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// TestRestartGamePreservesPlayerMetadata covers the regression where resetting a
// campaign rebuilt the protagonist note from name and prose alone, dropping the
// character sheet fields the player had entered.
func TestRestartGamePreservesPlayerMetadata(t *testing.T) {
	gameID, svc := turnFixture(t)
	paths := svc.GetResolver()
	gameDir := paths.GameDir(gameID)

	note := `---
id: sean
name: Sean
type: character
appearance: A weathered sailor with a storm-grey coat.
age: "34"
gender: male
voice:
  provider: native-os
  voice_id: en-us-1
  pitch: 0.95
portrait: assets/portraits/sean.png
pronouns: he/him
title: Captain
---
Sean grew up on the docks and never quite left them.`
	playerPath := filepath.Join(gameDir, "entities", "sean.md")
	if err := os.WriteFile(playerPath, []byte(note), 0644); err != nil {
		t.Fatal(err)
	}

	portraitDir := filepath.Join(gameDir, "assets", "portraits")
	if err := os.MkdirAll(portraitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(portraitDir, "sean.png"), []byte("portrait-bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	// Campaign configuration a restart must not disturb: artwork, the narrator
	// voice, and the spend ledger.
	assetsDir := filepath.Join(gameDir, "assets")
	for name, body := range map[string]string{"banner.png": "banner-bytes", "icon.png": "icon-bytes"} {
		if err := os.WriteFile(filepath.Join(assetsDir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.UpdateGameSettings(context.Background(), gameID, map[string]interface{}{"narrator_voice": "narrator-01"}); err != nil {
		t.Fatal(err)
	}
	store, err := svc.store(gameID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUsage(storage.UsageRecord{GameID: gameID, Role: "gm", Provider: "echo", Requests: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTTSJob(storage.TTSJob{ID: "job-1", GameID: gameID, Provider: "echo", Status: "processing"}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.RestartGame(context.Background(), gameID); err != nil {
		t.Fatalf("RestartGame failed: %v", err)
	}

	data, err := os.ReadFile(playerPath)
	if err != nil {
		t.Fatalf("read player note after restart: %v", err)
	}
	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("parse player note after restart: %v", err)
	}

	if ent.Name != "Sean" {
		t.Errorf("Name = %q, want Sean", ent.Name)
	}
	if ent.Age != "34" {
		t.Errorf("Age = %q, want 34", ent.Age)
	}
	if ent.Gender != "male" {
		t.Errorf("Gender = %q, want male", ent.Gender)
	}
	if ent.Appearance != "A weathered sailor with a storm-grey coat." {
		t.Errorf("Appearance = %q, want the original description", ent.Appearance)
	}
	if ent.Voice == nil || ent.Voice.VoiceID != "en-us-1" || ent.Voice.Pitch != 0.95 {
		t.Errorf("Voice = %+v, want the original voice config", ent.Voice)
	}
	if got, _ := ent.ExtraMeta["pronouns"].(string); got != "he/him" {
		t.Errorf("pronouns = %v, want he/him", ent.ExtraMeta["pronouns"])
	}
	if got, _ := ent.ExtraMeta["title"].(string); got != "Captain" {
		t.Errorf("title = %v, want Captain", ent.ExtraMeta["title"])
	}
	if ent.Portrait != "assets/portraits/sean.png" {
		t.Errorf("Portrait = %q, want the original reference", ent.Portrait)
	}
	if string(ent.Body) == "" {
		t.Errorf("Body is empty, want the character prose preserved")
	}

	portrait, err := os.ReadFile(filepath.Join(portraitDir, "sean.png"))
	if err != nil {
		t.Fatalf("portrait asset missing after restart: %v", err)
	}
	if string(portrait) != "portrait-bytes" {
		t.Errorf("portrait asset = %q, want the original bytes", portrait)
	}

	for name, want := range map[string]string{"banner.png": "banner-bytes", "icon.png": "icon-bytes"} {
		got, err := os.ReadFile(filepath.Join(assetsDir, name))
		if err != nil {
			t.Fatalf("read asset %s after restart: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("asset %s = %q, want %q", name, got, want)
		}
	}

	manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
	if err != nil {
		t.Fatalf("reload manifest: %v", err)
	}
	if got, _ := manifest.Settings["narrator_voice"].(string); got != "narrator-01" {
		t.Errorf("narrator_voice = %q, want the setting preserved", got)
	}

	usage, err := store.UsageByGame(gameID)
	if err != nil {
		t.Fatalf("UsageByGame: %v", err)
	}
	if len(usage) != 1 {
		t.Errorf("usage rows = %d, want the ledger preserved", len(usage))
	}

	jobs, err := store.ListTTSJobs(gameID)
	if err != nil {
		t.Fatalf("ListTTSJobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("batch jobs = %d, want them removed with the narration they speak", len(jobs))
	}
}
