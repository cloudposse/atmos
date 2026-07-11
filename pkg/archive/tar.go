package archive

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
)

// writeTar creates destination fresh, writing every entry. When gz is true,
// the tar stream is wrapped in gzip compression (format: tgz); otherwise it
// writes a plain tar.
func writeTar(destination string, entries []packEntry, gz bool) error {
	defer perf.Track(nil, "archive.writeTar")()

	f, err := os.Create(destination)
	if err != nil {
		return writeFailedError(destination, err)
	}
	defer f.Close()

	var w io.Writer = f
	var gzw *gzip.Writer
	if gz {
		gzw = gzip.NewWriter(f)
		w = gzw
	}

	tw := tar.NewWriter(w)
	for _, e := range entries {
		if err := addTarEntry(tw, e); err != nil {
			tw.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return writeFailedError(destination, err)
	}
	if gzw != nil {
		if err := gzw.Close(); err != nil {
			return writeFailedError(destination, err)
		}
	}
	return nil
}

// updateTar adds/refreshes entries in an uncompressed tar, leaving untouched
// entries as-is. If destination does not exist yet, it behaves like
// writeTar. Both the copy-forward of unchanged entries and the new/changed
// entries are written through the same tar.Writer, for the same reason as
// updateZip: a second writer over the same file would restart its internal
// state and corrupt the archive.
func updateTar(destination string, entries []packEntry) error {
	defer perf.Track(nil, "archive.updateTar")()

	changed := make(map[string]bool, len(entries))
	for _, e := range entries {
		changed[e.archivePath] = true
	}

	tmp, err := os.CreateTemp(filepath.Dir(destination), ".archive-update-*.tar")
	if err != nil {
		return writeFailedError(destination, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := writeTarUpdate(tmp, destination, changed, entries); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return writeFailedError(destination, err)
	}
	// destination is operator-provided step/hook configuration (the archive
	// step's own `destination:` field), not externally tainted input.
	if err := os.Rename(tmpPath, destination); err != nil { //nolint:gosec // G703: destination is trusted step config, not tainted input.
		return writeFailedError(destination, err)
	}
	return nil
}

func writeTarUpdate(dst *os.File, destination string, changed map[string]bool, entries []packEntry) error {
	tw := tar.NewWriter(dst)

	if err := copyUnchangedTarEntries(tw, destination, changed); err != nil {
		tw.Close()
		return err
	}
	for _, e := range entries {
		if err := addTarEntry(tw, e); err != nil {
			tw.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return writeFailedError(destination, err)
	}
	return nil
}

// copyUnchangedTarEntries re-encodes every entry from the existing
// destination archive that isn't about to be replaced. Unlike zip, tar has
// no raw-copy primitive in the standard library, but since this path only
// runs for uncompressed tar (see updatable), re-encoding costs no
// decompression/recompression. A missing destination is not an error — the
// update behaves like a fresh write.
func copyUnchangedTarEntries(tw *tar.Writer, destination string, changed map[string]bool) error {
	existing, err := os.Open(destination)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return writeFailedError(destination, err)
	}
	defer existing.Close()

	tr := tar.NewReader(existing)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return writeFailedError(destination, err)
		}
		if changed[hdr.Name] {
			continue
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return writeFailedError(destination, err)
		}
		// existing is a local archive the operator already owns (about to be
		// replaced by this very update), not untrusted external input.
		if _, err := io.Copy(tw, tr); err != nil { //nolint:gosec // G110: re-encoding the operator's own local archive, not a decompression bomb vector.
			return writeFailedError(destination, err)
		}
	}
}

func addTarEntry(tw *tar.Writer, e packEntry) error {
	src, err := os.Open(e.fsPath)
	if err != nil {
		return writeFailedError(e.fsPath, err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return writeFailedError(e.fsPath, err)
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return writeFailedError(e.fsPath, err)
	}
	header.Name = e.archivePath

	if err := tw.WriteHeader(header); err != nil {
		return writeFailedError(e.archivePath, err)
	}
	if _, err := io.Copy(tw, src); err != nil {
		return writeFailedError(e.archivePath, err)
	}
	return nil
}
