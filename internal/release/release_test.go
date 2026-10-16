package release_test

import (
	"testing"

	"appimage-manager/internal/release"
)

func TestNext(t *testing.T) {
	for _, c := range []struct {
		name    string
		current string
		commits []string
		want    string
	}{
		{"a fix is a patch", "v1.2.3", []string{"fix: keep the icon on rollback"}, "v1.2.4"},
		{"perf is a patch", "v1.2.3", []string{"perf: cache release lookups"}, "v1.2.4"},
		{"a feature is a minor", "v1.2.3", []string{"fix: a", "feat(core): add aim skip"}, "v1.3.0"},
		{"! is a major", "v1.2.3", []string{"feat(cli)!: rename --poll"}, "v2.0.0"},
		{"a BREAKING CHANGE footer is a major", "v1.2.3", []string{"fix: change the state format\n\nBREAKING CHANGE: apps.json moves"}, "v2.0.0"},
		{"before 1.0 a breaking change is a minor", "v0.4.1", []string{"feat!: new protocol"}, "v0.5.0"},
		{"docs, tests and CI don't release", "v1.2.3", []string{"docs: README", "test: more", "ci: pin actions", "refactor: tidy", "chore: deps", "build: go 1.26", "style: gofmt"}, ""},
		{"the first release is v0.1.0", "", []string{"feat: add apps"}, "v0.1.0"},
		{"the first release needs something releasable", "", []string{"docs: plan"}, ""},
		{"anything not conventional doesn't release", "v1.2.3", []string{"Merge branch 'x'", "wip"}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := release.Next(c.current, c.commits); got != c.want {
				t.Fatalf("Next(%q, %q) = %q; want %q", c.current, c.commits, got, c.want)
			}
		})
	}
}
