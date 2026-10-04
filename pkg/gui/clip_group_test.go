package gui

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestSegmentDTOsShareAGroupClip(t *testing.T) {
	key := strings.Repeat("a", 64)
	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The door opens."},
		{Kind: entity.SegmentNarration, Text: "Cold air rushes in."},
	}
	plan := clipPlan{
		segmentKeys: [][]string{{key}, {key}},
		groupKey:    []string{key, key},
		groups:      []ClipGroupDTO{{Key: key, AudioURLs: []string{clipURL(key)}, SegmentIndexes: []int{0, 1}}},
	}

	dtos := segmentDTOs(segments, "game", plan, nil)
	if len(dtos) != 2 {
		t.Fatalf("expected 2 dtos, got %d", len(dtos))
	}
	if dtos[0].ClipGroup != key || dtos[1].ClipGroup != key {
		t.Errorf("expected both segments to name the group %q, got %q and %q", key, dtos[0].ClipGroup, dtos[1].ClipGroup)
	}
	if len(dtos[0].AudioURLs) != 1 || len(dtos[1].AudioURLs) != 1 {
		t.Fatalf("expected one shared clip each, got %#v / %#v", dtos[0].AudioURLs, dtos[1].AudioURLs)
	}
	if dtos[0].AudioURLs[0] != dtos[1].AudioURLs[0] {
		t.Errorf("expected the group's segments to share one URL, got %q and %q", dtos[0].AudioURLs[0], dtos[1].AudioURLs[0])
	}
}

func TestSegmentDTOsLeaveUngroupedSegmentsUnnamed(t *testing.T) {
	segments := []entity.TurnSegment{{Kind: entity.SegmentNarration, Text: "A."}}
	plan := clipPlan{segmentKeys: [][]string{{strings.Repeat("b", 64)}}, groupKey: []string{""}}

	dtos := segmentDTOs(segments, "game", plan, nil)
	if dtos[0].ClipGroup != "" {
		t.Errorf("expected an ungrouped segment to carry no group, got %q", dtos[0].ClipGroup)
	}
}

func boolPointer(v bool) *bool { return &v }

// A batch backfill and the interactive path must plan a turn's groups under the
// same caps, or the batch writes clips the app never looks up. With sentence
// streaming on the app folds single-speaker, so the pipeline's group caps, which
// the batch planner uses, must clamp a two-speaker provider to one.
func TestBatchPlanFoldsUnderTheLiveCaps(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	cfg := svc.configMgr.Get()
	cfg.Media.TTS.Grouping = "auto"
	cfg.Media.TTS.StreamSentences = boolPointer(true)
	cfg.Media.TTS.Limits = &config.TTSLimits{MaxSpeakers: 2}

	pipeline, err := svc.audioPipeline()
	if err != nil {
		t.Fatalf("audioPipeline: %v", err)
	}
	if got := pipeline.GroupCaps().MaxSpeakers; got != 1 {
		t.Fatalf("pipeline group caps MaxSpeakers = %d, want 1 so the batch writes the live clips", got)
	}

	turn, err := svc.findTurn(gameID, 1)
	if err != nil {
		t.Fatal(err)
	}
	narrator := svc.narratorVoiceFor(gameID, cfg)
	voiceFor := svc.voiceFor(gameID)

	// The batch planner is pipeline.GroupClipKeys; the turn's clip plan must agree.
	plan := svc.clipPlanFor(cfg, gameID, turn.Segments)
	batch := pipeline.GroupClipKeys(turn.Segments, narrator, voiceFor)
	if len(plan.groups) != len(batch) {
		t.Fatalf("turn plan has %d groups, the batch planned %d", len(plan.groups), len(batch))
	}
	for i := range batch {
		if plan.groups[i].Key != batch[i].Key {
			t.Errorf("group %d: turn key %q, batch key %q", i, plan.groups[i].Key, batch[i].Key)
		}
	}
}

func TestGroupingAndStreamingPolicy(t *testing.T) {
	svc := &Service{}
	cases := []struct {
		name       string
		grouping   string
		stream     *bool
		wantGroup  bool
		wantStream bool
	}{
		{name: "off never groups", grouping: "off", stream: boolPointer(true), wantGroup: false, wantStream: true},
		{name: "always groups without streaming", grouping: "always", stream: boolPointer(true), wantGroup: true, wantStream: false},
		{name: "auto groups and streams", grouping: "auto", stream: boolPointer(true), wantGroup: true, wantStream: true},
		{name: "auto without streaming still groups", grouping: "auto", stream: boolPointer(false), wantGroup: true, wantStream: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Media.TTS.Grouping = tc.grouping
			cfg.Media.TTS.StreamSentences = tc.stream
			cfg.Media.TTS.Type = "builtin"
			if got := svc.groupingEnabled(cfg); got != tc.wantGroup {
				t.Errorf("groupingEnabled = %v, want %v", got, tc.wantGroup)
			}
			if got := svc.streamerRuns(cfg); got != tc.wantStream {
				t.Errorf("streamerRuns = %v, want %v", got, tc.wantStream)
			}
		})
	}
}
