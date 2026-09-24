package media_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestDisabledFactoriesReturnPlaceholders(t *testing.T) {
	tts, err := media.NewTTSClient(config.TTSConfig{Type: "disabled"})
	if err != nil || tts == nil {
		t.Fatalf("disabled tts: %v", err)
	}
	stt, err := media.NewSTTClient(config.STTConfig{Type: "disabled"})
	if err != nil || stt == nil {
		t.Fatalf("disabled stt: %v", err)
	}
	img, err := media.NewImageClient(config.ImageConfig{Type: "disabled"})
	if err != nil || img == nil {
		t.Fatalf("disabled image: %v", err)
	}
}
