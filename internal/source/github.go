package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ErrRateLimited means GitHub's API rate limit is used up; a token raises it.
var ErrRateLimited = errors.New("GitHub API rate limit exceeded")

// freshFor is how long a cached API response is used without asking GitHub
// again. Without a token even a 304 counts against the rate limit, so
// replayed aim calls (ADR 0008) must not send requests at all.
const freshFor = 10 * time.Minute

// GitHubOptions configures the GitHub Release Source.
type GitHubOptions struct {
	// APIBase defaults to https://api.github.com.
	APIBase string
	Client  *http.Client
	// Token is an optional GitHub token, raising the API rate limit.
	Token string
	// CacheDir holds API responses, shared between aim processes. Empty
	// disables caching.
	CacheDir string
	Now      func() time.Time
}

type gitHub struct {
	ref  Ref
	opts GitHubOptions
}

// NewGitHub returns the Release Source for a GitHub repository.
func NewGitHub(ref Ref, opts GitHubOptions) ReleaseSource {
	if opts.APIBase == "" {
		opts.APIBase = "https://api.github.com"
	}
	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &gitHub{ref: ref, opts: opts}
}

type apiRelease struct {
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	Assets      []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
	} `json:"assets"`
}

func (r apiRelease) release() Release {
	rel := Release{Tag: r.TagName, Published: r.PublishedAt}
	for _, a := range r.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size, Digest: a.Digest})
	}
	return rel
}

func (g *gitHub) Latest(ctx context.Context) (Release, error) {
	var r apiRelease
	if err := g.get(ctx, "/releases/latest", &r); err != nil {
		return Release{}, err
	}
	return r.release(), nil
}

// recentCount is how many Releases Recent looks at: one API page.
const recentCount = 30

func (g *gitHub) Recent(ctx context.Context) ([]Release, error) {
	var rs []apiRelease
	if err := g.get(ctx, fmt.Sprintf("/releases?per_page=%d", recentCount), &rs); err != nil {
		return nil, err
	}
	var releases []Release
	for _, r := range rs {
		if !r.Prerelease && !r.Draft {
			releases = append(releases, r.release())
		}
	}
	return releases, nil
}

// cached is an API response kept in CacheDir.
type cached struct {
	Fetched time.Time `json:"fetched"`
	ETag    string    `json:"etag"`
	Body    []byte    `json:"body"`
}

// get fetches a repository API path into v, using the cache when it can.
func (g *gitHub) get(ctx context.Context, path string, v any) error {
	url := fmt.Sprintf("%s/repos/%s/%s%s", g.opts.APIBase, g.ref.Owner, g.ref.Repo, path)
	entry, hit := g.readCache(url)
	if hit && g.opts.Now().Sub(entry.Fetched) < freshFor {
		return json.Unmarshal(entry.Body, v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.opts.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.opts.Token)
	}
	if hit && entry.ETag != "" {
		req.Header.Set("If-None-Match", entry.ETag)
	}
	resp, err := g.opts.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified && hit:
	case resp.StatusCode == http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		entry = cached{ETag: resp.Header.Get("ETag"), Body: body}
	case (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) &&
		resp.Header.Get("X-RateLimit-Remaining") == "0":
		return fmt.Errorf("GitHub %s: %w", url, ErrRateLimited)
	default:
		return fmt.Errorf("GitHub %s: %s", url, resp.Status)
	}
	entry.Fetched = g.opts.Now()
	g.writeCache(url, entry)
	return json.Unmarshal(entry.Body, v)
}

func (g *gitHub) cachePath(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(g.opts.CacheDir, "github-"+hex.EncodeToString(sum[:8])+".json")
}

func (g *gitHub) readCache(url string) (cached, bool) {
	if g.opts.CacheDir == "" {
		return cached{}, false
	}
	data, err := os.ReadFile(g.cachePath(url))
	if err != nil {
		return cached{}, false
	}
	var entry cached
	if json.Unmarshal(data, &entry) != nil {
		return cached{}, false
	}
	return entry, true
}

// writeCache stores entry; a failure only costs a request next time.
func (g *gitHub) writeCache(url string, entry cached) {
	if g.opts.CacheDir == "" {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil || os.MkdirAll(g.opts.CacheDir, 0o755) != nil {
		return
	}
	tmp, err := os.CreateTemp(g.opts.CacheDir, "github-*.tmp")
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr != nil || cerr != nil || os.Rename(tmp.Name(), g.cachePath(url)) != nil {
		os.Remove(tmp.Name())
	}
}
