package archive

import (
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Action names the archive verb. Only ActionReplace and ActionUpdate are
// implemented; ActionCreate and ActionExtract are reserved in the schema for
// a future phase so it never needs a breaking change.
type Action string

// Supported archive actions.
const (
	ActionCreate  Action = "create"
	ActionExtract Action = "extract"
	ActionUpdate  Action = "update"
	ActionReplace Action = "replace"
)

// PackOptions configures a pack (replace/update) operation.
type PackOptions struct {
	// Source is the directory or file to archive.
	Source string
	// Destination is the archive file to write.
	Destination string
	// Format is the archive format; empty infers it from Destination's extension.
	Format string
	// Subpath nests Source's content under this path inside the archive.
	Subpath string
	// Include, if non-empty, keeps only files matching at least one glob.
	Include []string
	// Exclude drops files matching any glob, evaluated before Include.
	Exclude []string
}

// defaultDirPerm is the mode used when creating a destination's parent
// directory tree.
const defaultDirPerm = 0o755

// defaultArchivePerm is the file mode applied to a freshly written archive
// when its destination doesn't already exist.
const defaultArchivePerm = 0o644

// Run executes action against opts.
func Run(action Action, opts *PackOptions) error {
	defer perf.Track(nil, "archive.Run")()

	switch action {
	case ActionReplace, "":
		return replace(opts)
	case ActionUpdate:
		return update(opts)
	case ActionCreate, ActionExtract:
		return errUtils.Build(errUtils.ErrArchiveActionNotImplemented).
			WithExplanationf("action %q is reserved but not implemented in this version", action).
			WithHint("Use action: replace to always rebuild the archive fresh").
			WithContext("action", string(action)).
			Err()
	default:
		return errUtils.Build(errUtils.ErrArchiveActionNotImplemented).
			WithExplanationf("unknown archive action %q", action).
			WithHint("Use one of: create, extract, update, replace").
			WithContext("action", string(action)).
			Err()
	}
}

// replace always rebuilds Destination fresh from Source, overwriting any
// prior contents. Works on every supported format.
func replace(opts *PackOptions) error {
	defer perf.Track(nil, "archive.replace")()

	if err := validatePackOptions(opts); err != nil {
		return err
	}

	format, err := DetectFormat(opts.Format, opts.Destination)
	if err != nil {
		return err
	}
	writer, err := formatWriter(format)
	if err != nil {
		return err
	}

	entries, err := collectEntries(opts.Source, opts.Subpath, opts.Include, opts.Exclude)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(opts.Destination), defaultDirPerm); err != nil {
		return writeFailedError(opts.Destination, err)
	}
	return writer(opts.Destination, entries)
}

// update adds new / refreshes changed entries from Source into the existing
// Destination archive; entries not touched by Source are left as-is.
// Rejected outright for whole-stream-compressed formats.
func update(opts *PackOptions) error {
	defer perf.Track(nil, "archive.update")()

	if err := validatePackOptions(opts); err != nil {
		return err
	}

	format, err := DetectFormat(opts.Format, opts.Destination)
	if err != nil {
		return err
	}
	if !updatable(format) {
		return errUtils.Build(errUtils.ErrArchiveUpdateUnsupportedFormat).
			WithExplanationf("update is not supported for format %q; only zip and uncompressed tar support incremental updates", format).
			WithHint("Use action: replace instead").
			WithContext("format", format).
			Err()
	}

	entries, err := collectEntries(opts.Source, opts.Subpath, opts.Include, opts.Exclude)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(opts.Destination), defaultDirPerm); err != nil {
		return writeFailedError(opts.Destination, err)
	}

	switch format {
	case FormatZip:
		return updateZip(opts.Destination, entries)
	case FormatTar:
		return updateTar(opts.Destination, entries)
	default:
		// Unreachable: updatable() only returns true for FormatZip/FormatTar.
		return errUtils.Build(errUtils.ErrArchiveUpdateUnsupportedFormat).
			WithContext("format", format).
			Err()
	}
}

func validatePackOptions(opts *PackOptions) error {
	if opts == nil {
		return errUtils.Build(errUtils.ErrArchiveOptionsRequired).Err()
	}
	if strings.TrimSpace(opts.Source) == "" {
		return errUtils.Build(errUtils.ErrArchiveSourceRequired).Err()
	}
	if strings.TrimSpace(opts.Destination) == "" {
		return errUtils.Build(errUtils.ErrArchiveDestinationRequired).Err()
	}
	return nil
}

// destinationMode returns the file mode to apply to a rebuilt/updated
// destination archive: the existing file's mode when destination already
// exists (so the temp-file+rename pattern used by replace/update doesn't
// silently downgrade permissions to the temp file's default 0600), or
// defaultArchivePerm when destination doesn't exist yet.
func destinationMode(destination string) (os.FileMode, error) {
	info, err := os.Stat(destination)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultArchivePerm, nil
		}
		return 0, writeFailedError(destination, err)
	}
	return info.Mode().Perm(), nil
}

// atomicRewrite writes a fresh archive stream through write into a temp file
// in destination's directory, then renames it into place only after a
// successful close. A failure partway through write (a source file
// vanishing mid-run, a read/close error) therefore never leaves destination
// truncated or corrupt — the previous valid archive, if any, is untouched.
// Destination's existing permissions are preserved across the rewrite (or
// defaultArchivePerm applied to a brand-new file) rather than downgraded to
// the temp file's default mode. Shared by writeZip/updateZip and
// writeTar/updateTar, which otherwise differ only in their archive format.
func atomicRewrite(destination, tmpPattern string, write func(tmp *os.File) error) error {
	mode, err := destinationMode(destination)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(destination), tmpPattern)
	if err != nil {
		return writeFailedError(destination, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := write(tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return writeFailedError(destination, err)
	}
	// tmpPath is created just above via os.CreateTemp in destination's own
	// directory, not externally tainted input.
	if err := os.Chmod(tmpPath, mode); err != nil { //nolint:gosec // G703: tmpPath is created above via os.CreateTemp, not tainted input.
		return writeFailedError(destination, err)
	}
	// destination is operator-provided step/hook configuration (the archive
	// step's own `destination:` field), not externally tainted input.
	if err := os.Rename(tmpPath, destination); err != nil { //nolint:gosec // G703: destination is trusted step config, not tainted input.
		return writeFailedError(destination, err)
	}
	return nil
}

// formatWriter returns the fresh-write function for format, or a typed
// "not implemented" error for formats recognized but not yet supported for
// writing (tar.bz2, tar.xz — see docs/prd/archive-step.md Open Questions).
func formatWriter(format string) (func(destination string, entries []packEntry) error, error) {
	switch format {
	case FormatZip:
		return writeZip, nil
	case FormatTar:
		return func(destination string, entries []packEntry) error {
			return writeTar(destination, entries, false)
		}, nil
	case FormatTGZ:
		return func(destination string, entries []packEntry) error {
			return writeTar(destination, entries, true)
		}, nil
	default:
		return nil, errUtils.Build(errUtils.ErrArchiveFormatNotImplemented).
			WithExplanationf("writing format %q is not implemented in this version", format).
			WithHint("Use zip, tar, or tgz").
			WithContext("format", format).
			Err()
	}
}
