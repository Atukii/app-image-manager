package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"
)

type direct struct {
	ref    Ref
	client *http.Client
	now    func() time.Time
}

// NewDirect returns the Release Source for a Direct URL: one stable link
// with no version numbers. A nil client or now uses the defaults.
func NewDirect(ref Ref, client *http.Client, now func() time.Time) ReleaseSource {
	if client == nil {
		client = http.DefaultClient
	}
	if now == nil {
		now = time.Now
	}
	return &direct{ref: ref, client: client, now: now}
}

// Latest describes the file currently at the link. Its Tag fingerprints
// the ETag, Last-Modified and Content-Length headers, so it changes exactly
// when the file does (CHK-3). Published is the time of the lookup, so a
// changed Tag always counts as newer.
func (d *direct) Latest(ctx context.Context) (Release, error) {
	resp, err := d.head(ctx)
	if err != nil {
		return Release{}, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("%s: %s", d.ref.URL, resp.Status)
	}
	fingerprint := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%d",
		resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), resp.ContentLength))

	name := d.ref.URL
	if u, err := url.Parse(d.ref.URL); err == nil {
		name = path.Base(u.Path)
	}
	return Release{
		Tag:       "direct-" + hex.EncodeToString(fingerprint[:6]),
		Published: d.now(),
		Assets:    []Asset{{Name: name, URL: d.ref.URL, Size: max(resp.ContentLength, 0)}},
	}, nil
}

// Recent is just the Latest Release: a Direct URL has no history.
func (d *direct) Recent(ctx context.Context) ([]Release, error) {
	rel, err := d.Latest(ctx)
	if err != nil {
		return nil, err
	}
	return []Release{rel}, nil
}

// head asks for the link's headers, falling back to GET for servers that
// refuse HEAD.
func (d *direct) head(ctx context.Context) (*http.Response, error) {
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		req, err := http.NewRequestWithContext(ctx, method, d.ref.URL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := d.client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusMethodNotAllowed || method == http.MethodGet {
			return resp, nil
		}
		resp.Body.Close()
	}
	panic("unreachable")
}
