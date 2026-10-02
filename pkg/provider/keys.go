package provider

// Canonical adapter keys. Each is the single definition of an adapter's
// identity; descriptors and resolvers reference these constants rather than
// repeating the string.
const (
	KeyLLMOpenAIChat      Key = "llm:openaichat"
	KeyLLMGemini          Key = "llm:gemini"
	KeyLLMCLI             Key = "llm:cli"
	KeyLLMNarrativeOracle Key = "llm:narrative-oracle"

	KeyTTSGemini     Key = "tts:gemini"
	KeyTTSElevenLabs Key = "tts:elevenlabs"
	KeyTTSNativeOS   Key = "tts:native-os"
	KeyTTSSherpaONNX Key = "tts:sherpa-onnx"
	KeyTTSPiper      Key = "tts:piper"
	KeyTTSHTTP       Key = "tts:http"
	KeyTTSCartesia   Key = "tts:cartesia"

	KeySTTWhisperHTTP Key = "stt:whisper-http"
	KeySTTWhisperCLI  Key = "stt:whisper-cli"
	KeySTTWebSpeech   Key = "stt:web-speech"
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
		KeyLLMOpenAIChat, KeyLLMGemini, KeyLLMCLI, KeyLLMNarrativeOracle,
		KeyTTSGemini, KeyTTSElevenLabs, KeyTTSNativeOS, KeyTTSSherpaONNX, KeyTTSPiper, KeyTTSHTTP, KeyTTSCartesia,
		KeySTTWhisperHTTP, KeySTTWhisperCLI, KeySTTWebSpeech, KeySTTCartesia,
		KeyImageGemini, KeyImageHTTP, KeyImageCLI, KeyImageProceduralArt,
		KeyEmbeddingBuiltin, KeyEmbeddingOpenAI, KeyEmbeddingGemini,
	}
}
