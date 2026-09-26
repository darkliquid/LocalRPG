//go:build sherpa

// Registers the opt-in Sherpa-ONNX Kokoro TTS provider. It requires cgo, so it
// is excluded from the default build; enable it with `go build -tags sherpa`.
package all

import (
	_ "github.com/darkliquid/localrpg/pkg/provider/ttssherpa"
)
