package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/media"
)

// TestPromptPrefixIsStable guards the consistency mechanism: two turns in one
// scene share the stable prefix, and the moment still differs.
func TestPromptPrefixIsStable(t *testing.T) {
	style := media.NewSceneStyle("hall", "oil painting", "A vaulted stone hall.")
	base := ScenePromptContext{
		Location:   "The Hall",
		Appearance: "A vaulted stone hall.",
		Style:      "oil painting",
		Scene:      style,
	}

	climb := base
	climb.Action = "climb the wall"
	climb.Cue = "You reach for the ledge."
	wait := base
	wait.Action = "wait in the shadows"
	wait.Cue = "Silence settles."

	a := BuildScenePrompt(climb)
	b := BuildScenePrompt(wait)
	if ScenePromptPrefix(climb) != ScenePromptPrefix(wait) {
		t.Fatalf("the prefix should be identical for one scene:\n%s\n%s", ScenePromptPrefix(climb), ScenePromptPrefix(wait))
	}
	if a == b {
		t.Fatal("the moment should still change the prompt")
	}
	if !strings.Contains(a, "climb the wall") || !strings.Contains(b, "wait in the shadows") {
		t.Fatal("the action should be in the prompt")
	}
}

// TestPromptWithoutASceneIsUnchanged guards that a caller with no scene style gets
// exactly the prompt it got before scene consistency existed.
func TestPromptWithoutASceneIsUnchanged(t *testing.T) {
	ctx := ScenePromptContext{
		Cue:       "A door opens.",
		Action:    "step through",
		Location:  "The Hall",
		Style:     "oil painting",
		Cast:      []SceneCastMember{{Name: "Kaelen"}},
		Outcome:   "strong",
		Narration: "The door swings wide.",
	}
	withEmpty := BuildScenePrompt(ctx)
	if strings.Contains(withEmpty, "palette:") {
		t.Fatalf("an empty scene style should add no clause: %s", withEmpty)
	}
	if ScenePromptPrefix(ctx) != "location: The Hall, oil painting" {
		t.Fatalf("prefix = %q", ScenePromptPrefix(ctx))
	}
}

// conditionedGen records whether the conditioner path was taken.
type conditionedGen struct {
	plainCalls       int
	conditionedCalls int
	reference        []byte
}

func (g *conditionedGen) GenerateImage(context.Context, string) ([]byte, error) {
	g.plainCalls++
	return []byte("plain"), nil
}

func (g *conditionedGen) GenerateSceneWithReference(_ context.Context, _ media.SceneRequest, reference []byte) ([]byte, error) {
	g.conditionedCalls++
	g.reference = reference
	return []byte("conditioned"), nil
}

func TestConditionerIsUsedWhenAvailable(t *testing.T) {
	dir := t.TempDir()
	gen := &conditionedGen{}
	worker := NewSceneWorker(core.NewPathResolver(dir), gen)
	written := waitForScene(t, worker, "campaign", 2, SceneJob{Prompt: "a hall", Reference: []byte("previous")})

	if gen.conditionedCalls != 1 || gen.plainCalls != 0 {
		t.Fatalf("calls = %+v, want the conditioned path", gen)
	}
	if string(gen.reference) != "previous" {
		t.Fatalf("reference = %q", gen.reference)
	}
	if _, err := os.Stat(filepath.Join(dir, "games", "campaign", written)); err != nil {
		t.Fatalf("the conditioned image was not written: %v", err)
	}
}

func TestPlainGeneratorIsUsedWithoutAReference(t *testing.T) {
	dir := t.TempDir()
	gen := &conditionedGen{}
	worker := NewSceneWorker(core.NewPathResolver(dir), gen)
	waitForScene(t, worker, "campaign", 3, SceneJob{Prompt: "a hall"})

	if gen.plainCalls != 1 || gen.conditionedCalls != 0 {
		t.Fatalf("calls = %+v, want the plain path", gen)
	}
}

// waitForScene enqueues a job and waits for the worker to write it, so a test can
// assert what was written without a sleep. It returns the written relative path.
func waitForScene(t *testing.T, worker *SceneWorker, gameID string, turn int, job SceneJob) string {
	t.Helper()
	done := make(chan string, 1)
	worker.SetOnReady(func(_ string, _ int, relPath string) { done <- relPath })
	worker.EnqueueScene(gameID, turn, job)

	select {
	case relPath := <-done:
		return relPath
	case <-time.After(5 * time.Second):
		t.Fatal("the scene job did not finish")
		return ""
	}
}

// TestSceneReferenceFindsThePreviousImage guards the reference lookup: the most
// recent illustration of the same location is used, and a different location is
// not.
func TestSceneReferenceFindsThePreviousImage(t *testing.T) {
	dir := t.TempDir()
	paths := core.NewPathResolver(dir)
	scenes := filepath.Join(paths.GameDir("campaign"), "assets", "scenes")
	if err := os.MkdirAll(scenes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scenes, "turn-1.png"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scenes, "turn-2.png"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}

	timeline := NewTimeline(paths, nil, NewHistoryLogger(filepath.Join(paths.GameDir("campaign"), "history.jsonl")), "campaign")
	o := &TurnOrchestrator{timeline: timeline}

	past := []Turn{{Number: 1, Location: "hall"}, {Number: 2, Location: "hall"}, {Number: 3, Location: "yard"}}
	if got := o.sceneReference("hall", past); string(got) != "second" {
		t.Fatalf("reference = %q, want the most recent hall image", got)
	}
	if got := o.sceneReference("yard", past); got != nil {
		t.Fatalf("a location with no illustration should have no reference, got %q", got)
	}
	if got := o.sceneReference("", past); got != nil {
		t.Fatal("no location should have no reference")
	}
}
