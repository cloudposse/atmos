package installer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// exposeOnedirCommands keeps configured command names beside their archive
// executables so PATH lookup and executable-relative support files both work.
// Copies preserve regular archive files without requiring symlink privileges.
func exposeOnedirCommands(treeDir string, eps []entrypoint, goos string) ([]entrypoint, error) {
	root, err := os.OpenRoot(treeDir)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to open installed tree: %w", ErrFileOperation, err)
	}
	defer root.Close()

	exposed := slices.Clone(eps)
	for idx, ep := range eps {
		name, err := onedirCommandName(ep, goos)
		if err != nil {
			return nil, err
		}
		if err := root.Chmod(ep.src, defaultMkdirPermissions); err != nil {
			return nil, fmt.Errorf("%w: failed to make command %s executable: %w", ErrFileOperation, ep.src, err)
		}
		base := filepath.Base(ep.src)
		if name == base || (goos == "windows" && strings.EqualFold(name, base)) {
			continue
		}
		alias := filepath.Join(filepath.Dir(ep.src), name)
		if err := copyOnedirCommand(root, ep.src, alias); err != nil {
			return nil, err
		}
		exposed[idx].src = alias
	}
	return exposed, nil
}

// onedirCommandName validates the configured name and preserves Windows executable extensions.
func onedirCommandName(ep entrypoint, goos string) (string, error) {
	name := ep.name
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("%w: invalid command name %q", ErrFileOperation, name)
	}
	if goos == "windows" && strings.EqualFold(filepath.Ext(ep.src), windowsExeExt) {
		name = ensureWindowsExeExtensionForOS(name, goos)
	}
	return name, nil
}

// copyOnedirCommand refuses collisions and keeps every read and write inside the
// preserved tree, including paths that traverse an archive-provided symlink.
func copyOnedirCommand(root *os.Root, source, destination string) error {
	input, err := root.Open(source)
	if err != nil {
		return fmt.Errorf("%w: failed to open command %s: %w", ErrFileOperation, source, err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return fmt.Errorf("%w: failed to inspect command %s: %w", ErrFileOperation, source, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: command %s is not a regular file", ErrFileOperation, source)
	}
	entry, err := root.Lstat(source)
	if err != nil {
		return fmt.Errorf("%w: failed to inspect command entry %s: %w", ErrFileOperation, source, err)
	}
	if entry.Mode()&os.ModeSymlink != 0 {
		// Reuse the archive's link so the executable still resolves beside its
		// runtime files. Copying its target into the link's directory can break it.
		if err := root.Link(source, destination); err != nil {
			return fmt.Errorf("%w: failed to expose linked command %s: %w", ErrFileOperation, destination, err)
		}
		return nil
	}
	output, err := root.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("%w: failed to expose command %s: %w", ErrFileOperation, destination, err)
	}
	_, copyErr := io.Copy(output, input)
	if err := errors.Join(copyErr, output.Close()); err != nil {
		return fmt.Errorf("%w: failed to copy command %s: %w", ErrFileOperation, destination, err)
	}
	return nil
}
