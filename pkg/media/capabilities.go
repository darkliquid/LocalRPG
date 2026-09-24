package media

import "context"

// Capabilities is the neutral, package-local description a TTS adapter offers.
// pkg/provider converts it into a Descriptor, and a drift guard checks the two
// agree.
type Capabilities struct {
	VoiceCatalog     bool
	VoiceOptions     bool
	SpeechCues       bool
	MarkdownEmphasis bool
	Metered          bool
	ExtendedVoices   bool
}

// Describe derives capabilities by type-asserting the adapter's real
// interfaces, so a descriptor cannot claim what the code does not implement.
func Describe(client TTSClient) Capabilities {
	var caps Capabilities
	if _, ok := client.(VoiceCatalog); ok {
		caps.VoiceCatalog = true
	}
	if _, ok := client.(VoiceOptions); ok {
		caps.VoiceOptions = true
	}
	if _, ok := client.(SpeechCueAdvertiser); ok {
		caps.SpeechCues = true
	}
	if aware, ok := client.(MarkdownAware); ok {
		caps.MarkdownEmphasis = aware.SupportsMarkdown()
	}
	if metered, ok := client.(MeteredProvider); ok {
		caps.Metered = metered.Metered()
	}
	if _, ok := client.(ExtendedVoiceSearcher); ok {
		caps.ExtendedVoices = true
	}
	return caps
}

// ExtendedVoiceSearcher is implemented by TTS clients that publish a searchable
// library beyond their default catalog.
type ExtendedVoiceSearcher interface {
	ListExtendedVoices(ctx context.Context, query string) ([]ProviderVoice, error)
}
