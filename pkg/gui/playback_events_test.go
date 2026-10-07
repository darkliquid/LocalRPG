package gui

import "testing"

func TestPlaybackCompletionBroadcasts(t *testing.T) {
	svc := &Service{}
	ch, cancel := svc.SubscribeAudioStatus()
	defer cancel()

	svc.broadcastAudioStatus(AudioStatusDTO{Available: true, Playing: false})

	select {
	case status := <-ch:
		if status.Playing {
			t.Fatalf("status = %+v", status)
		}
	default:
		t.Fatal("expected a broadcast on the subscription")
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	svc := &Service{}
	ch, cancel := svc.SubscribeAudioStatus()
	cancel()

	svc.broadcastAudioStatus(AudioStatusDTO{Playing: false})

	select {
	case <-ch:
		t.Fatal("a cancelled subscription should not receive events")
	default:
	}
}
