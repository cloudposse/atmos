package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/linkpath"
)

// sourceRecord remembers one captured source entry for the final recheck.
type sourceRecord struct {
	abs    string
	rel    string
	info   os.FileInfo
	digest digest.Digest
	target string
}

// capturer holds the state of one capture.
type capturer struct {
	ctx     context.Context
	opts    *options
	root    string
	dataDir string

	// rootInfo identifies the source root as it was resolved, so a replacement is detected.
	rootInfo os.FileInfo
	entries  []artifact.Entry
	// exclusions are the entries the excluder left out.
	exclusions      []artifact.Exclusion
	excludedSources []sourceRecord
	staged          map[string]string
	dirs            map[string][]string
	sources         []sourceRecord
	total           int64
	counter         int

	// staging identifies the private staging directory, which is skipped when it lies inside the source.
	staging os.FileInfo
}

// walkDir captures the directory at abs, whose slash-separated inventory path is rel ("" for the root).
func (c *capturer) walkDir(abs, rel string) error {
	names, err := readDirNames(abs)
	if err != nil {
		return err
	}
	c.dirs[abs] = names
	for _, name := range names {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		childAbs := filepath.Join(abs, name)
		childRel := name
		if rel != "" {
			childRel = rel + "/" + name
		}
		if err := c.captureChild(childAbs, childRel); err != nil {
			return err
		}
	}
	return nil
}

// captureChild classifies one directory child with lstat and captures it.
func (c *capturer) captureChild(abs, rel string) error {
	info, err := os.Lstat(abs)
	if err != nil {
		return changedError(rel, err)
	}
	if c.staging != nil && os.SameFile(c.staging, info) {
		// The private staging directory changes while files are staged, so it is never source content.
		return nil
	}
	mode := info.Mode()
	if c.opts.excluder != nil {
		if rule, excluded := c.opts.excluder(rel, mode.IsDir()); excluded {
			return c.exclude(abs, rel, rule, info)
		}
	}
	switch {
	case mode.IsDir():
		if err := c.addEntry(&artifact.Entry{Path: rel, Type: artifact.EntryDirectory, Mode: permBits(info)}); err != nil {
			return err
		}
		c.sources = append(c.sources, sourceRecord{abs: abs, rel: rel, info: info})
		return c.walkDir(abs, rel)
	case mode.IsRegular():
		return c.captureFile(abs, rel, info)
	case mode&os.ModeSymlink != 0:
		return c.captureSymlink(abs, rel, info)
	default:
		return fmt.Errorf("%w: %q is a device, socket, pipe, or other special file (%s)", errUtils.ErrArtifactPackagingInvalid, rel, mode.Type())
	}
}

// exclude records a bounded exclusion and its source type for the final recheck.
func (c *capturer) exclude(abs, rel, rule string, info os.FileInfo) error {
	if len(c.exclusions) >= c.opts.limits.MaxEntries {
		return fmt.Errorf("%w: more than %d exclusions", errUtils.ErrArtifactTooLarge, c.opts.limits.MaxEntries)
	}
	c.exclusions = append(c.exclusions, artifact.Exclusion{Path: rel, Rule: rule})
	c.excludedSources = append(c.excludedSources, sourceRecord{abs: abs, rel: rel, info: info})
	return nil
}

// captureSymlink records a relative symlink without following it. Absolute or escaping targets
// are rejected when the entry is validated.
func (c *capturer) captureSymlink(abs, rel string, info os.FileInfo) error {
	target, err := os.Readlink(abs)
	if err != nil {
		return changedError(rel, err)
	}
	if filepath.IsAbs(target) || filepath.VolumeName(target) != "" || strings.HasPrefix(target, "/") {
		return fmt.Errorf("%w: symlink %q has absolute target %q", errUtils.ErrArtifactUnsafePath, rel, target)
	}
	target = filepath.ToSlash(target)
	entry := artifact.Entry{Path: rel, Type: artifact.EntrySymlink, Target: target}
	if err := entry.Validate(); err != nil {
		return err
	}
	if err := c.addEntry(&entry); err != nil {
		return err
	}
	c.sources = append(c.sources, sourceRecord{abs: abs, rel: rel, info: info, target: target})
	return nil
}

