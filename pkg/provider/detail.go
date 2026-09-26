package provider

import "strings"

// MaxProviderDetailBytes bounds how much of a provider's response body or CLI
// stderr is kept in an error, so a large error page cannot bloat a failure that
// is also written to the trace and to the HTTP response.
const MaxProviderDetailBytes = 8192

// TruncateDetail renders a provider payload as a compact, bounded string for an
// error message. It truncates on a rune boundary so the result stays valid UTF-8.
func TruncateDetail(data []byte) string {
	return TruncateDetailString(string(data))
}

// TruncateDetailString is TruncateDetail for an already-stringified payload such
// as captured stderr.
func TruncateDetailString(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) > MaxProviderDetailBytes {
		return string(runes[:MaxProviderDetailBytes]) + "..."
	}
	return trimmed
}
