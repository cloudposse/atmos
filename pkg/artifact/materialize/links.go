package materialize

import (
	"errors"
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/linkpath"
)

// checkLinkContainment resolves every symlink physically and rejects any that leaves root. A
// lexical check of each target is not enough, because a link chain can clean to a contained path
// yet resolve outside it. A dangling link is checked through its longest existing ancestor plus the
// missing remainder, so it cannot escape once the missing path is created.
func checkLinkContainment(stage string, record *artifact.Record) error {
	realRoot, err := filepath.EvalSymlinks(stage)
	if err != nil {
		return fmt.Errorf("%w: resolve staging root: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	recorded := make(map[string]string)
	for index := range record.Inventory {
		if entry := &record.Inventory[index]; entry.Type == artifact.EntrySymlink {
			recorded[entry.Path] = entry.Target
		}
	}
	for index := range record.Inventory {
		entry := &record.Inventory[index]
		if entry.Type != artifact.EntrySymlink {
			continue
		}
		resolved, err := linkpath.Resolve(filepath.Join(stage, filepath.FromSlash(entry.Path)))
		switch {
		case errors.Is(err, errUtils.ErrSymlinkLoop):
			// A loop never reaches a path outside the root, so it is not an escape.
			continue
		case errors.Is(err, errUtils.ErrPathThroughNonDirectory):
			// The target runs through a regular file, so it can never resolve to anything. Capture accepts
			// such a link when its recorded path stays inside the tree, and so does materialization: the
			// same lexical check, over the recorded links, decides.
			if linkpath.Escapes(recorded, entry.Path) {
				return fmt.Errorf("%w: symlink %q resolves outside the package", errUtils.ErrArtifactUnsafePath, entry.Path)
			}
			continue
		case err != nil:
			// Any other failure leaves the target unknown, and an unknown target cannot be shown to stay inside.
			return fmt.Errorf("%w: symlink %q cannot be resolved: %w", errUtils.ErrArtifactUnsafePath, entry.Path, err)
		case !linkpath.Within(realRoot, resolved):
			return fmt.Errorf("%w: symlink %q resolves outside the package", errUtils.ErrArtifactUnsafePath, entry.Path)
		}
	}
	return nil
}
