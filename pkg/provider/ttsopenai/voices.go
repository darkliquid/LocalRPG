package ttsopenai

import (
	"context"

	"github.com/darkliquid/localrpg/pkg/media"
)

// voices is OpenAI's fixed speech voice set. The audio speech API has no
// voices endpoint, so the catalogue is a constant list rather than a fetch.
// Gender and accent tags follow the names OpenAI documents; "recommended"
// marks the voices OpenAI currently steers new apps to.
var voices = []media.ProviderVoice{
	{ID: "alloy", Name: "Alloy", Tags: []string{"neutral", "balanced"}, Description: "Neutral and balanced."},
	{ID: "ash", Name: "Ash", Tags: []string{"neutral"}, Description: "Even, steady."},
	{ID: "ballad", Name: "Ballad", Tags: []string{"expressive"}, Description: "Expressive, closer to song."},
	{ID: "cedar", Name: "Cedar", Tags: []string{"recommended"}, Description: "One of the current recommended voices."},
	{ID: "coral", Name: "Coral", Tags: []string{"female", "clear"}, Description: "Clear and friendly."},
	{ID: "echo", Name: "Echo", Tags: []string{"male"}, Description: "Smooth male voice."},
	{ID: "fable", Name: "Fable", Tags: []string{"british", "narrative"}, Description: "Narrative British voice, story-oriented."},
	{ID: "marin", Name: "Marin", Tags: []string{"recommended"}, Description: "One of the current recommended voices."},
	{ID: "nova", Name: "Nova", Tags: []string{"female"}, Description: "Bright female voice."},
	{ID: "onyx", Name: "Onyx", Tags: []string{"male", "deep"}, Description: "Deep, authoritative male voice."},
	{ID: "sage", Name: "Sage", Tags: []string{"calm"}, Description: "Calm and measured."},
	{ID: "shimmer", Name: "Shimmer", Tags: []string{"female", "soft"}, Description: "Soft and airy."},
	{ID: "verse", Name: "Verse", Tags: []string{"expressive"}, Description: "Expressive, dramatic."},
}

// ListVoices returns OpenAI's fixed speech voice set.
func (c *client) ListVoices(_ context.Context) ([]media.ProviderVoice, error) {
	out := make([]media.ProviderVoice, len(voices))
	copy(out, voices)
	return out, nil
}
