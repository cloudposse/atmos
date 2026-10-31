package materialize

import (
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// checkLinkContainment resolves every symlink physically and rejects any that leaves root. A
// lexical check of each target is not enough, because a link chain can clean to a contained path
// yet resolve outside it. Dangling links stay allowed.
func checkLinkContainment(stage string, record *artifact.Record) error {
	realRoot, err := filepath.EvalSymlinks(stage)
	if err != nil {
		return fmt.Errorf("%w: resolve staging root: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	for index := range record.Inventory {
		entry := &record.Inventory[index]
		if entry.Type != artifact.EntrySymlink {
			continue
		}
		resolved, reachable := resolveLink(filepath.Join(stage, filepath.FromSlash(entry.Path)))
		if reachable && !within(realRoot, resolved) {
			return fmt.Errorf("%w: symlink %q resolves outside the package", errUtils.ErrArtifactUnsafePath, entry.Path)
		}
	}
	return nil
}

// resolveLink fully resolves a symlink and reports false for a dangling or looping link, which
// points at nothing reachable.
func resolveLink(linkPath string) (string, bool) {
	resolved, err := filepath.EvalSymlinks(linkPath)
	return resolved, err == nil
}

// within reports whether target is root or lies beneath it.
func within(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
