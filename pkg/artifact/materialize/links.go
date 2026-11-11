package materialize

import (
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
	for index := range record.Inventory {
		entry := &record.Inventory[index]
		if entry.Type != artifact.EntrySymlink {
			continue
		}
		resolved, reachable := linkpath.Resolve(filepath.Join(stage, filepath.FromSlash(entry.Path)))
		if reachable && !linkpath.Within(realRoot, resolved) {
			return fmt.Errorf("%w: symlink %q resolves outside the package", errUtils.ErrArtifactUnsafePath, entry.Path)
		}
	}
	return nil
}
