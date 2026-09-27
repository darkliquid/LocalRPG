package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
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
