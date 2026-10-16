package asset_test

import (
	"testing"

	"appimage-manager/internal/asset"
)

func TestPatternReplacesTheVersionWithAWildcard(t *testing.T) {
	got := asset.Pattern("Obsidian-1.8.4.AppImage", "v1.8.4")

	if got != "Obsidian-*.AppImage" {
		t.Fatalf("got %q; want \"Obsidian-*.AppImage\"", got)
	}
}

func TestPickWithPatternSkipsOtherArchitectures(t *testing.T) {
	names := []string{
		"Obsidian-1.9.0-arm64.AppImage",
		"Obsidian-1.9.0.AppImage",
		"obsidian_1.9.0_amd64.deb",
	}

	got, _ := asset.Pick(names, "Obsidian-*.AppImage")

	if got != "Obsidian-1.9.0.AppImage" {
		t.Fatalf("got %q; want \"Obsidian-1.9.0.AppImage\"", got)
	}
}

func TestPickLeavesTheChoiceToTheUserWhenSeveralAppImagesMatch(t *testing.T) {
	names := []string{"App-1.0.AppImage", "App-1.0-nosandbox.AppImage", "App-1.0.AppImage.zsync"}

	got, candidates := asset.Pick(names, "")

	if got != "" || len(candidates) != 2 {
		t.Fatalf("got %q with candidates %q; want no choice and 2 candidates", got, candidates)
	}
}
