package source_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"appimage-manager/internal/source"
)

var obsidianRef = source.Ref{Kind: source.GitHub, Owner: "obsidianmd", Repo: "obsidian-releases"}

const latestObsidian = `{
  "tag_name": "v1.9.0",
  "published_at": "2026-10-01T12:00:00Z",
  "prerelease": false,
  "assets": [{
    "name": "Obsidian-1.9.0.AppImage",
    "browser_download_url": "https://github.com/obsidianmd/obsidian-releases/releases/download/v1.9.0/Obsidian-1.9.0.AppImage",
    "size": 123456,
    "digest": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
  }]
}`

// fakeGitHub serves canned API responses, keyed by request path.
func fakeGitHub(t *testing.T, responses map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newGitHub(t *testing.T, srv *httptest.Server) source.ReleaseSource {
	t.Helper()
	return source.NewGitHub(obsidianRef, source.GitHubOptions{APIBase: srv.URL, Client: srv.Client(), CacheDir: t.TempDir()})
}

func TestGitHubLatestReturnsTheLatestReleaseWithItsAssets(t *testing.T) {
	srv := fakeGitHub(t, map[string]string{"/repos/obsidianmd/obsidian-releases/releases/latest": latestObsidian})

	got, err := newGitHub(t, srv).Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := source.Release{
		Tag:       "v1.9.0",
		Published: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		Assets: []source.Asset{{
			Name:   "Obsidian-1.9.0.AppImage",
			URL:    "https://github.com/obsidianmd/obsidian-releases/releases/download/v1.9.0/Obsidian-1.9.0.AppImage",
			Size:   123456,
			Digest: "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestGitHubSendsTheConfiguredToken(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Write([]byte(latestObsidian))
	}))
	defer srv.Close()
	gh := source.NewGitHub(obsidianRef, source.GitHubOptions{APIBase: srv.URL, Client: srv.Client(), Token: "ghp_example", CacheDir: t.TempDir()})

	if _, err := gh.Latest(context.Background()); err != nil {
		t.Fatal(err)
	}

	if auth != "Bearer ghp_example" {
		t.Fatalf("got Authorization %q; want \"Bearer ghp_example\"", auth)
	}
}

func TestGitHubReportsAnExhaustedRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer srv.Close()

	_, err := newGitHub(t, srv).Latest(context.Background())

	if !errors.Is(err, source.ErrRateLimited) {
		t.Fatalf("got %v; want ErrRateLimited", err)
	}
}

// countingGitHub serves latestObsidian with an ETag, answering 304 to
// matching If-None-Match requests, and counts the requests it receives.
func countingGitHub(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("ETag", `W/"v1.9.0"`)
		if r.Header.Get("If-None-Match") == `W/"v1.9.0"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Write([]byte(latestObsidian))
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestGitHubAnswersARepeatLookupWithinTenMinutesFromTheCache(t *testing.T) {
	srv, requests := countingGitHub(t)
	cache := t.TempDir()
	now := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	lookup := func(at time.Time) {
		// A new Release Source each time, as each aim process would make.
		gh := source.NewGitHub(obsidianRef, source.GitHubOptions{
			APIBase: srv.URL, Client: srv.Client(), CacheDir: cache, Now: func() time.Time { return at },
		})
		if rel, err := gh.Latest(context.Background()); err != nil || rel.Tag != "v1.9.0" {
			t.Fatalf("got %q, %v", rel.Tag, err)
		}
	}

	lookup(now)
	lookup(now.Add(9 * time.Minute))

	if *requests != 1 {
		t.Fatalf("got %d requests; want 1", *requests)
	}
}

func TestGitHubRevalidatesAStaleCacheWithItsETag(t *testing.T) {
	srv, requests := countingGitHub(t)
	cache := t.TempDir()
	now := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	for _, at := range []time.Time{now, now.Add(11 * time.Minute)} {
		gh := source.NewGitHub(obsidianRef, source.GitHubOptions{
			APIBase: srv.URL, Client: srv.Client(), CacheDir: cache, Now: func() time.Time { return at },
		})
		if rel, err := gh.Latest(context.Background()); err != nil || rel.Tag != "v1.9.0" {
			t.Fatalf("at %s: got %q, %v; want v1.9.0 from the 304-revalidated cache", at, rel.Tag, err)
		}
	}

	if *requests != 2 {
		t.Fatalf("got %d requests; want 2 (the second a conditional 304)", *requests)
	}
}

func TestGitHubRecentSkipsPrereleasesAndDrafts(t *testing.T) {
	srv := fakeGitHub(t, map[string]string{"/repos/obsidianmd/obsidian-releases/releases": `[
		{"tag_name": "v1.10.0-beta.1", "prerelease": true},
		{"tag_name": "v1.10.0", "draft": true},
		{"tag_name": "v1.9.0"},
		{"tag_name": "v1.8.4"}
	]`})

	got, err := newGitHub(t, srv).Recent(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var tags []string
	for _, r := range got {
		tags = append(tags, r.Tag)
	}
	if want := []string{"v1.9.0", "v1.8.4"}; !reflect.DeepEqual(tags, want) {
		t.Fatalf("got %q; want %q", tags, want)
	}
}
