package snapshot

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/text/unicode/norm"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Default capture limits. Both are overridable with [WithLimits].
const (
	// DefaultMaxBytes bounds the total staged file bytes.
	DefaultMaxBytes int64 = 16 << 30

	// DefaultMaxEntries bounds the number of inventory entries.
	DefaultMaxEntries = 100_000
)

const (
	stagingPattern = "atmos-artifact-*"
	dataDirName    = "data"
	stagingDirPerm = 0o700
	stagedFilePerm = 0o600
)

// Limits bound a capture. Zero values select the defaults.
type Limits struct {
	// MaxBytes is the maximum total size of staged file content.
	MaxBytes int64

	// MaxEntries is the maximum number of inventory entries.
	MaxEntries int
}

// Option customizes [Capture].
type Option func(*options)

type options struct {
	stagingRoot string
	limits      Limits
	onCaptured  func(path string)
	excluder    Excluder
}

// Excluder decides whether a source entry is left out of a capture. It receives the slash-separated
// path relative to the source root and whether the entry is a directory, and returns the visible
// rule that excludes it. An excluded directory is skipped with everything below it, and the exclusion
// is reported on the [Snapshot] so that dry-run output can show what was left out and why.
type Excluder func(entryPath string, isDir bool) (rule string, excluded bool)

// WithExcluder leaves entries out of the capture. The generic files producer never sets one.
func WithExcluder(excluder Excluder) Option {
	defer perf.Track(nil, "snapshot.WithExcluder")()

	return func(o *options) { o.excluder = excluder }
}

// WithStagingRoot sets the directory that holds the private staging directory. The default is the
// operating system's temporary directory.
func WithStagingRoot(dir string) Option {
	defer perf.Track(nil, "snapshot.WithStagingRoot")()

	return func(o *options) { o.stagingRoot = dir }
}

// WithLimits overrides the capture limits.
func WithLimits(limits Limits) Option {
	defer perf.Track(nil, "snapshot.WithLimits")()

	return func(o *options) { o.limits = limits }
}

// WithCaptureHook registers a callback invoked after each regular file has been copied and before
// the final recheck. Tests use it to mutate a source file mid-capture.
func WithCaptureHook(hook func(path string)) Option {
	defer perf.Track(nil, "snapshot.WithCaptureHook")()

	return func(o *options) { o.onCaptured = hook }
}

// Snapshot is a stable, staged copy of a source plus its sorted inventory. It owns a private
// staging directory; the caller must Close it.
type Snapshot struct {
	// Root is the private staging directory.
	Root string

	// Entries is the inventory, sorted by path.
	Entries []artifact.Entry

	// TotalBytes is the total size of staged file content.
	TotalBytes int64

	// SourceIsFile reports that the source was a single file rather than a directory.
	SourceIsFile bool

	// Exclusions lists the entries an [Excluder] left out, sorted by path. Excluded directories are
	// listed once, not per descendant.
	Exclusions []artifact.Exclusion

	staged map[string]string
}

// Close removes the staging directory. It is idempotent.
func (s *Snapshot) Close() error {
	defer perf.Track(nil, "snapshot.Snapshot.Close")()

	if s == nil || s.Root == "" {
		return nil
	}
	err := os.RemoveAll(s.Root)
	s.Root = ""
	s.staged = nil
	return err
}

// StagedPath returns the staged copy of the regular file at the inventory path.
func (s *Snapshot) StagedPath(entryPath string) (string, bool) {
	defer perf.Track(nil, "snapshot.Snapshot.StagedPath")()

	p, ok := s.staged[entryPath]
	return p, ok
}

// Open opens the staged copy of the regular file at the inventory path.
func (s *Snapshot) Open(entryPath string) (io.ReadCloser, error) {
	defer perf.Track(nil, "snapshot.Snapshot.Open")()

	p, ok := s.staged[entryPath]
	if !ok {
		return nil, fmt.Errorf("%w: %q is not a staged file", errUtils.ErrArtifactNotFound, entryPath)
	}
	return os.Open(p)
}

