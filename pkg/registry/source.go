package registry

import (
	"fmt"
	"net/url"
	"strings"
)

// gitSourceSchemes are the transports a git+ registry URL may use. The client's
// fetchOrCachedIndex routes any git+ prefix to the clone path, so the scheme is
// what decides whether the URL is usable.
var gitSourceSchemes = map[string]bool{
	"https": true,
	"http":  true,
	"ssh":   true,
	"file":  true,
}

// NormalizeSource validates a registry URL and returns its canonical string.
// Accepted forms are an http:// or https:// index, or a git+<scheme>:// repository
// the client clones. The CLI and the GUI share this rule so a URL rejected by one
// is rejected by the other.
func NormalizeSource(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("registry URL is empty")
	}

	if rest, ok := strings.CutPrefix(trimmed, "git+"); ok {
		u, err := url.Parse(rest)
		if err != nil || !gitSourceSchemes[u.Scheme] || (u.Host == "" && u.Scheme != "file") {
			return "", fmt.Errorf("invalid git registry URL %q", trimmed)
		}
		return trimmed, nil
	}

	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("invalid registry URL %q", trimmed)
	}
	return trimmed, nil
}