package harness

// Capabilities is the neutral, package-local description a provider adapter
// offers. pkg/provider converts it into a Descriptor, and a drift guard checks
// the two agree.
type Capabilities struct {
	Streaming    bool
	Tools        bool
	Thinking     bool
	Vision       bool
	Sessions     bool
	ContextCache bool
}

// Describe derives capabilities from the adapter's real surfaces, so a
// descriptor cannot claim a capability the code does not implement.
func Describe(p ModelProvider) Capabilities {
	caps := Capabilities{Streaming: true}
	if caller, ok := p.(ToolCaller); ok {
		caps.Tools = caller.ToolCallerCapable()
	}
	if thinker, ok := p.(interface{ SupportsThinking() bool }); ok {
		caps.Thinking = thinker.SupportsThinking()
	}
	if _, ok := p.(SessionProvider); ok {
		caps.Sessions = true
	}
	if _, ok := p.(ContextCacher); ok {
		caps.ContextCache = true
	}
	return caps
}
