// Package linkpath resolves symlink chains physically so containment checks can see where a
// link really points, including links whose final components do not exist yet.
package linkpath

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// maxLinkHops bounds the symlinks followed while resolving one path, so a loop terminates.
const (
	maxLinkHops     = 255
	parentComponent = ".."
)

// Resolve returns the physical absolute path that target refers to. It follows every symlink it
// meets, like filepath.EvalSymlinks. Unlike EvalSymlinks, it also resolves a path whose tail does
// not exist: the longest existing prefix is resolved physically, and the missing remainder is
// joined lexically. A dangling link therefore still reports where it would land once the missing
// path is created. Parent components are applied only after the symlinks before them are resolved,
// so "link/../x" is not collapsed to a sibling of "link".
//
// Resolve fails with [errUtils.ErrSymlinkLoop] when the links form a loop, with
// [errUtils.ErrPathThroughNonDirectory] when the path passes through a non-directory, and with another
// error when it cannot be inspected. Callers decide separately
// what each failure means.
func Resolve(target string) (string, error) {
	defer perf.Track(nil, "linkpath.Resolve")()

	absolute, err := absolutePath(target)
	if err != nil {
		return "", fmt.Errorf("%w: make %q absolute: %w", errUtils.ErrArtifactUnsafePath, target, err)
	}
	volume := filepath.VolumeName(absolute)
	w := &walker{base: volume + string(filepath.Separator), pending: splitComponents(absolute[len(volume):])}
	w.resolved = w.base
	for len(w.pending) > 0 {
		if err := w.step(); err != nil {
			return "", err
		}
	}
	return filepath.Join(append([]string{w.resolved}, w.missing...)...), nil
}

// absolutePath makes target absolute without cleaning it: filepath.Abs would collapse parent
// components lexically, before any symlink in front of them has been resolved.
func absolutePath(target string) (string, error) {
	if filepath.IsAbs(target) {
		return target, nil
	}
	working, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return working + string(filepath.Separator) + target, nil
}

// Escapes reports whether the symlink at from leaves the tree that links describes. The tree is
// recorded, not read from disk: links maps the slash-separated path of every symlink, relative to
// the tree root, to its target. Resolution follows the recorded links exactly like Resolve follows
// physical ones (a dangling tail is resolved lexically), so the answer cannot change when the
// files the links were read from change afterwards. A link chain that loops does not escape.
func Escapes(links map[string]string, from string) bool {
	defer perf.Track(nil, "linkpath.Escapes")()

	_, outcome := followRecorded(links, from, ignoreVisit)
	return outcome == recordedEscaped
}

// ResolveRecorded returns the slash-separated path, relative to the tree root, that from refers to
// once every recorded link on the way is followed. Like Escapes it reads only links, never the
// disk, so a caller can test where a captured link lands without looking at the source again. The
// root itself resolves to the empty path. ResolveRecorded reports false when the chain leaves the
// tree or loops, because neither names a path inside the tree.
func ResolveRecorded(links map[string]string, from string) (string, bool) {
	defer perf.Track(nil, "linkpath.ResolveRecorded")()

	resolved, outcome := followRecorded(links, from, ignoreVisit)
	if outcome != recordedInside {
		return "", false
	}
	return strings.Join(resolved, "/"), true
}

// TraversedRecorded returns, in the order the filesystem looks them up, the slash-separated path of
// every entry that resolving from through the recorded links passes through, relative to the tree
// root. A parent component is applied only after the directory before it was looked up, so
// "cache/p/../x" traverses "cache" and "cache/p" even though it reduces to "x". Paths reached
// through a recorded link are reported by the physical location they resolve to. It reports false
// when the chain leaves the tree or loops.
func TraversedRecorded(links map[string]string, from string) ([]string, bool) {
	defer perf.Track(nil, "linkpath.TraversedRecorded")()

	var traversed []string
	_, outcome := followRecorded(links, from, func(visited string) { traversed = append(traversed, visited) })
	return traversed, outcome == recordedInside
}

