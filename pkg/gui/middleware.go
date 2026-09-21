package gui

import "net/http"

// embeddedWebviewOrigins are the origins used by the native Wails webview when
// it requests the API through an in-process scheme handler rather than a
// network listener. They are trusted because those requests never leave the
// process.
var embeddedWebviewOrigins = []string{
	"wails://wails",
	"http://wails.localhost",
}

// ProtectCrossOrigin wraps h with the standard library's cross-origin request
// protection, so browsers on other sites cannot drive the local API while
// same-origin callers, local tooling, and the native webview keep working.
func ProtectCrossOrigin(h http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	for _, origin := range embeddedWebviewOrigins {
		_ = protection.AddTrustedOrigin(origin)
	}
	return protection.Handler(h)
}
