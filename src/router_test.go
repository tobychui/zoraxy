package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

/*
	Tests for the static / HTML response caching headers.

	Production may hand the browser a validated cached copy, but a development
	build (`-dev`) must never let the browser reuse anything, otherwise UI edits
	would not show up on reload. These tests pin both behaviors.
*/

func withDevelopmentBuild(t *testing.T, dev bool) {
	t.Helper()
	original := *development_build
	*development_build = dev
	t.Cleanup(func() { *development_build = original })
}

func newStaticRequest(path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	serveStaticWithCache(rec, req, next)
	return rec
}

func newHTMLRequest(path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	handleInjectHTML(rec, req, path)
	return rec
}

func TestStaticAssetsNotCachedInDevMode(t *testing.T) {
	withDevelopmentBuild(t, true)

	rec := newStaticRequest("/favicon.png")

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("dev static: expected Cache-Control %q, got %q", "no-store", got)
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Fatalf("dev static: expected no ETag, got %q", got)
	}
}

func TestHTMLNotCachedInDevMode(t *testing.T) {
	withDevelopmentBuild(t, true)

	rec := newHTMLRequest("/index.html")

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("dev html: expected Cache-Control %q, got %q", "no-store", got)
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Fatalf("dev html: expected no ETag, got %q", got)
	}
}

func TestStaticAssetsRevalidatedInProduction(t *testing.T) {
	withDevelopmentBuild(t, false)

	rec := newStaticRequest("/favicon.png")

	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("prod static: expected Cache-Control %q, got %q", "no-cache", got)
	}
	if got := rec.Header().Get("ETag"); got == "" {
		t.Fatal("prod static: expected an ETag to be set")
	}
}

func TestHTMLRevalidatedInProduction(t *testing.T) {
	withDevelopmentBuild(t, false)

	rec := newHTMLRequest("/index.html")

	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("prod html: expected Cache-Control %q, got %q", "no-cache", got)
	}
	if got := rec.Header().Get("ETag"); got == "" {
		t.Fatal("prod html: expected an ETag to be set")
	}
}