// ignoreVisit is the visitor of callers that only need where a path ends up.
func ignoreVisit(string) {}

// recordedOutcome is how the resolution of a path through recorded links ended.
type recordedOutcome int

const (
	// The path resolved to a location inside the tree.
	recordedInside recordedOutcome = iota
	// The path climbed above the tree root.
	recordedEscaped
	// The link chain did not terminate.
	recordedLooped
)

// followRecorded resolves from through the recorded links and returns the components reached. It
// calls visit with the path of each entry it looks up that is not itself a recorded link.
func followRecorded(links map[string]string, from string, visit func(string)) ([]string, recordedOutcome) {
	pending := splitComponents(from)
	var resolved []string
	for hops := 0; len(pending) > 0; {
		component := pending[0]
		pending = pending[1:]
		switch component {
		case ".":
		case parentComponent:
			if len(resolved) == 0 {
				return nil, recordedEscaped
			}
			resolved = resolved[:len(resolved)-1]
		default:
			current := strings.Join(append(resolved, component), "/")
			target, isLink := links[current]
			if !isLink {
				resolved = append(resolved, component)
				visit(current)
				continue
			}
			hops++
			if hops > maxLinkHops {
				return nil, recordedLooped
			}
			pending = append(splitComponents(target), pending...)
		}
	}
	return resolved, recordedInside
}

// walker resolves a path one component at a time.
type walker struct {
	// base is the filesystem root, with its volume on Windows.
	base string

	// resolved is the physical directory reached so far.
	resolved string

	// pending holds the components still to resolve.
	pending []string

	// missing holds the lexical tail below the first missing component.
	missing []string

	// hops counts the symlinks followed.
	hops int
}

// step consumes one component. It returns nil to continue and an error when resolution failed.
func (w *walker) step() error {
	component := w.pending[0]
	w.pending = w.pending[1:]
	if len(w.missing) > 0 {
		w.stepMissing(component)
		return nil
	}
	if component == "." || component == parentComponent {
		w.resolved = stepDot(w.resolved, component)
		return nil
	}
	next := filepath.Join(w.resolved, component)
	info, err := os.Lstat(next)
	if errors.Is(err, fs.ErrNotExist) {
		w.missing = append(w.missing, component)
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: inspect %q: %w", errUtils.ErrArtifactUnsafePath, next, err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		if !info.IsDir() && len(w.pending) > 0 {
			return fmt.Errorf("%w: %w: %q is not a directory", errUtils.ErrArtifactUnsafePath, errUtils.ErrPathThroughNonDirectory, next)
		}
		w.resolved = next
		return nil
	}
	return w.follow(next)
}

// stepMissing resolves the missing tail lexically until it returns to an existing directory.
func (w *walker) stepMissing(component string) {
	switch component {
	case ".":
	case parentComponent:
		w.missing = w.missing[:len(w.missing)-1]
	default:
		w.missing = append(w.missing, component)
	}
}

// follow splices the target of the symlink at link in front of the pending components.
func (w *walker) follow(link string) error {
	w.hops++
	if w.hops > maxLinkHops {
		return fmt.Errorf("%w: more than %d links followed at %q", errUtils.ErrSymlinkLoop, maxLinkHops, link)
	}
	components, absolute, err := linkComponents(link)
	if err != nil {
		return fmt.Errorf("%w: read link %q: %w", errUtils.ErrArtifactUnsafePath, link, err)
	}
	if absolute {
		w.resolved = w.base
	}
	w.pending = append(components, w.pending...)
	return nil
}

// stepDot applies a current or parent directory component to a fully resolved directory.
func stepDot(resolved, component string) string {
	if component == parentComponent {
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
	return err == nil && relative != parentComponent && !strings.HasPrefix(relative, parentComponent+string(filepath.Separator))
}

// splitComponents splits a path into its non-empty components.
func splitComponents(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == filepath.Separator })
}
