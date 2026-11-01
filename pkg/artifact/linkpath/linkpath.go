// Package linkpath resolves symlink chains physically so containment checks can see where a
// link really points, including links whose final components do not exist yet.
package linkpath

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// maxLinkHops bounds the symlinks followed while resolving one path, so a loop terminates.
const maxLinkHops = 255

// Resolve returns the physical absolute path that target refers to. It follows every symlink it
// meets, like filepath.EvalSymlinks. Unlike EvalSymlinks, it also resolves a path whose tail does
// not exist: the longest existing prefix is resolved physically, and the missing remainder is
// joined lexically. A dangling link therefore still reports where it would land once the missing
// path is created. Resolve reports false when the path loops, passes through a non-directory, or
// cannot be inspected.
func Resolve(target string) (string, bool) {
	defer perf.Track(nil, "linkpath.Resolve")()

	absolute, err := filepath.Abs(target)
	if err != nil {
		return "", false
	}
	volume := filepath.VolumeName(absolute)
	w := &walker{base: volume + string(filepath.Separator), pending: splitComponents(absolute[len(volume):])}
	w.resolved = w.base
	for len(w.pending) > 0 {
		done, ok := w.step()
		if done {
			return w.resolved, ok
		}
	}
	return w.resolved, true
}

// walker resolves a path one component at a time.
type walker struct {
	// base is the filesystem root, with its volume on Windows.
	base string

	// resolved is the physical directory reached so far.
	resolved string

	// pending holds the components still to resolve.
	pending []string

	// hops counts the symlinks followed.
	hops int
}

// step consumes one component. It reports done when resolution ended, and ok when it succeeded.
func (w *walker) step() (done, ok bool) {
	component := w.pending[0]
	w.pending = w.pending[1:]
	if component == "." || component == ".." {
		w.resolved = stepDot(w.resolved, component)
		return false, true
	}
	next := filepath.Join(w.resolved, component)
	info, err := os.Lstat(next)
	if errors.Is(err, fs.ErrNotExist) {
		// Nothing below a missing component can be a symlink, so the rest is lexical.
		w.resolved = filepath.Join(append([]string{next}, w.pending...)...)
		return true, true
	}
	if err != nil {
		return true, false
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		w.resolved = next
		return false, true
	}
	return w.follow(next)
}

// follow splices the target of the symlink at link in front of the pending components.
func (w *walker) follow(link string) (done, ok bool) {
	w.hops++
	components, absolute, err := linkComponents(link)
	if err != nil || w.hops > maxLinkHops {
		return true, false
	}
	if absolute {
		w.resolved = w.base
	}
	w.pending = append(components, w.pending...)
	return false, true
}

// stepDot applies a "." or ".." component to a fully resolved directory.
func stepDot(resolved, component string) string {
	if component == ".." {
		return filepath.Dir(resolved)
	}
	return resolved
}

// linkComponents reads the symlink at link and returns the components of its target and whether the
// target is absolute, in which case resolution restarts from the root.
func linkComponents(link string) ([]string, bool, error) {
	target, err := os.Readlink(link)
	if err != nil {
		return nil, false, err
	}
	absolute := filepath.IsAbs(target) || strings.HasPrefix(target, string(filepath.Separator))
	return splitComponents(target[len(filepath.VolumeName(target)):]), absolute, nil
}

// Within reports whether target is root or lies beneath it.
func Within(root, target string) bool {
	defer perf.Track(nil, "linkpath.Within")()

	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// splitComponents splits a path into its non-empty components.
func splitComponents(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == filepath.Separator })
}
