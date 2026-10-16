package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"appimage-manager/internal/appimage"
	"appimage-manager/internal/state"
)

// Found is an AppImage that AiM doesn't manage yet.
type Found struct {
	Path string `json:"path"`
	// SourceURL is suggested from the AppImage's update information; empty
	// when it has none AiM understands (SCAN-3).
	SourceURL string `json:"sourceUrl,omitempty"`
}

// Scan looks in the ScanDirs for AppImages that are neither Managed Apps
// nor Ignored AppImages (SCAN-1).
func (c *Core) Scan(ctx context.Context) ([]Found, error) {
	st, err := c.store.Load()
	if err != nil {
		return nil, err
	}
	ignored := map[string]bool{}
	for _, path := range st.Ignored {
		ignored[path] = true
	}
	managed := map[string]bool{}
	for id := range st.Apps {
		managed[c.appFolder(id)] = true
	}
	var found []Found
	for _, dir := range c.deps.Paths.ScanDirs {
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			switch {
			case err != nil:
				return nil
			case d.IsDir() && managed[path]:
				return filepath.SkipDir
			case d.IsDir() && path != dir && filepath.Dir(path) != dir:
				return filepath.SkipDir // one level of subfolders, like App Folders
			case !d.IsDir() && strings.HasSuffix(strings.ToLower(path), ".appimage") && !ignored[path]:
				found = append(found, Found{Path: path, SourceURL: suggestSource(path)})
			}
			return nil
		})
	}
	return found, nil
}

// suggestSource turns GitHub update information
// ("gh-releases-zsync|owner|repo|…") into the repository's URL.
func suggestSource(path string) string {
	info, err := appimage.Inspect(path)
	if err != nil {
		return ""
	}
	parts := strings.Split(info.UpdateInfo, "|")
	if len(parts) >= 3 && strings.HasPrefix(parts[0], "gh-releases") && parts[1] != "" && parts[2] != "" {
		return "https://github.com/" + parts[1] + "/" + parts[2]
	}
	return ""
}

// Ignore makes path an Ignored AppImage: Scans won't offer it again.
func (c *Core) Ignore(ctx context.Context, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	_, err = c.store.Update(func(s *state.State) error {
		if !slices.Contains(s.Ignored, abs) {
			s.Ignored = append(s.Ignored, abs)
		}
		return nil
	})
	return err
}
