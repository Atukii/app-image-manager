// Package version decides whether one Release is newer than another.
package version

import (
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Release is the part of a Release that matters for comparing Versions.
type Release struct {
	Tag       string
	Published time.Time
}

// Newer reports whether candidate is a newer Release than installed.
//
// Tags are compared as semantic versions (a leading "v" is optional). If
// either tag is not a valid version, candidate is newer when its tag differs
// and it was published later.
func Newer(candidate, installed Release) bool {
	c, i := canonical(candidate.Tag), canonical(installed.Tag)
	if semver.IsValid(c) && semver.IsValid(i) {
		return semver.Compare(c, i) > 0
	}
	return candidate.Tag != installed.Tag && candidate.Published.After(installed.Published)
}

func canonical(tag string) string {
	return "v" + strings.TrimPrefix(tag, "v")
}

var inFilename = regexp.MustCompile(`\d+(\.\d+)+`)

// FromFilename guesses a Version from an AppImage's filename.
func FromFilename(name string) (string, bool) {
	v := inFilename.FindString(name)
	return v, v != ""
}
