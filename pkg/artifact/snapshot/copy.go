package snapshot

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// captureFile copies one regular file into staging while hashing it, detecting a file that was
// swapped, grew, shrank, or was modified while it was read.
func (c *capturer) captureFile(abs, rel string, before os.FileInfo) error {
	if c.total+before.Size() > c.opts.limits.MaxBytes {
		return fmt.Errorf("%w: staged content exceeds %d bytes", errUtils.ErrArtifactTooLarge, c.opts.limits.MaxBytes)
	}
	staged, sum, err := c.stageFile(abs, rel, before)
	if err != nil {
		return err
	}
	if hook := c.opts.onCaptured; hook != nil {
		hook(rel)
	}
	entry := artifact.Entry{Path: rel, Type: artifact.EntryFile, Size: before.Size(), Mode: permBits(before), Digest: sum}
	if err := c.addEntry(&entry); err != nil {
		return err
	}
	c.total += before.Size()
	c.staged[rel] = staged
	c.sources = append(c.sources, sourceRecord{abs: abs, rel: rel, info: before, digest: sum})
	return nil
}

// stageFile copies abs into the staging data directory and returns the staged path and digest.
func (c *capturer) stageFile(abs, rel string, before os.FileInfo) (string, digest.Digest, error) {
	src, err := openSame(abs, rel, before)
	if err != nil {
		return "", "", err
	}
	defer src.Close()

	c.counter++
	stagedPath := filepath.Join(c.dataDir, strconv.Itoa(c.counter))
	dst, err := os.OpenFile(stagedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, stagedFilePerm)
	if err != nil {
		return "", "", fmt.Errorf("%w: stage %q: %w", errUtils.ErrArtifactPackagingInvalid, rel, err)
	}
	hasher := sha256.New()
	copied, copyErr := copyContext(c.ctx, io.MultiWriter(dst, hasher), io.LimitReader(src, before.Size()+1))
	if closeErr := dst.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return "", "", fmt.Errorf("%w: stage %q: %w", errUtils.ErrArtifactPackagingInvalid, rel, copyErr)
	}
	after, err := src.Stat()
	if err != nil {
		return "", "", changedError(rel, err)
	}
	if copied != before.Size() || !sameFileState(before, after) {
		return "", "", changedError(rel, errSizeOrTimeChanged)
	}
	return stagedPath, digest.NewDigest(digest.SHA256, hasher), nil
}

// errSizeOrTimeChanged explains a file that changed while it was being read.
var errSizeOrTimeChanged = errors.New("changed size or modification time while it was being read")

// errReplaced explains a path that was swapped for another file while it was being captured.
var errReplaced = errors.New("was replaced while it was being captured")

// openSame opens abs and confirms it is still the regular file that was lstat'd.
func openSame(abs, rel string, before os.FileInfo) (*os.File, error) {
	src, err := os.Open(abs)
	if err != nil {
		return nil, changedError(rel, err)
	}
	opened, err := src.Stat()
	if err != nil {
		_ = src.Close()
		return nil, changedError(rel, err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		_ = src.Close()
		return nil, changedError(rel, errReplaced)
	}
	return src, nil
}

// sameFileState reports whether two stats describe the same unmodified file state.
func sameFileState(a, b os.FileInfo) bool {
	return a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && a.Mode() == b.Mode()
}

// recheck verifies every captured source entry still matches what was staged: type, size,
// modification time, directory listing, link target, and file digest.
func (c *capturer) recheck() error {
	for index := range c.sources {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		if err := c.recheckSource(&c.sources[index]); err != nil {
			return err
		}
	}
	for abs, names := range c.dirs {
		current, err := readDirNames(abs)
		if err != nil {
			return err
		}
		if !slices.Equal(current, names) {
			return fmt.Errorf("%w: directory %q gained or lost entries while it was being captured", errUtils.ErrArtifactSourceChanged, abs)
		}
	}
	return nil
}

func (c *capturer) recheckSource(record *sourceRecord) error {
	now, err := os.Lstat(record.abs)
	if err != nil {
		return changedError(record.rel, err)
	}
	if now.Mode().Type() != record.info.Mode().Type() {
		return fmt.Errorf("%w: %q changed type while it was being captured", errUtils.ErrArtifactSourceChanged, record.rel)
	}
	switch {
	case record.digest != "":
		return c.recheckFile(record, now)
	case record.target != "":
		current, err := os.Readlink(record.abs)
		if err != nil || filepath.ToSlash(current) != record.target {
			return fmt.Errorf("%w: symlink %q changed while it was being captured", errUtils.ErrArtifactSourceChanged, record.rel)
		}
	}
	return nil
}

// recheckFile compares stat state and then re-hashes the source file against the staged digest.
func (c *capturer) recheckFile(record *sourceRecord, now os.FileInfo) error {
	if !sameFileState(record.info, now) {
		return fmt.Errorf("%w: %q changed size, mode, or modification time while it was being captured", errUtils.ErrArtifactSourceChanged, record.rel)
	}
	file, err := os.Open(record.abs)
	if err != nil {
		return changedError(record.rel, err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := copyContext(c.ctx, hasher, file); err != nil {
		return fmt.Errorf("%w: recheck %q: %w", errUtils.ErrArtifactSourceChanged, record.rel, err)
	}
	if digest.NewDigest(digest.SHA256, hasher) != record.digest {
		return fmt.Errorf("%w: %q content changed while it was being captured", errUtils.ErrArtifactSourceChanged, record.rel)
	}
	return nil
}
