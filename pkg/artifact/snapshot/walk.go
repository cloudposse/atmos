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
	entries []artifact.Entry
	// exclusions are the entries the excluder left out.
	exclusions []artifact.Exclusion
	staged     map[string]string
	dirs       map[string][]string
	sources    []sourceRecord
	total      int64
	counter    int
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
	mode := info.Mode()
	if c.opts.excluder != nil {
		if rule, excluded := c.opts.excluder(rel, mode.IsDir()); excluded {
			c.exclusions = append(c.exclusions, artifact.Exclusion{Path: rel, Rule: rule})
			return nil
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

// checkLinkContainment resolves every captured symlink physically and rejects any that leaves the
// source. A lexical target check is not enough, because a chain of links can clean to a contained
// path yet resolve outside it. Dangling links stay allowed.
func (c *capturer) checkLinkContainment() error {
	realRoot, err := filepath.EvalSymlinks(c.root)
	if err != nil {
		return fmt.Errorf("%w: resolve source: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	for index := range c.sources {
		record := &c.sources[index]
		if record.target == "" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(record.abs)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(realRoot, resolved)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: symlink %q resolves outside the source", errUtils.ErrArtifactUnsafePath, record.rel)
		}
	}
	return nil
}

// changedError wraps a failure observed while a source entry was being captured.
func changedError(rel string, cause error) error {
	return fmt.Errorf("%w: %q: %w", errUtils.ErrArtifactSourceChanged, rel, cause)
}
