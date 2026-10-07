package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestShouldIllustrate(t *testing.T) {
	cfg := DefaultTriggerConfig()
	if ok, _ := ShouldIllustrate(Turn{SceneBreak: true}, Turn{}, cfg); !ok {
		t.Fatal("a scene break is significant")
	}
	if ok, _ := ShouldIllustrate(Turn{Narration: "You wait."}, Turn{}, cfg); ok {
		t.Fatal("a quiet one-liner is not significant")
	}
	failed := Turn{Checks: []harness.CheckResult{{Outcome: "miss"}}, Narration: "The lock holds."}
	if ok, reason := ShouldIllustrate(failed, Turn{}, cfg); !ok || reason == "" {
		t.Fatal("a failed check is significant and should carry a reason")
	}
}

func TestShouldIllustrateLocationAndSpeaker(t *testing.T) {
	cfg := DefaultTriggerConfig()
	moved := Turn{Location: "hall"}
	if ok, _ := ShouldIllustrate(moved, Turn{Location: "tavern"}, cfg); !ok {
		t.Fatal("a location change is significant")
	}
	entered := Turn{Segments: []entity.TurnSegment{{SpeakerID: "kaelen"}}}
	if ok, reason := ShouldIllustrate(entered, Turn{}, cfg); !ok || reason == "" {
		t.Fatal("a new speaker is significant")
	}
}

func TestGateHonoursPolicies(t *testing.T) {
	turn := Turn{SceneBreak: true}
	cases := []struct {
		policy string
		want   bool
	}{
		{"off", false}, {"manual", false}, {"every_turn", true},
		{"scene_break", true}, {"significant", true},
	}
	for _, c := range cases {
		o := &TurnOrchestrator{imageTrigger: c.policy}
		if got := o.shouldIllustrate(turn, nil); got != c.want {
			t.Errorf("policy %q: got %v want %v", c.policy, got, c.want)
		}
	}

	significant := &TurnOrchestrator{imageTrigger: "significant"}
	if significant.shouldIllustrate(Turn{Narration: "You wait."}, nil) {
		t.Error("a quiet turn under significant should not be illustrated")
	}

	// The unset policy keeps today's scene-break behaviour.
	def := &TurnOrchestrator{}
	if def.shouldIllustrate(Turn{Narration: "You wait."}, nil) {
		t.Error("the unset policy should not illustrate a quiet turn")
	}
	if !def.shouldIllustrate(turn, nil) {
		t.Error("the unset policy should illustrate a scene break")
	}
}
