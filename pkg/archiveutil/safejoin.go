// Package archiveutil provides shared safety helpers for extracting archive
// entries (zip, tar) to the filesystem. It is intentionally a leaf package
// (depending only on errors/perf) so every archive-extracting package in the
// module -- including pkg/oci, which pkg/archive transitively depends on via
// pkg/downloader -- can import it without an import cycle.
package archiveutil

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
	defer perf.Track(nil, "archiveutil.SafeJoin")()

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
	// can never actually escape destDir, so this branch is unreachable in practice. It's
	// kept anyway as a second, independent check in case a future edit weakens or
	// reorders the guards above.
	if target != cleanDestDir && !strings.HasPrefix(target, cleanDestDir+string(os.PathSeparator)) {
		return "", fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrArchiveEntryEscapesDest, entryName)
	}

	return target, nil
}
