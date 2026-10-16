package source_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"appimage-manager/internal/source"
)

func TestDirectTagChangesExactlyWhenTheFileChanges(t *testing.T) {
	etag := `"build-1"`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", "Wed, 01 Oct 2026 12:00:00 GMT")
		w.Header().Set("Content-Length", "123456")
	}))
	defer srv.Close()
	direct := source.NewDirect(source.Ref{Kind: source.Direct, URL: srv.URL + "/latest/Example.AppImage"}, srv.Client(), nil)
	tag := func() string {
		rel, err := direct.Latest(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return rel.Tag
	}

	first, unchanged := tag(), tag()
	etag = `"build-2"`
	changed := tag()

	if first != unchanged || unchanged == changed {
		t.Fatalf("got tags %q, %q, %q; want the first two equal and the third different", first, unchanged, changed)
	}
}

func TestDirectOffersTheLinkedFileAsTheOnlyAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	link := srv.URL + "/latest/Example.AppImage"

	rel, err := source.NewDirect(source.Ref{Kind: source.Direct, URL: link}, srv.Client(), nil).Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(rel.Assets) != 1 || rel.Assets[0].Name != "Example.AppImage" || rel.Assets[0].URL != link {
		t.Fatalf("got assets %+v; want only Example.AppImage at %s", rel.Assets, link)
	}
}
