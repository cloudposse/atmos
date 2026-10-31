package materialize

import (
	"context"
	"fmt"
	"os"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultMaxBytes bounds the total declared size of extracted file content.
const DefaultMaxBytes int64 = 64 << 30

// Option customizes extraction.
type Option func(*options)

type options struct {
	maxBytes int64
}

// WithMaxBytes overrides the limit on total extracted file content.
func WithMaxBytes(limit int64) Option {
	defer perf.Track(nil, "materialize.WithMaxBytes")()

	return func(o *options) { o.maxBytes = limit }
}

func applyOptions(opts []Option) options {
	applied := options{maxBytes: DefaultMaxBytes}
	for _, opt := range opts {
		if opt != nil {
			opt(&applied)
		}
	}
	if applied.maxBytes <= 0 {
		applied.maxBytes = DefaultMaxBytes
	}
	return applied
}

// Extract writes content into the empty directory stage, verifying every member against the
// Record inventory, checking that no symlink resolves outside stage, and applying final modes
// last. The scratch directory is private and holds temporary spool files. Extract does not close
// content.
func Extract(ctx context.Context, content *artifact.Content, stage, scratch string, opts ...Option) error {
	defer perf.Track(nil, "materialize.Extract")()

	applied := applyOptions(opts)
	if content == nil || content.Record == nil {
		return artifact.NewUnsupportedCapability("", "pull", "materialize/graph", "a native OCI graph without a portable Record cannot be materialized as a file tree")
	}
	record := content.Record
	if err := record.Validate(); err != nil {
		return err
	}
	if err := checkTotalSize(record, applied.maxBytes); err != nil {
		return err
	}
	root, err := os.OpenRoot(stage)
	if err != nil {
		return fmt.Errorf("%w: open staging root: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	defer root.Close()

	t := newTree(root, record)
	if err := fill(ctx, t, content, scratch); err != nil {
		return err
	}
	if err := checkLinkContainment(stage, record); err != nil {
		return err
	}
	return t.complete()
}

func checkTotalSize(record *artifact.Record, limit int64) error {
	var total int64
	for index := range record.Inventory {
		total += record.Inventory[index].Size
		if total > limit || total < 0 {
			return fmt.Errorf("%w: content exceeds the %d byte extraction limit", errUtils.ErrArtifactTooLarge, limit)
		}
	}
	return nil
}

// fill dispatches on the packaging mode.
func fill(ctx context.Context, t *tree, content *artifact.Content, scratch string) error {
	switch content.Representation.Packaging {
	case artifact.PackagingNone:
		return fillFiles(ctx, t, content)
	case artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip:
		return fillArchive(ctx, t, content, scratch)
	default:
		return fmt.Errorf("%w: cannot materialize packaging %q", errUtils.ErrArtifactPackagingInvalid, content.Representation.Packaging)
	}
}

// fillFiles writes a per-file payload: file entries come from their path-addressed streams;
// directories and symlinks come from the inventory.
func fillFiles(ctx context.Context, t *tree, content *artifact.Content) error {
	streams := make(map[string]*artifact.ContentStream, len(content.Streams))
	for index := range content.Streams {
		stream := &content.Streams[index]
		if stream.Role != artifact.BlobPayload {
			continue
		}
		if _, duplicate := streams[stream.Path]; duplicate {
			return fmt.Errorf("%w: payload %q appears more than once", errUtils.ErrArtifactIntegrityFailed, stream.Path)
		}
		streams[stream.Path] = stream
	}
	paths := make([]string, 0, len(t.inventory))
	for entryPath := range t.inventory {
		paths = append(paths, entryPath)
	}
	sort.Strings(paths)
	for _, entryPath := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fillInventoryEntry(t, t.inventory[entryPath], streams); err != nil {
			return err
		}
		delete(streams, entryPath)
	}
	for extra := range streams {
		return fmt.Errorf("%w: payload %q is not in the artifact inventory", errUtils.ErrArtifactIntegrityFailed, extra)
	}
	return nil
}

func fillInventoryEntry(t *tree, entry *artifact.Entry, streams map[string]*artifact.ContentStream) error {
	if _, err := t.entryFor(entry.Path); err != nil {
		return err
	}
	switch entry.Type {
	case artifact.EntryDirectory:
		return t.mkdir(entry)
	case artifact.EntrySymlink:
		return t.symlink(entry)
	default:
		stream, ok := streams[entry.Path]
		if !ok || stream.Reader == nil {
			return fmt.Errorf("%w: payload for %q is missing", errUtils.ErrArtifactIntegrityFailed, entry.Path)
		}
		return t.writeFile(entry, stream.Reader)
	}
}
