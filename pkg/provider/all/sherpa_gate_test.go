package all

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestSherpaProviderIsOptIn(t *testing.T) {
	if _, ok := provider.Lookup("tts-sherpa-onnx"); ok {
		t.Fatal("tts-sherpa-onnx must not be registered in the default build")
	}
}
