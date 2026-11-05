package source

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// safePath rejects symlinks in existing ancestors, including the destination itself.
func safePath(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for p := absolute; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symbolic link %s", ErrInvalid, p)
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}

func within(root, relative string) (string, error) {
	// Manifest paths are portable slash-separated paths. On Windows, IsAbs
	// does not identify a rooted path without a drive, such as /absolute.
	if strings.HasPrefix(relative, "/") || filepath.IsAbs(relative) || strings.ContainsAny(relative, "\\:") {
		return "", fmt.Errorf("%w: absolute or nonportable path %s", ErrInvalid, relative)
	}
	for _, segment := range strings.Split(relative, "/") {
		if segment == ".." {
			return "", fmt.Errorf("%w: traversal %s", ErrInvalid, relative)
		}
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	return target, safePath(target)
}

func treeDigest(root string) (string, error) {
	return filteredTreeDigest(root, nil)
}

func filteredTreeDigest(root string, excluded []string) (string, error) {
	if err := safePath(root); err != nil {
		return "", err
	}
	source, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer source.Close()
	h := sha256.New()
	err = fs.WalkDir(source.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		skip, err := validateSnapshotEntry(path, d, excluded)
		if err != nil || skip {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := source.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%o\x00%d\x00", path, info.Mode().Perm()&0o111, len(data))
		_, err = h.Write(data)
		return err
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func validateTreeEntry(path string, d fs.DirEntry) (bool, error) {
	if d.Name() == ".git" && d.IsDir() {
		return true, filepath.SkipDir
	}
	if d.Type()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("%w: source symlink %s", ErrInvalid, path)
	}
	if d.IsDir() {
		return true, nil
	}
	if !d.Type().IsRegular() {
		return false, fmt.Errorf("%w: nonregular file %s", ErrInvalid, path)
	}
	return false, nil
}

func validateSnapshotEntry(path string, d fs.DirEntry, excluded []string) (bool, error) {
	if contains(excluded, path) {
		if d.IsDir() {
			return true, filepath.SkipDir
		}
		return true, nil
	}
	return validateTreeEntry(path, d)
}

func copyTree(root, target string) error {
	return copyFilteredTree(root, target, nil)
}

//nolint:revive // Keep sequential validation and I/O errors next to the operation they guard.
func copyFilteredTree(root, target string, excluded []string) error {
	if err := safePath(root); err != nil {
		return err
	}
	source, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer source.Close()
	if err = os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	dest, err := os.OpenRoot(target)
	if err != nil {
		return err
	}
	defer dest.Close()
	return fs.WalkDir(source.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		skip, err := validateSnapshotEntry(path, d, excluded)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return dest.MkdirAll(path, 0o755)
		}
		if skip {
			return nil
		}
		data, err := source.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return dest.WriteFile(path, data, 0o644|info.Mode().Perm()&0o111)
	})
}
