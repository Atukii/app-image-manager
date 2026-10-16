package source_test

import (
	"testing"

	"appimage-manager/internal/source"
)

func TestResolveReadsRepoTagAndAssetFromADirectDownloadLink(t *testing.T) {
	got, err := source.Resolve("https://github.com/neovim/neovim/releases/download/v0.12.6/nvim-linux-x86_64.appimage")
	if err != nil {
		t.Fatal(err)
	}

	want := source.Ref{Kind: source.GitHub, Owner: "neovim", Repo: "neovim", Tag: "v0.12.6", Asset: "nvim-linux-x86_64.appimage"}
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestResolveFindsTheRepositoryFromAnyGitHubPage(t *testing.T) {
	for sourceURL, tag := range map[string]string{
		"https://github.com/obsidianmd/obsidian-releases":                     "",
		"https://github.com/obsidianmd/obsidian-releases.git":                 "",
		"https://github.com/obsidianmd/obsidian-releases/releases":            "",
		"https://github.com/obsidianmd/obsidian-releases/releases/latest":     "",
		"https://www.github.com/obsidianmd/obsidian-releases/releases/":       "",
		"https://github.com/obsidianmd/obsidian-releases/releases/tag/v1.8.4": "v1.8.4",
	} {
		got, err := source.Resolve(sourceURL)
		if err != nil {
			t.Errorf("%s: %v", sourceURL, err)
			continue
		}

		want := source.Ref{Kind: source.GitHub, Owner: "obsidianmd", Repo: "obsidian-releases", Tag: tag}
		if got != want {
			t.Errorf("%s: got %+v; want %+v", sourceURL, got, want)
		}
	}
}

func TestResolveTreatsOtherHostsAsADirectURL(t *testing.T) {
	const link = "https://downloads.example.org/latest/Example.AppImage"

	got, err := source.Resolve(link)
	if err != nil {
		t.Fatal(err)
	}

	if want := (source.Ref{Kind: source.Direct, URL: link}); got != want {
		t.Fatalf("got %+v; want %+v", got, want)
	}
}
