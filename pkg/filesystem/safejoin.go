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

	// Reject absolute-looking and backslash-containing entry names outright rather
	// than relying on filepath.Join/Rel to neutralize them: Join treats an embedded
	// leading "/" as just another path segment (so "/etc/passwd" harmlessly becomes
	// destDir/etc/passwd, not an escape), but both shapes are never legitimate zip/tar
	// entry names -- the format always uses forward slashes -- so treat either as a
	// suspicious archive outright.
	if filepath.IsAbs(entryName) || strings.HasPrefix(entryName, "/") || strings.HasPrefix(entryName, "\\") {
		return "", fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrArchiveEntryEscapesDest, entryName)
	}
	if strings.Contains(entryName, "\\") {
		return "", fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrArchiveEntryEscapesDest, entryName)
	}

	cleanDestDir := filepath.Clean(destDir)
	target := filepath.Join(cleanDestDir, filepath.FromSlash(entryName))

	// filepath.Rel is the actual containment check: it reflects the true
	// relationship between target and cleanDestDir after Join has resolved any ".."
	// components, so it correctly accepts a harmless internal ".." that never leaves
	// destDir (e.g. "a/../b") and correctly rejects one that does, including the
	// sibling-directory-name case ("../destEVIL/file" is not a "destEVIL" component
	// match but does resolve outside cleanDestDir). A naive
	// `strings.HasPrefix(target, cleanDestDir+separator)` check would also incorrectly
	// reject a valid target when cleanDestDir is a root path with no room for a
	// trailing separator before the next path segment (destDir "." or "" -> cleanDestDir
	// "." with target "file", or destDir "/" with target "/file").
	rel, err := filepath.Rel(cleanDestDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrArchiveEntryEscapesDest, entryName)
	}

	return target, nil
}
