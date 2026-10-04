package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestDocsService_GetDocsList(t *testing.T) {
	svc := &Service{}
	docs, err := svc.GetDocsList(context.Background())
	if err != nil {
		t.Fatalf("GetDocsList failed: %v", err)
	}

	if len(docs) < 10 {
		t.Fatalf("expected at least 10 docs, got %d", len(docs))
	}

	// Verify first article is overview
	if docs[0].ID != "01-overview" {
		t.Errorf("expected first article to be 01-overview, got %s", docs[0].ID)
	}

	// Verify categories are populated
	foundCategories := make(map[string]bool)
	for _, doc := range docs {
		if doc.Title == "" {
			t.Errorf("doc %s missing title", doc.ID)
		}
		if doc.Category == "" {
			t.Errorf("doc %s missing category", doc.ID)
		}
		foundCategories[doc.Category] = true
	}

	expectedCategories := []string{
		"Core Concepts",
		"Configuration & Providers",
		"Studio Guides",
		"Codex & Content Reference",
		"Local AI & Self-Hosting",
	}
	for _, cat := range expectedCategories {
		if !foundCategories[cat] {
			t.Errorf("missing expected category %q", cat)
		}
	}
}

func TestDocsService_GetDocArticle(t *testing.T) {
	svc := &Service{}
	article, err := svc.GetDocArticle(context.Background(), "01-overview")
	if err != nil {
		t.Fatalf("GetDocArticle failed: %v", err)
	}

	if article.ID != "01-overview" {
		t.Errorf("expected id 01-overview, got %s", article.ID)
	}
	if article.Title != "Architecture & Core Concepts" {
		t.Errorf("unexpected title: %s", article.Title)
	}
	if len(article.Content) == 0 {
		t.Errorf("expected non-empty article content")
	}

	// Test non-existent article returns ErrDocNotFound
	_, err = svc.GetDocArticle(context.Background(), "non-existent-article")
	if err == nil {
		t.Errorf("expected error for non-existent article, got nil")
	}

	// Verify local provider setup guides
	localDocIDs := []string{
		"14-local-llm-ollama",
		"15-local-tts-kokoro",
		"16-local-tts-fish-audio",
		"17-local-stt-whisper",
		"18-local-image-comfyui",
		"19-local-stack-docker-compose",
	}
	for _, docID := range localDocIDs {
		art, err := svc.GetDocArticle(context.Background(), docID)
		if err != nil {
			t.Errorf("GetDocArticle(%q) failed: %v", docID, err)
			continue
		}
		if art.Category != "Local AI & Self-Hosting" {
			t.Errorf("article %q expected category 'Local AI & Self-Hosting', got %q", docID, art.Category)
		}
		if len(art.Content) == 0 {
			t.Errorf("article %q has empty content", docID)
		}
	}
}

func TestServer_DocsEndpoints(t *testing.T) {
	svc := &Service{}
	server := NewServer(svc, nil)

	// Test GET /api/docs
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/docs returned status %d", w.Code)
	}

	// Test GET /api/docs/01-overview
	req = httptest.NewRequest(http.MethodGet, "/api/docs/01-overview", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/docs/01-overview returned status %d", w.Code)
	}

	// Test GET /api/docs/not-found
	req = httptest.NewRequest(http.MethodGet, "/api/docs/non-existent-doc", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/docs/non-existent-doc expected 404, got %d", w.Code)
	}
}

// The Usage panel links unpriced rows to this article by ID, so the contract is
// asserted here rather than only in the frontend.
func TestDocsService_UsagePricingArticle(t *testing.T) {
	svc := &Service{}
	article, err := svc.GetDocArticle(context.Background(), "11-usage-and-pricing")
	if err != nil {
		t.Fatalf("GetDocArticle(11-usage-and-pricing) failed: %v", err)
	}
	if article.Category != "Configuration & Providers" {
		t.Errorf("unexpected category: %s", article.Category)
	}
	if !strings.Contains(article.Content, "providers.prices") {
		t.Errorf("expected article to document the providers.prices config key")
	}
}

// Cross-references between articles are navigated by article ID, so a link to a
// mistyped or removed article would silently fail in the viewer.
func TestDocsService_InternalLinksResolve(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	list, err := svc.GetDocsList(ctx)
	if err != nil {
		t.Fatalf("GetDocsList failed: %v", err)
	}
	known := make(map[string]bool, len(list))
	for _, summary := range list {
		known[summary.ID] = true
	}

	linkPattern := regexp.MustCompile(`\]\(([^)\s]+)\)`)
	for _, summary := range list {
		article, err := svc.GetDocArticle(ctx, summary.ID)
		if err != nil {
			t.Fatalf("GetDocArticle(%s) failed: %v", summary.ID, err)
		}
		for _, match := range linkPattern.FindAllStringSubmatch(article.Content, -1) {
			target := match[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			ref := strings.TrimSuffix(strings.TrimPrefix(target, "/"), ".md")
			if !known[ref] {
				t.Errorf("article %s links to unknown article %q", summary.ID, target)
			}
		}
	}
}
