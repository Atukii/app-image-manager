package source

import (
	"context"
	"time"
)

// ReleaseSource is where AiM looks for new Releases of a Managed App.
type ReleaseSource interface {
	// Latest returns the latest Release. Prereleases are never returned.
	Latest(ctx context.Context) (Release, error)
	// Recent returns the most recent Releases, newest first, without
	// prereleases. It is used to identify an adopted AppImage's Version.
	Recent(ctx context.Context) ([]Release, error)
}

// Release is one published version of an app.
type Release struct {
	Tag       string
	Published time.Time
	Assets    []Asset
}

// Asset is one downloadable file attached to a Release.
type Asset struct {
	Name string
	URL  string
	Size int64
	// Digest is "sha256:<hex>" when the Release Source publishes one, and
	// empty otherwise.
	Digest string
}
