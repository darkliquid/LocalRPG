package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractURLsReducesHTMLToText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}
		_, _ = w.Write([]byte("<html><head><title>Saltmarch</title></head><body>" +
			"<h1>Saltmarch</h1><p>A port.</p><script>x</script></body></html>"))
	}))
	defer srv.Close()

	chunks, errs := ExtractURLs(context.Background(), []string{srv.URL + "/page"}, FetchOptions{})
	if len(errs) != 0 || len(chunks) != 1 {
		t.Fatalf("chunks %d errs %v", len(chunks), errs)
	}
	if !strings.Contains(chunks[0].Text, "A port.") || strings.Contains(chunks[0].Text, "<script>") {
		t.Fatalf("text = %q", chunks[0].Text)
	}
	if chunks[0].Title != "Saltmarch" {
		t.Fatalf("title = %q", chunks[0].Title)
	}
	if chunks[0].Source != srv.URL+"/page" {
		t.Fatalf("source = %q", chunks[0].Source)
	}
}

func TestExtractURLsHonoursRobots(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /private\n"))
			return
		}
		_, _ = w.Write([]byte("<html><body><p>Secret.</p></body></html>"))
	}))
	defer srv.Close()

	chunks, errs := ExtractURLs(context.Background(), []string{srv.URL + "/private/page"}, FetchOptions{})
	if len(chunks) != 0 {
		t.Fatalf("a disallowed path must not be ingested: %+v", chunks)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "robots.txt") {
		t.Fatalf("errs = %v", errs)
	}
}

func TestExtractURLsOneFailureDoesNotFailTheBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/ok" {
			_, _ = w.Write([]byte("<html><body><p>A port.</p></body></html>"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	chunks, errs := ExtractURLs(context.Background(), []string{srv.URL + "/broken", srv.URL + "/ok"}, FetchOptions{})
	if len(chunks) != 1 {
		t.Fatalf("the working URL should still ingest: %+v", chunks)
	}
	if len(errs) != 1 {
		t.Fatalf("errs = %v", errs)
	}
}

func TestExtractURLsRefusesANonHTTPScheme(t *testing.T) {
	_, errs := ExtractURLs(context.Background(), []string{"file:///etc/passwd"}, FetchOptions{})
	if len(errs) != 1 {
		t.Fatalf("errs = %v", errs)
	}
}

func TestExtractURLsBoundsTheURLCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("<html><body><p>Text.</p></body></html>"))
	}))
	defer srv.Close()

	urls := []string{srv.URL + "/a", srv.URL + "/b", srv.URL + "/c"}
	chunks, _ := ExtractURLs(context.Background(), urls, FetchOptions{MaxURLs: 1})
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
}

func TestParseRobotsUsesTheWildcardGroup(t *testing.T) {
	body := "User-agent: Googlebot\nDisallow: /everything\n\nUser-agent: *\nDisallow: /private\n# a comment\n"
	got := parseRobots(body)
	if len(got) != 1 || got[0] != "/private" {
		t.Fatalf("rules = %+v", got)
	}
}

func TestHTMLToTextStripsMarkup(t *testing.T) {
	title, text := HTMLToText("<html><head><title> A &amp; B </title></head><body>" +
		"<style>p{}</style><h1>Heading</h1><p>First.</p><p>Second.</p></body></html>")
	if title != "A & B" {
		t.Fatalf("title = %q", title)
	}
	if !strings.Contains(text, "Heading") || !strings.Contains(text, "First.") {
		t.Fatalf("text = %q", text)
	}
	if strings.Contains(text, "p{}") || strings.Contains(text, "<h1>") {
		t.Fatalf("markup survived: %q", text)
	}
}

func TestFetchOptionsResolveDefaults(t *testing.T) {
	got := FetchOptions{}.resolve()
	if got.UserAgent != DefaultUserAgent || got.MaxBytes == 0 || got.MaxURLs == 0 || got.Client == nil {
		t.Fatalf("resolved = %+v", got)
	}
}
