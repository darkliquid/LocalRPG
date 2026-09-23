package media

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestProviderKey(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.TTSConfig
		want string
	}{
		{"disabled", config.TTSConfig{Type: "disabled"}, "disabled"},
		{"empty", config.TTSConfig{}, "disabled"},
		{"builtin named", config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"}, "builtin:elevenlabs"},
		{"builtin sherpa", config.TTSConfig{Type: "builtin", BuiltinName: "sherpa-onnx"}, "builtin:sherpa-onnx"},
		{"builtin unnamed", config.TTSConfig{Type: "builtin"}, "builtin:echo"},
		{"http host and port", config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880/v1/audio/speech"}, "http:localhost:8880"},
		{"cli basename", config.TTSConfig{Type: "cli", Command: "/usr/local/bin/piper"}, "cli:piper"},
		{"unknown type", config.TTSConfig{Type: "Foo Bar"}, "foo-bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProviderKey(tc.cfg); got != tc.want {
				t.Errorf("ProviderKey(%+v) = %q, want %q", tc.cfg, got, tc.want)
			}
		})
	}
}

func TestKeyPresent(t *testing.T) {
	t.Setenv("ELEVENLABS_API_KEY", "")

	if KeyPresent(config.TTSConfig{}) {
		t.Errorf("an unconfigured provider has no key")
	}
	if !KeyPresent(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs", APIKey: "abc"}) {
		t.Errorf("a configured key must report present")
	}

	t.Setenv("ELEVENLABS_API_KEY", "from-env")
	if !KeyPresent(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"}) {
		t.Errorf("the environment key must count as present")
	}
	if KeyPresent(config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880"}) {
		t.Errorf("an unrelated provider must not read the ElevenLabs environment key")
	}
}
