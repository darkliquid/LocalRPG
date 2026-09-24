package media

import (
	"context"
	"errors"
)

// ErrWebSpeechIsClientSide reports that browser speech recognition has no backend
// client. The descriptor exists so the catalogue can publish the preset and its
// capabilities; the browser performs the recognition.
var ErrWebSpeechIsClientSide = errors.New("web speech recognition runs in the browser; the backend client is a placeholder")

type webSpeechSTTClient struct{}

func (webSpeechSTTClient) Transcribe(context.Context, []byte) (string, error) {
	return "", ErrWebSpeechIsClientSide
}

// NewWebSpeechSTTProvider returns the placeholder browser-speech client.
func NewWebSpeechSTTProvider() STTClient { return webSpeechSTTClient{} }
