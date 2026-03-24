package tests

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// mergeIntoCoverDir copies coverage data files produced by a single subprocess
// from src (a per-test GOCOVERDIR) into dst (the shared GOCOVERDIR).
//
// Go coverage-instrumented binaries write two kinds of files to GOCOVERDIR:
//   - covmeta.<hash>       – package-level metadata; identical for every run of
//     the same binary.  We skip the copy if the file already exists in dst
//     because the content is always the same.
//   - covcounters.<hash>.<pid>.<nanos> – per-execution counter data; unique per
//     process (the name includes PID and nanosecond timestamp), so no conflict.
//
// This is safe to call concurrently from multiple goroutines because:
//   - covmeta writes use O_EXCL for an atomic claim-or-skip, eliminating the
//     TOCTOU race of the previous os.Stat+copyFile pattern.
//   - covcounters names are globally unique, so concurrent copies never
//     overwrite each other.
func mergeIntoCoverDir(src, dst string) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// covmeta files are binary-identical for all processes of the same binary.
		// Use O_EXCL to atomically claim the destination file: only the first
		// goroutine wins; all others get EEXIST and skip.  We write directly to the
		// claimed file (without closing first) so the file is never left empty if the
		// copy fails — on error we remove the empty claim file.
		if strings.HasPrefix(entry.Name(), "covmeta.") {
			claimed, excErr := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec
			if excErr != nil {
				continue // EEXIST — another goroutine already wrote this file; skip.
			}
			if copyErr := copyToFile(srcPath, claimed); copyErr != nil {
				claimed.Close()    //nolint:errcheck
				os.Remove(dstPath) //nolint:errcheck // remove empty claim on error
				continue
			}
			if closeErr := claimed.Close(); closeErr != nil {
				os.Remove(dstPath) //nolint:errcheck
			}
			continue
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			continue
		}
	}
}

// copyFile copies a single file using streaming I/O to avoid loading the
// entire file into memory (coverage counter files can be several MB each).
// src and dst are always paths within t.TempDir() or the shared coverDir, never
// user-supplied input, so the G304/G306 gosec warnings are safe to suppress here.
func copyFile(src, dst string) (retErr error) {
	in, err := os.Open(src) //nolint:gosec // src is always a controlled temp path
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // dst is always a controlled temp path
	if err != nil {
		return err
	}
	// Explicitly close the write side so OS flush errors (e.g. disk full) are
	// captured and returned to the caller rather than silently swallowed by a
	// deferred close.
	defer func() {
		if closeErr := out.Close(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	_, err = io.Copy(out, in)
	return err
}

// copyToFile copies src into an already-open (and writable) dst file.
// Used by mergeIntoCoverDir to write into a file that was claimed with O_EXCL,
// avoiding the need to close-and-reopen between the claim and the copy.
func copyToFile(src string, dst *os.File) error {
	in, err := os.Open(src) //nolint:gosec // src is always a controlled temp path
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck
	_, err = io.Copy(dst, in)
	return err
}
