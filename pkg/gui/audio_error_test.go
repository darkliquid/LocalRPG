package gui

import (
	"context"
	"testing"
)

func TestASynthesisFailureBroadcastsAnError(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	// A TTS provider whose endpoint refuses the connection: synthesis fails, and
	// the failure must reach the client rather than the beat going quiet.
	cfg := svc.configMgr.Get()
	cfg.Media.TTS.Type = "http"
	cfg.Media.TTS.Endpoint = "http://127.0.0.1:9/v1"

	turn, err := svc.findTurn(gameID, 1)
	if err != nil {
		t.Fatal(err)
	}

	ch, cancel := svc.SubscribeAudioStatus()
	defer cancel()

	plan := &turnAudioPlan{queue: make(chan string, 8), played: newClipSet()}
	svc.emitTurnClips(context.Background(), gameID, *turn, true, func(string) {}, plan)

	select {
	case status := <-ch:
		if status.Error == "" {
			t.Fatalf("expected an error status, got %+v", status)
		}
		if status.Stage != "synthesize" {
			t.Fatalf("Stage = %q, want synthesize", status.Stage)
		}
		if status.Playing {
			t.Fatalf("a failed beat must not report as playing: %+v", status)
		}
	default:
		t.Fatal("a failed synthesis broadcast nothing")
	}
}