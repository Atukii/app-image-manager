package testkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"appimage-manager/internal/source"
)

// Releases is a fake Release Source: tests publish Releases whose Assets
// are fake AppImages, served with their Digests from a local HTTP server.
type Releases struct {
	t        *testing.T
	Server   *httptest.Server
	files    map[string][]byte // URL path → content
	releases []source.Release  // newest first
	// Downloads counts Asset downloads.
	Downloads int
	// Failures is how many of the next Latest lookups fail.
	Failures int
	// FailWith, when set, makes every Latest lookup fail with it.
	FailWith error
	// NoDigests publishes Assets without Digests, as older releases and
	// Direct URLs have.
	NoDigests bool
}

// NewReleases starts a fake Release Source, stopped when the test ends.
func NewReleases(t *testing.T) *Releases {
	t.Helper()
	r := &Releases{t: t, files: map[string][]byte{}}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		data, ok := r.files[req.URL.Path]
		if !ok {
			http.NotFound(w, req)
			return
		}
		r.Downloads++
		w.Write(data)
	}))
	t.Cleanup(r.Server.Close)
	return r
}

// Publish adds a Release whose Assets are the given AppImages and returns
// their contents by name.
func (r *Releases) Publish(tag string, published time.Time, assets map[string]AppImage) map[string][]byte {
	r.t.Helper()
	rel := source.Release{Tag: tag, Published: published}
	contents := map[string][]byte{}
	for name, img := range assets {
		data, err := os.ReadFile(img.Build(r.t))
		if err != nil {
			r.t.Fatal(err)
		}
		urlPath := "/download/" + tag + "/" + name
		r.files[urlPath] = data
		sum := sha256.Sum256(data)
		a := source.Asset{Name: name, URL: r.Server.URL + urlPath, Size: int64(len(data)), Digest: "sha256:" + hex.EncodeToString(sum[:])}
		if r.NoDigests {
			a.Digest = ""
		}
		rel.Assets = append(rel.Assets, a)
		contents[name] = data
	}
	r.releases = append([]source.Release{rel}, r.releases...)
	return contents
}

// Tamper changes the bytes served for a published Asset, so they no longer
// match its Digest.
func (r *Releases) Tamper(tag, name string) {
	r.files["/download/"+tag+"/"+name] = []byte("tampered")
}

// Latest returns the newest Release.
func (r *Releases) Latest(context.Context) (source.Release, error) {
	if r.FailWith != nil {
		return source.Release{}, r.FailWith
	}
	if r.Failures > 0 {
		r.Failures--
		return source.Release{}, errors.New("dial tcp: lookup api.github.com: no such host")
	}
	if len(r.releases) == 0 {
		return source.Release{}, os.ErrNotExist
	}
	return r.releases[0], nil
}

// Recent returns every Release, newest first.
func (r *Releases) Recent(context.Context) ([]source.Release, error) { return r.releases, nil }
