package artifact

import (
	"fmt"
	"path"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// maxSymlinkExpansions bounds how many in-package links one target may traverse. It matches the
// limit that common operating systems apply before reporting a symlink loop.
const maxSymlinkExpansions = 40

// validateSymlinkTargets resolves every symlink target against the inventory, so a target that is
// lexically contained but passes through an in-package link cannot leave the package. For example
// "link -> dir/up/../outside" cleans to "dir/outside", yet when "dir/up -> .." is also in the
// inventory, the filesystem follows "dir/up" to the package root before applying "..".
//
// Resolution follows the way a filesystem walks a path: component by component, replacing a link
// with its own target, and rejecting a ".." that would pop above the package root. Paths that name
// nothing in the inventory resolve literally, so dangling targets stay valid. The expansion limit
// rejects link cycles of any shape.
func validateSymlinkTargets(inventory []Entry) error {
	links := make(map[string]string)
	for _, entry := range inventory {
		if entry.Type == EntrySymlink {
			links[entry.Path] = entry.Target
		}
	}
	if len(links) == 0 {
		return nil
	}
	for index := range inventory {
		if inventory[index].Type != EntrySymlink {
			continue
		}
		if err := resolveSymlink(links, &inventory[index]); err != nil {
			return err
		}
	}
	return nil
}

// resolveSymlink walks the target of one link through the in-package links.
func resolveSymlink(links map[string]string, link *Entry) error {
	var resolved []string
	if dir := path.Dir(link.Path); dir != "." {
		resolved = strings.Split(dir, "/")
	}
	pending := strings.Split(link.Target, "/")
	for expansions := 0; len(pending) > 0; {
		component := pending[0]
		pending = pending[1:]
		switch component {
		case "", ".":
			continue
		case "..":
			if len(resolved) == 0 {
				return fmt.Errorf("%w: symlink %q target %q escapes the package", errUtils.ErrArtifactUnsafePath, link.Path, link.Target)
			}
			resolved = resolved[:len(resolved)-1]
			continue
		}
		candidate := strings.Join(append(resolved[:len(resolved):len(resolved)], component), "/")
		target, isLink := links[candidate]
		if !isLink {
			resolved = append(resolved, component)
			continue
		}
		expansions++
		if expansions > maxSymlinkExpansions {
			return fmt.Errorf("%w: symlink %q target %q loops or traverses more than %d links", errUtils.ErrArtifactUnsafePath, link.Path, link.Target, maxSymlinkExpansions)
		}
		// A link is replaced by its target, resolved from the directory that holds the link.
		pending = append(strings.Split(target, "/"), pending...)
	}
	return nil
}
