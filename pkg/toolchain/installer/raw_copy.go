package installer

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	log "github.com/charmbracelet/log"
)

// copyFile stages a raw binary beside its destination before replacing it.
// Rewriting an existing executable in place can invalidate macOS code-signing
// pages cached against its inode, even when the replacement bytes are identical.
func (i *Installer) copyFile(src, dst string) error {
	log.Debug("Copying binary", "src", filepath.Base(src), "dst", filepath.Base(dst))

	stageDir, err := os.MkdirTemp(filepath.Dir(dst), ".copy-")
	if err != nil {
		return fmt.Errorf("%w: failed to create copy staging directory: %w", ErrFileOperation, err)
	}
	defer os.RemoveAll(stageDir)

	staged := filepath.Join(stageDir, "binary")
	if err := copyFileFallback(src, staged); err != nil {
		return fmt.Errorf("%w: failed to copy file: %w", ErrFileOperation, err)
	}
	// Preserve an existing destination's permissions, matching os.Create.
	// A new destination retains the creation mode filtered through the umask.
	info, err := os.Lstat(dst)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: failed to inspect destination file: %w", ErrFileOperation, err)
	}
	if err == nil {
		unchanged, err := rawBinaryMatches(staged, dst, info)
		if err != nil {
			return fmt.Errorf("%w: failed to compare destination file: %w", ErrFileOperation, err)
		}
		if unchanged {
			return nil
		}
		if err := os.Chmod(staged, info.Mode().Perm()); err != nil {
			return fmt.Errorf("%w: failed to preserve destination permissions: %w", ErrFileOperation, err)
		}
	}
	// The sibling staging directory guarantees the same filesystem. Do not
	// fall back to an in-place copy if replacement fails.
	if err := os.Rename(staged, dst); err != nil {
		return fmt.Errorf("%w: failed to replace destination file: %w", ErrFileOperation, err)
	}
	return nil
}

// rawBinaryMatches avoids replacing an identical executable that Windows may
// still have open for scanning. Hash both complete files with bounded memory;
// neither existence nor matching size is enough to reuse installed bytes.
func rawBinaryMatches(staged, destination string, expected os.FileInfo) (bool, error) {
	if !expected.Mode().IsRegular() {
		return false, fmt.Errorf("%w: destination is not a regular file", ErrFileOperation)
	}
	source, err := os.Open(staged)
	if err != nil {
		return false, err
	}
	defer source.Close()
	installed, err := os.Open(destination)
	if err != nil {
		return false, err
	}
	defer installed.Close()
	opened, err := installed.Stat()
	if err != nil {
		return false, err
	}
	if !os.SameFile(expected, opened) {
		return false, fmt.Errorf("%w: destination changed while opening", ErrFileOperation)
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, source); err != nil {
		return false, err
	}
	stagedDigest := digest.Sum(nil)
	digest.Reset()
	if _, err := io.Copy(digest, installed); err != nil {
		return false, err
	}
	current, err := os.Lstat(destination)
	if err != nil {
		return false, err
	}
	if !os.SameFile(expected, current) {
		return false, fmt.Errorf("%w: destination changed while comparing", ErrFileOperation)
	}
	return bytes.Equal(stagedDigest, digest.Sum(nil)), nil
}