// addEntry appends an inventory entry, enforcing the entry limit.
func (c *capturer) addEntry(entry *artifact.Entry) error {
	if len(c.entries) >= c.opts.limits.MaxEntries {
		return fmt.Errorf("%w: more than %d entries", errUtils.ErrArtifactTooLarge, c.opts.limits.MaxEntries)
	}
	c.entries = append(c.entries, *entry)
	return nil
}

// permBits returns the ordinary permission bits of info with privileged bits cleared.
func permBits(info os.FileInfo) uint32 {
	return uint32(info.Mode().Perm())
}

// readDirNames lists a directory's child names in bytewise order.
func readDirNames(dir string) ([]string, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: read directory %q: %w", errUtils.ErrArtifactSourceChanged, dir, err)
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name())
	}
	slices.Sort(names)
	return names, nil
}

// checkLinkContainment resolves every captured symlink through the captured links and rejects any
// that leaves the source. A lexical check of each target is not enough, because a chain of links
// can clean to a contained path yet resolve outside it. A dangling link is resolved through its
// existing part plus the missing remainder, so it cannot escape once the missing path is created.
// The check reads the links as captured and never the source, which may have changed since.
// A contained link is then checked against the excluder, so that it cannot name excluded content.
func (c *capturer) checkLinkContainment() error {
	links := make(map[string]string)
	for index := range c.sources {
		if record := &c.sources[index]; record.target != "" {
			links[record.rel] = record.target
		}
	}
	var captured map[string]bool
	if c.opts != nil && c.opts.excluder != nil {
		captured = c.capturedTypes()
	}
	for index := range c.sources {
		record := &c.sources[index]
		if record.target == "" {
			continue
		}
		if linkpath.Escapes(links, record.rel) {
			return fmt.Errorf("%w: symlink %q resolves outside the source", errUtils.ErrArtifactUnsafePath, record.rel)
		}
		if captured == nil {
			continue
		}
		if err := c.checkExcludedTarget(links, captured, record.rel); err != nil {
			return err
		}
	}
	return nil
}

// capturedTypes maps the path of every entry the walk classified, captured or excluded, to whether
// it was a directory. It is the walk's own record of the source, so the excluder can be asked about
// a link target with the same arguments the walk used, without inspecting the source again.
func (c *capturer) capturedTypes() map[string]bool {
	types := make(map[string]bool, len(c.sources)+len(c.excludedSources))
	for index := range c.sources {
		types[c.sources[index].rel] = c.sources[index].info.IsDir()
	}
	for index := range c.excludedSources {
		types[c.excludedSources[index].rel] = c.excludedSources[index].info.IsDir()
	}
	return types
}

// checkExcludedTarget rejects a symlink whose target is an excluded entry, lies below one, or passes
// through one on the way. The filesystem looks up every directory before it processes a ".." that
// follows it, so a link such as "cache/p/../../keep.txt" breaks when "cache" is left out even
// though it reduces to "keep.txt". The target is therefore followed through the captured links the
// way the filesystem would, and each path it traverses is tested as the walk classified it, so the
// answer cannot change when the source changes after capture. A path the walk never classified is
// either missing (a dangling link) or below an excluded directory, which is traversed, and tested,
// first.
func (c *capturer) checkExcludedTarget(links map[string]string, captured map[string]bool, link string) error {
	traversed, inside := linkpath.TraversedRecorded(links, link)
	if !inside {
		return nil
	}
	for _, candidate := range traversed {
		isDir, classified := captured[candidate]
		if !classified {
			continue
		}
		if _, excluded := c.opts.excluder(candidate, isDir); excluded {
			return fmt.Errorf("%w: symlink %q targets excluded path %q", errUtils.ErrArtifactUnsafePath, link, candidate)
		}
	}
	return nil
}

// changedError wraps a failure observed while a source entry was being captured.
func changedError(rel string, cause error) error {
	return fmt.Errorf("%w: %q: %w", errUtils.ErrArtifactSourceChanged, rel, cause)
}
