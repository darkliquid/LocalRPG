// Package all blank-imports every provider package so their init functions
// register with pkg/provider. Import it from main for side effects.
package all

import (
	_ "github.com/darkliquid/localrpg/pkg/provider/clillm"
	_ "github.com/darkliquid/localrpg/pkg/provider/geminillm"
	_ "github.com/darkliquid/localrpg/pkg/provider/imagecli"
	_ "github.com/darkliquid/localrpg/pkg/provider/imagegemini"
	_ "github.com/darkliquid/localrpg/pkg/provider/imagehttp"
	_ "github.com/darkliquid/localrpg/pkg/provider/imageprocedural"
	_ "github.com/darkliquid/localrpg/pkg/provider/openaichat"
	_ "github.com/darkliquid/localrpg/pkg/provider/oracle"
	_ "github.com/darkliquid/localrpg/pkg/provider/sttwebspeech"
	_ "github.com/darkliquid/localrpg/pkg/provider/sttwhispercli"
	_ "github.com/darkliquid/localrpg/pkg/provider/sttwhisperhttp"
	_ "github.com/darkliquid/localrpg/pkg/provider/ttselevenlabs"
	_ "github.com/darkliquid/localrpg/pkg/provider/ttsgemini"
	_ "github.com/darkliquid/localrpg/pkg/provider/ttshttp"
	_ "github.com/darkliquid/localrpg/pkg/provider/ttsnativeos"
	_ "github.com/darkliquid/localrpg/pkg/provider/ttspiper"
	_ "github.com/darkliquid/localrpg/pkg/provider/ttssherpa"
)
