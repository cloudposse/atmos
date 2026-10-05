package managers

import "github.com/cloudposse/atmos/pkg/perf"

// DuplicatePath returns the first path that appears more than once in paths,
// or "" if every path is unique. Shared by every file manager whose set
// entries carry a Path (json, yaml), so "two set entries can't target the
// same field" behavior stays consistent across manager types.
func DuplicatePath(paths []string) string {
	defer perf.Track(nil, "managers.DuplicatePath")()

	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		if seen[p] {
			return p
		}
		seen[p] = true
	}
	return ""
}
