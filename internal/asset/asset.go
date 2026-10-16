// Package asset derives Asset Patterns and picks the right Asset from a Release.
package asset

import (
	"path"
	"strings"
)

// otherArchitectures are names that mark an Asset as built for a CPU other
// than x86_64, the only architecture AiM supports.
var otherArchitectures = []string{"aarch64", "arm64", "armhf", "armv7l", "i386", "i686"}

// Pattern derives the Asset Pattern for filename, the Asset installed from the
// Release tagged version: the Version in the filename becomes "*".
func Pattern(filename, version string) string {
	return strings.Replace(filename, strings.TrimPrefix(version, "v"), "*", 1)
}

// Pick chooses the Asset to install from a Release's Asset names. Candidates
// are x86_64 AppImages matching pattern (any AppImage when pattern is empty);
// matching ignores case. chosen is set only when exactly one candidate is
// left, otherwise the user has to choose among candidates.
func Pick(names []string, pattern string) (chosen string, candidates []string) {
	for _, name := range names {
		if isCandidate(strings.ToLower(name), strings.ToLower(pattern)) {
			candidates = append(candidates, name)
		}
	}
	if len(candidates) == 1 {
		chosen = candidates[0]
	}
	return chosen, candidates
}

func isCandidate(name, pattern string) bool {
	if !strings.HasSuffix(name, ".appimage") {
		return false
	}
	for _, arch := range otherArchitectures {
		if strings.Contains(name, arch) {
			return false
		}
	}
	if pattern == "" {
		return true
	}
	matched, _ := path.Match(pattern, name)
	return matched
}