// Lookup returns the inventory entry at path.
func (s *Snapshot) Lookup(entryPath string) (artifact.Entry, bool) {
	defer perf.Track(nil, "snapshot.Snapshot.Lookup")()

	index := sort.Search(len(s.Entries), func(i int) bool { return s.Entries[i].Path >= entryPath })
	if index < len(s.Entries) && s.Entries[index].Path == entryPath {
		return s.Entries[index], true
	}
	return artifact.Entry{}, false
}

// Capture snapshots source into bounded disk staging. It fails, and removes all staged data, when
// the source contains unsafe entries, exceeds the limits, changes during capture, or ctx is
// canceled.
func Capture(ctx context.Context, source string, opts ...Option) (*Snapshot, error) {
	defer perf.Track(nil, "snapshot.Capture")()

	var applied options
	for _, opt := range opts {
		if opt != nil {
			opt(&applied)
		}
	}
	if applied.limits.MaxBytes <= 0 {
		applied.limits.MaxBytes = DefaultMaxBytes
	}
	if applied.limits.MaxEntries <= 0 {
		applied.limits.MaxEntries = DefaultMaxEntries
	}
	root, err := os.MkdirTemp(applied.stagingRoot, stagingPattern)
	if err != nil {
		return nil, fmt.Errorf("%w: create staging directory: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	snap, err := captureInto(ctx, source, root, &applied)
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, err
	}
	return snap, nil
}

func captureInto(ctx context.Context, source, root string, opts *options) (*Snapshot, error) {
	dataDir := filepath.Join(root, dataDirName)
	if err := os.Mkdir(dataDir, stagingDirPerm); err != nil {
		return nil, fmt.Errorf("%w: create staging data directory: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	resolved, info, err := resolveSource(source)
	if err != nil {
		return nil, err
	}
	c := &capturer{ctx: ctx, opts: opts, root: resolved, dataDir: dataDir, staged: make(map[string]string), dirs: make(map[string][]string)}
	if info.IsDir() {
		err = c.walkDir(resolved, "")
	} else {
		err = c.captureFile(resolved, filepath.Base(resolved), info)
	}
	if err != nil {
		return nil, err
	}
	if err := c.recheck(); err != nil {
		return nil, err
	}
	if info.IsDir() {
		if err := c.checkLinkContainment(); err != nil {
			return nil, err
		}
	}
	if err := c.finish(); err != nil {
		return nil, err
	}
	sort.Slice(c.entries, func(i, j int) bool { return c.entries[i].Path < c.entries[j].Path })
	sort.Slice(c.exclusions, func(i, j int) bool { return c.exclusions[i].Path < c.exclusions[j].Path })
	return &Snapshot{Root: root, Entries: c.entries, TotalBytes: c.total, SourceIsFile: !info.IsDir(), Exclusions: c.exclusions, staged: c.staged}, nil
}

// resolveSource resolves a user-chosen top-level source (which may itself be a symlink) and checks
// it is a directory or regular file. Links inside the source are never followed.
func resolveSource(source string) (string, os.FileInfo, error) {
	if source == "" {
		return "", nil, fmt.Errorf("%w: source is empty", errUtils.ErrArtifactPackagingInvalid)
	}
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", nil, fmt.Errorf("%w: source %q: %w", errUtils.ErrArtifactPackagingInvalid, source, err)
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("%w: source %q: %w", errUtils.ErrArtifactPackagingInvalid, source, err)
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("%w: source %q must be a regular file or directory", errUtils.ErrArtifactPackagingInvalid, source)
	}
	return resolved, info, nil
}

// finish validates every entry and rejects paths that collide after Unicode normalization.
func (c *capturer) finish() error {
	seen := make(map[string]string, len(c.entries))
	for index := range c.entries {
		entry := &c.entries[index]
		if err := entry.Validate(); err != nil {
			return err
		}
		normalized := norm.NFC.String(entry.Path)
		if previous, duplicate := seen[normalized]; duplicate {
			return fmt.Errorf("%w: paths %q and %q are duplicates after Unicode normalization", errUtils.ErrArtifactPackagingInvalid, previous, entry.Path)
		}
		seen[normalized] = entry.Path
	}
	return nil
}
