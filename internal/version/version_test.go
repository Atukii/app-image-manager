package version_test

import (
	"testing"
	"time"

	"appimage-manager/internal/version"
)

func TestNewerPatchReleaseIsNewer(t *testing.T) {
	installed := version.Release{Tag: "v0.12.6"}
	candidate := version.Release{Tag: "v0.12.7"}

	if !version.Newer(candidate, installed) {
		t.Fatal("want v0.12.7 to be newer than v0.12.6")
	}
}

func TestNewerTreatsMissingPatchAsZero(t *testing.T) {
	for _, pair := range [][2]string{{"v1.2", "v1.2.0"}, {"v1.2.0", "v1.2"}} {
		candidate, installed := version.Release{Tag: pair[0]}, version.Release{Tag: pair[1]}

		if version.Newer(candidate, installed) {
			t.Errorf("want %s and %s to be the same Version", pair[0], pair[1])
		}
	}
}

func TestNewerFallsBackToPublishDateForNonVersionTags(t *testing.T) {
	installed := version.Release{Tag: "build-471", Published: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	candidate := version.Release{Tag: "build-482", Published: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}

	if !version.Newer(candidate, installed) {
		t.Fatal("want a later Release with a different non-version tag to be newer")
	}
}

func TestFromFilenameFindsTheVersion(t *testing.T) {
	got, ok := version.FromFilename("Obsidian-1.8.4.AppImage")

	if !ok || got != "1.8.4" {
		t.Fatalf("got %q, %v; want \"1.8.4\", true", got, ok)
	}
}

func TestFromFilenameIgnoresArchitectureNames(t *testing.T) {
	if got, ok := version.FromFilename("nvim-linux-x86_64.appimage"); ok {
		t.Fatalf("got %q; want no Version in an architecture-only filename", got)
	}
}
