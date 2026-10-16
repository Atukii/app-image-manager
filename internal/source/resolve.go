// Package source finds Releases at a Release Source and downloads their
// Assets.
package source

import (
	"fmt"
	"net/url"
	"strings"
)

// Kind is the type of Release Source.
type Kind string

const (
	GitHub Kind = "github"
	Direct Kind = "direct"
)

// Ref identifies a Release Source, plus what a Source URL said about the
// Release and Asset it pointed at.
type Ref struct {
	Kind Kind
	// GitHub only.
	Owner, Repo string
	// Tag and Asset are set when the Source URL named a Release or an Asset.
	Tag, Asset string
	// URL is the download link of a Direct URL source.
	URL string
}

// Resolve works out which Release Source a Source URL refers to.
func Resolve(sourceURL string) (Ref, error) {
	u, err := url.Parse(sourceURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return Ref{}, fmt.Errorf("%q is not an http(s) URL", sourceURL)
	}
	if host := strings.TrimPrefix(strings.ToLower(u.Host), "www."); host != "github.com" {
		return Ref{Kind: Direct, URL: sourceURL}, nil
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return Ref{}, fmt.Errorf("%q does not name a GitHub repository", sourceURL)
	}
	ref := Ref{Kind: GitHub, Owner: parts[0], Repo: strings.TrimSuffix(parts[1], ".git")}
	switch {
	case len(parts) == 6 && parts[2] == "releases" && parts[3] == "download":
		ref.Tag, ref.Asset = parts[4], parts[5]
	case len(parts) == 5 && parts[2] == "releases" && parts[3] == "tag":
		ref.Tag = parts[4]
	}
	return ref, nil
}
