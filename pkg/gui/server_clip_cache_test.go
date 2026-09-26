package gui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClipHeadersAreCacheableWithETag(t *testing.T) {
	rec := httptest.NewRecorder()
	setClipHeaders(rec, []byte("OggS-data"))
	if rec.Header().Get("ETag") == "" {
		t.Error("expected an ETag on the clip response")
	}
	if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want it cacheable", cc)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "ogg") {
		t.Errorf("Content-Type = %q, want an Ogg type", ct)
	}
}

func TestClipETagFollowsTheBytes(t *testing.T) {
	first := httptest.NewRecorder()
	setClipHeaders(first, []byte("one clip"))
	second := httptest.NewRecorder()
	setClipHeaders(second, []byte("another clip"))
	if first.Header().Get("ETag") == second.Header().Get("ETag") {
		t.Error("different clip bytes produced the same ETag")
	}
}
