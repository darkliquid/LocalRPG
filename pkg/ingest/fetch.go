package ingest

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// FetchOptions bounds a URL ingestion. The zero value uses the documented
// defaults: a descriptive user agent, a 2 MB per-page cap, at most 20 URLs, and
// a 20-second timeout.
type FetchOptions struct {
	UserAgent string
	MaxBytes  int64
	MaxURLs   int
	Timeout   time.Duration
	// Client overrides the shared client, for a caller that needs a custom
	// transport. A test uses it to prove the folder path makes no call.
	Client *http.Client
}

// DefaultUserAgent is the descriptive agent every fetch sends, so a site owner
// can see who is asking and why.
const DefaultUserAgent = "LocalRPG/1.0 (+https://github.com/darkliquid/localrpg; world source ingestion)"

// httpClient is the shared client, a package variable so a test can replace it.
var httpClient = &http.Client{Timeout: 20 * time.Second}

func (o FetchOptions) resolve() FetchOptions {
	if strings.TrimSpace(o.UserAgent) == "" {
		o.UserAgent = DefaultUserAgent
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = 2 << 20
	}
	if o.MaxURLs <= 0 {
		o.MaxURLs = 20
	}
	if o.Timeout <= 0 {
		o.Timeout = 20 * time.Second
	}
	if o.Client == nil {
		o.Client = httpClient
	}
	return o
}

// ExtractURLs fetches each URL, reduces the page to readable text, and chunks
// it. A failure is reported per URL and never fails the batch, so one dead link
// does not lose the rest. It does not crawl: only the URLs it is given are
// fetched.
func ExtractURLs(ctx context.Context, urls []string, opts FetchOptions) ([]Chunk, []error) {
	opts = opts.resolve()
	if len(urls) > opts.MaxURLs {
		urls = urls[:opts.MaxURLs]
	}

	robots := newRobotsCache(opts)
	var chunks []Chunk
	var errs []error

	for _, raw := range urls {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return chunks, append(errs, err)
		}
		page, err := fetchPage(ctx, opts, robots, raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", raw, err))
			continue
		}
		title, text := HTMLToText(page)
		if strings.TrimSpace(text) == "" {
			errs = append(errs, fmt.Errorf("%s: no readable text (the page may need JavaScript)", raw))
			continue
		}
		if title == "" {
			title = raw
		}
		for _, part := range SplitText(text) {
			chunks = append(chunks, Chunk{Source: raw, Title: title, Text: part})
		}
	}
	return chunks, errs
}

// fetchPage fetches one URL, honouring the host's robots.txt.
func fetchPage(ctx context.Context, opts FetchOptions, robots *robotsCache, raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}

	allowed, err := robots.allowed(ctx, opts, parsed)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", fmt.Errorf("robots.txt disallows this path")
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// Fetching the page the user named is the feature rather than a privilege
	// boundary: the app serves one user over loopback or a 0600 socket, and the
	// scheme allowlist above is the control that matters.
	// lgtm[go/request-forgery]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", opts.UserAgent)
	req.Header.Set("Accept", "text/html,text/plain;q=0.9,*/*;q=0.5")

	resp, err := opts.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, opts.MaxBytes))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(body), nil
}

var (
	scriptRe = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	styleRe  = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	titleRe  = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title>`)
	tagRe    = regexp.MustCompile(`(?s)<[^>]*>`)
	blockRe  = regexp.MustCompile(`(?i)</?(p|div|br|li|ul|ol|h[1-6]|tr|table|section|article|header|footer|blockquote)\b[^>]*>`)
	spaceRe  = regexp.MustCompile(`[ \t\f\v]+`)
	blankRe  = regexp.MustCompile(`\n{3,}`)
)

// HTMLToText reduces a page to its title and readable prose, dropping scripts,
// styles, and markup. It is deliberately small: it handles a prose page, not a
// JavaScript application.
func HTMLToText(page string) (string, string) {
	title := ""
	if m := titleRe.FindStringSubmatch(page); len(m) > 1 {
		title = collapseWhitespace(html.UnescapeString(stripTags(m[1])))
	}

	body := scriptRe.ReplaceAllString(page, " ")
	body = styleRe.ReplaceAllString(body, " ")
	body = titleRe.ReplaceAllString(body, " ")
	body = blockRe.ReplaceAllString(body, "\n")
	body = tagRe.ReplaceAllString(body, " ")
	body = html.UnescapeString(body)

	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = collapseWhitespace(line)
		if line == "" {
			continue
		}
		kept = append(kept, line)
	}
	return title, blankRe.ReplaceAllString(strings.Join(kept, "\n\n"), "\n\n")
}

func stripTags(s string) string { return tagRe.ReplaceAllString(s, "") }

// collapseWhitespace squeezes runs of spaces and tabs and trims the result.
func collapseWhitespace(s string) string {
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// robotsCache fetches and remembers one robots.txt per host.
type robotsCache struct {
	opts   FetchOptions
	rules  map[string][]string
	loaded map[string]bool
}

func newRobotsCache(opts FetchOptions) *robotsCache {
	return &robotsCache{opts: opts, rules: map[string][]string{}, loaded: map[string]bool{}}
}

// allowed reports whether robots.txt permits fetching a path. A robots.txt that
// cannot be read permits the fetch: the absence of a rule is not a prohibition,
// and a network failure must not silently block every URL.
func (c *robotsCache) allowed(ctx context.Context, opts FetchOptions, target *url.URL) (bool, error) {
	host := target.Scheme + "://" + target.Host
	if !c.loaded[host] {
		c.loaded[host] = true
		c.rules[host] = c.fetch(ctx, opts, host+"/robots.txt")
	}
	for _, rule := range c.rules[host] {
		if rule != "" && strings.HasPrefix(target.Path, rule) {
			return false, nil
		}
	}
	return true, nil
}

// fetch reads a robots.txt and returns its Disallow paths.
func (c *robotsCache) fetch(ctx context.Context, opts FetchOptions, robotsURL string) []string {
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// The robots URL is the user's own URL with its path replaced, so it is the
	// same intended fetch as the page itself.
	// lgtm[go/request-forgery]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", opts.UserAgent)

	resp, err := opts.Client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return nil
	}
	return parseRobots(string(body))
}

// parseRobots extracts the Disallow paths that apply to every agent. A
// per-agent block is out of scope: this client sends one agent and does not
// crawl, so the wildcard group is the one that governs it.
func parseRobots(body string) []string {
	var paths []string
	applies := false
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			applies = value == "*"
		case "disallow":
			if applies && value != "" {
				paths = append(paths, value)
			}
		}
	}
	return paths
}
