package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// SafeJoin resolves an archive entry name against destDir and returns the
// resulting filesystem path, rejecting any entry that would let extraction
// write outside destDir (a "Zip Slip" path-traversal attempt via ".."
// components, an absolute path, or a Windows-style backslash path).
//
// Every archive-extraction call site in this repo (OCI artifact pulls,
// toolchain package installs, CI cache restores, and the Playwright SAML
// driver seed) delegates its containment check to this function so the
// guard only needs to be reasoned about and fixed in one place.
//
// The GitHub PR artifact extractor in pkg/toolchain/pr_artifact.go
// intentionally does NOT call this helper: its guards are duplicated inline,
// directly adjacent to each filesystem sink, because CodeQL's go/zipslip
// dataflow analysis only credits a containment check it can see immediately
// next to the sink -- a check performed inside a function in another package
// isn't reliably recognized as a sanitizer. That shape was arrived at
// empirically closing alert #5230 and must not be refactored into a shared
// call.
func SafeJoin(destDir, entryName string) (string, error) {
	defer perf.Track(nil, "filesystem.SafeJoin")()

	if filepath.IsAbs(entryName) || strings.HasPrefix(entryName, "/") || strings.HasPrefix(entryName, "\\") {
		return "", fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrArchiveEntryEscapesDest, entryName)
	}
	if strings.Contains(entryName, "\\") {
		return "", fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrArchiveEntryEscapesDest, entryName)
	}
	for _, part := range strings.Split(filepath.ToSlash(entryName), "/") {
		if part == ".." {
			return "", fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrArchiveEntryEscapesDest, entryName)
		}
	}

	cleanDestDir := filepath.Clean(destDir)
	target := filepath.Join(cleanDestDir, filepath.FromSlash(entryName))

	// Defense in depth: given the guards above (no absolute path, no backslash, no ".."
	// component), Join+Clean of destDir with a purely relative, dot-dot-free entry name
	// can never actually let target escape destDir. This re-verifies that via
	// filepath.Rel rather than a string-prefix comparison, since a naive
	// `strings.HasPrefix(target, cleanDestDir+separator)` check incorrectly rejects a
	// valid target when cleanDestDir is a root path with no room for a trailing
	// separator to appear before the next path segment (destDir "." or "" -> cleanDestDir
	// "." with target "file", or destDir "/" with target "/file").
	rel, err := filepath.Rel(cleanDestDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrArchiveEntryEscapesDest, entryName)
	}

	return target, nil
}
