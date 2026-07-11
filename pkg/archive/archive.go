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
	if strings.TrimSpace(opts.Source) == "" {
		return errUtils.Build(errUtils.ErrArchiveSourceRequired).Err()
	}
	if strings.TrimSpace(opts.Destination) == "" {
		return errUtils.Build(errUtils.ErrArchiveDestinationRequired).Err()
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
