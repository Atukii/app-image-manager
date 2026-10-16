package source_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"appimage-manager/internal/source"
)

const (
	content       = "pretend AppImage bytes"
	contentDigest = "sha256:b87712afb7091924ca2d7bf12978b58d268826b96b0f80a424548902441e7ed5" // from shasum -a 256
)

func serve(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadSavesAnAssetWhoseDigestMatches(t *testing.T) {
	srv := serve(t, content)
	dest := filepath.Join(t.TempDir(), "Example.AppImage")

	err := source.Download(context.Background(), srv.Client(), source.Asset{URL: srv.URL, Digest: contentDigest}, dest, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(dest); string(got) != content {
		t.Fatalf("got %q; want %q", got, content)
	}
}

func TestDownloadRefusesAnAssetWhoseDigestDoesNotMatch(t *testing.T) {
	srv := serve(t, "tampered bytes")
	dest := filepath.Join(t.TempDir(), "Example.AppImage")

	err := source.Download(context.Background(), srv.Client(), source.Asset{URL: srv.URL, Digest: contentDigest}, dest, nil)

	if _, statErr := os.Stat(dest); !errors.Is(err, source.ErrDigestMismatch) || statErr == nil {
		t.Fatalf("got error %v and file present %v; want ErrDigestMismatch and no file", err, statErr == nil)
	}
}

func TestDownloadReportsProgressUpToTheFullSize(t *testing.T) {
	srv := serve(t, content)
	var last int64

	err := source.Download(context.Background(), srv.Client(), source.Asset{URL: srv.URL, Size: int64(len(content))},
		filepath.Join(t.TempDir(), "Example.AppImage"), func(done, total int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}

	if last != 22 {
		t.Fatalf("got final progress %d; want 22 bytes", last)
	}
}
