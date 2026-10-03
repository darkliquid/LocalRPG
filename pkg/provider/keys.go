package provider

// Canonical adapter keys. Each is the single definition of an adapter's
// identity; descriptors and resolvers reference these constants rather than
// repeating the string.
const (
	KeyLLMOpenAIChat      Key = "llm:openaichat"
	KeyLLMGemini          Key = "llm:gemini"
	KeyLLMCLI             Key = "llm:cli"
	KeyLLMNarrativeOracle Key = "llm:narrative-oracle"
	KeyLLMInworld         Key = "llm:inworld"

	KeyTTSGemini     Key = "tts:gemini"
	KeyTTSElevenLabs Key = "tts:elevenlabs"
	KeyTTSNativeOS   Key = "tts:native-os"
	KeyTTSSherpaONNX Key = "tts:sherpa-onnx"
	KeyTTSPiper      Key = "tts:piper"
	KeyTTSFishAudio  Key = "tts:fish-audio"
	KeyTTSHTTP       Key = "tts:http"
	KeyTTSInworld    Key = "tts:inworld"
	KeyTTSCartesia   Key = "tts:cartesia"

	KeySTTWhisperHTTP Key = "stt:whisper-http"
	KeySTTWhisperCLI  Key = "stt:whisper-cli"
	KeySTTWebSpeech   Key = "stt:web-speech"
	KeySTTInworld     Key = "stt:inworld"
	KeySTTCartesia    Key = "stt:cartesia"

	KeyImageGemini        Key = "image:gemini"
	KeyImageHTTP          Key = "image:http"
	KeyImageCLI           Key = "image:cli"
	KeyImageProceduralArt Key = "image:procedural-art"

	KeyEmbeddingBuiltin Key = "embedding:builtin"
	KeyEmbeddingOpenAI  Key = "embedding:openai"
	KeyEmbeddingGemini  Key = "embedding:gemini"
)

// AllKeys lists every canonical adapter key, for validation and docs.
func AllKeys() []Key {
	return []Key{
		KeyLLMOpenAIChat, KeyLLMGemini, KeyLLMCLI, KeyLLMNarrativeOracle, KeyLLMInworld,
		KeyTTSGemini, KeyTTSElevenLabs, KeyTTSNativeOS, KeyTTSSherpaONNX, KeyTTSPiper,
		KeyTTSFishAudio, KeyTTSHTTP, KeyTTSInworld, KeyTTSCartesia,
		KeySTTWhisperHTTP, KeySTTWhisperCLI, KeySTTWebSpeech, KeySTTInworld, KeySTTCartesia,
		KeyImageGemini, KeyImageHTTP, KeyImageCLI, KeyImageProceduralArt,
		KeyEmbeddingBuiltin, KeyEmbeddingOpenAI, KeyEmbeddingGemini,
	}
}
