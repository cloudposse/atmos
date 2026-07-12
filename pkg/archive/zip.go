package archive

import (
	"archive/zip"
	"io"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// writeZip creates destination fresh, writing every entry, via atomicRewrite.
func writeZip(destination string, entries []packEntry, repro *reproducibleTimestamps) error {
	defer perf.Track(nil, "archive.writeZip")()

	return atomicRewrite(destination, ".archive-write-*.zip", func(tmp *os.File) error {
		return writeZipEntries(tmp, destination, entries, repro)
	})
}

func writeZipEntries(tmp *os.File, destination string, entries []packEntry, repro *reproducibleTimestamps) error {
	zw := zip.NewWriter(tmp)
	for _, e := range entries {
		if err := addZipEntry(zw, e, repro); err != nil {
			zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return writeFailedError(destination, err)
	}
	return nil
}

// updateZip adds/refreshes entries in destination, leaving untouched entries
// as-is. If destination does not exist yet, it behaves like writeZip. Both
// the copy-forward of unchanged entries and the new/changed entries are
// written through the same zip.Writer, since a zip.Writer tracks byte
// offsets from the start of the underlying stream — two independent writers
// over the same file would corrupt those offsets.
func updateZip(destination string, entries []packEntry, repro *reproducibleTimestamps) error {
	defer perf.Track(nil, "archive.updateZip")()

	changed := make(map[string]bool, len(entries))
	for _, e := range entries {
		changed[e.archivePath] = true
	}

	return atomicRewrite(destination, ".archive-update-*.zip", func(tmp *os.File) error {
		return writeZipUpdate(tmp, destination, changed, entries, repro)
	})
}

func writeZipUpdate(dst *os.File, destination string, changed map[string]bool, entries []packEntry, repro *reproducibleTimestamps) error {
	zw := zip.NewWriter(dst)

	if err := copyUnchangedZipEntries(zw, destination, changed); err != nil {
		zw.Close()
		return err
	}
	for _, e := range entries {
		if err := addZipEntry(zw, e, repro); err != nil {
			zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return writeFailedError(destination, err)
	}
	return nil
}

// copyUnchangedZipEntries copies every entry from the existing destination
// archive that isn't about to be replaced, using zip.Writer.Copy's raw copy
// (no decompress/recompress). A missing destination is not an error — the
// update behaves like a fresh write.
//
// These entries are not touched by reproducible normalization: they carry
// whatever mtime/mode a prior write already gave them. Making an update
// fully reproducible on top of a non-reproducible archive would require
// re-encoding every existing entry instead of raw-copying it, which is a
// materially different (and much more expensive) operation than update's
// incremental contract promises. Only action: replace is the reproducible path.
func copyUnchangedZipEntries(zw *zip.Writer, destination string, changed map[string]bool) error {
	existing, err := zip.OpenReader(destination)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errUtils.Build(errUtils.ErrArchiveWriteFailed).
			WithCause(err).
			WithExplanationf("failed to open existing archive %q for update", destination).
			WithContext("destination", destination).
			Err()
	}
	defer existing.Close()

	for _, f := range existing.File {
		if changed[f.Name] {
			continue
		}
		if err := zw.Copy(f); err != nil {
			return writeFailedError(destination, err)
		}
	}
	return nil
}

func addZipEntry(zw *zip.Writer, e packEntry, repro *reproducibleTimestamps) error {
	src, err := os.Open(e.fsPath)
	if err != nil {
		return writeFailedError(e.fsPath, err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return writeFailedError(e.fsPath, err)
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return writeFailedError(e.fsPath, err)
	}
	header.Name = e.archivePath
	header.Method = zip.Deflate
	if modTime := repro.modTimeFor(e.fsPath); !modTime.IsZero() {
		header.Modified = modTime
		header.SetMode(normalizeMode(info.Mode()))
	}

	w, err := zw.CreateHeader(header)
	if err != nil {
		return writeFailedError(e.archivePath, err)
	}
	if _, err := io.Copy(w, src); err != nil {
		return writeFailedError(e.archivePath, err)
	}
	return nil
}

func writeFailedError(path string, cause error) error {
	return errUtils.Build(errUtils.ErrArchiveWriteFailed).
		WithCause(cause).
		WithContext("path", path).
		Err()
}
