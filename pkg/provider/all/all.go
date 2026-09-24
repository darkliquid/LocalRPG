// Package all blank-imports every provider package so their init functions
// register with pkg/provider. Import it from main for side effects.
package all

import (
	_ "github.com/darkliquid/localrpg/pkg/provider/clillm"
	_ "github.com/darkliquid/localrpg/pkg/provider/geminillm"
	_ "github.com/darkliquid/localrpg/pkg/provider/openaichat"
	_ "github.com/darkliquid/localrpg/pkg/provider/oracle"
)
