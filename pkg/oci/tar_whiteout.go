package oci

import (
	"archive/tar"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
)

const (
	whiteoutPrefix   = ".wh."
	opaqueWhiteout   = ".wh..wh..opq"
	tarSpoolFileMode = 0o600
	tarRootPath      = "."
)

type tarWhiteout struct {
	target string
	opaque bool
}

// tarLayer keeps its spool on the destination volume and confines cleanup to the
// same opened root, even if the extraction directory is renamed.
type tarLayer struct {
	*os.File
	root *os.Root
	name string
}

// checkPath prevents archive entries and aliases from overwriting the active spool.
func (layer *tarLayer) checkPath(name string) error {
	spoolInfo, err := layer.Stat()
	if err != nil {
		return err
	}
	current := tarRootPath
	for _, part := range strings.Split(filepath.FromSlash(name), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := layer.root.Stat(current)
		if err == nil && os.SameFile(info, spoolInfo) {
			return fmt.Errorf("%w: archive entry targets internal spool: %s", ErrInvalidFilePath, name)
		}
		// Check every prefix: descendants of the spool return ENOTDIR, while
		// unrelated missing/nondirectory whiteout parents must remain harmless.
		// Other resolution errors remain subject to the confined extraction rules.
	}
	return nil
}

// prepareTarLayer spools a layer so whiteouts affect only lower layers, even when
// their headers follow same-layer files. Payloads stay on disk rather than in memory.
func prepareTarLayer(reader io.Reader, root *os.Root, extractPath string) (*tarLayer, error) {
	name := ".atmos-oci-layer-" + rand.Text() + ".tar"
	file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, tarSpoolFileMode)
	if err != nil {
		return nil, err
	}
	layer := &tarLayer{File: file, root: root, name: name}
	ready := false
	defer func() {
		if !ready {
			removeTarLayer(layer)
		}
	}()
	whiteouts, err := scanTarWhiteouts(tar.NewReader(io.TeeReader(reader, layer)), extractPath, layer)
	if err != nil {
		return nil, err
	}
	for _, whiteout := range whiteouts {
		if err := applyTarWhiteout(root, whiteout, layer.name); err != nil {
			return nil, err
		}
	}
	if _, err := layer.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	ready = true
	return layer, nil
}

func removeTarLayer(layer *tarLayer) {
	_ = layer.Close()
	_ = layer.root.Remove(layer.name)
}

// scanTarWhiteouts validates every header before any deletion or extraction.
func scanTarWhiteouts(reader *tar.Reader, extractPath string, layer *tarLayer) ([]tarWhiteout, error) {
	var whiteouts []tarWhiteout
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return whiteouts, nil
		}
		if err != nil {
			return nil, err
		}
		if err := layer.validatePath(header.Name, extractPath); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(path.Base(header.Name), whiteoutPrefix) {
			continue
		}
		whiteout, err := parseTarWhiteout(header)
		if err != nil {
			return nil, err
		}
		if err := layer.checkPath(whiteout.target); err != nil {
			return nil, err
		}
		whiteouts = append(whiteouts, whiteout)
	}
}

func (layer *tarLayer) validatePath(name, extractPath string) error {
	if _, err := filesystem.SafeJoin(extractPath, name); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidFilePath, name)
	}
	if err := layer.checkPath(name); err != nil {
		return err
	}
	for _, parent := range strings.Split(path.Dir(name), "/") {
		if strings.HasPrefix(parent, whiteoutPrefix) {
			return fmt.Errorf("%w: whiteout cannot be a parent directory: %s", ErrInvalidFilePath, name)
		}
	}
	return nil
}

func parseTarWhiteout(header *tar.Header) (tarWhiteout, error) {
	base := path.Base(header.Name)
	name := strings.TrimPrefix(base, whiteoutPrefix)
	if name == "" || name == "." || name == ".." {
		return tarWhiteout{}, fmt.Errorf("%w: invalid whiteout %s", ErrInvalidFilePath, header.Name)
	}
	if header.Typeflag != tar.TypeReg || header.Size != 0 {
		return tarWhiteout{}, fmt.Errorf("%w: whiteout must be an empty regular file: %s", errUtils.ErrLayerExtraction, header.Name)
	}
	directory := path.Dir(header.Name)
	if base == opaqueWhiteout {
		return tarWhiteout{target: filepath.FromSlash(directory), opaque: true}, nil
	}
	return tarWhiteout{target: filepath.Join(filepath.FromSlash(directory), name)}, nil
}

func applyTarWhiteout(root *os.Root, whiteout tarWhiteout, spoolName string) error {
	parent := filepath.Dir(whiteout.target)
	if whiteout.opaque {
		parent = whiteout.target
	}
	exists, err := whiteoutDirectoryExists(root, parent)
	if err != nil || !exists {
		return err
	}
	if !whiteout.opaque {
		return root.RemoveAll(whiteout.target)
	}
	return applyOpaqueWhiteout(root, whiteout.target, spoolName)
}

func applyOpaqueWhiteout(root *os.Root, target, spoolName string) error {
	directory, err := root.Open(target)
	if err != nil {
		return err
	}
	defer directory.Close()
	children, err := directory.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, child := range children {
		if target == tarRootPath && child.Name() == spoolName {
			continue // Keep the active spool until extraction and close finish on every OS.
		}
		if err := root.RemoveAll(filepath.Join(target, child.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Whiteouts may unlink a symlink itself, but never traverse a symlink parent.
// Missing parents mean there is no lower-layer entry to remove.
func whiteoutDirectoryExists(root *os.Root, directory string) (bool, error) {
	current := tarRootPath
	for _, part := range strings.Split(filepath.Clean(directory), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("%w: whiteout traverses symlink %s", ErrInvalidFilePath, current)
		}
		if !info.IsDir() {
			return false, nil
		}
	}
	return true, nil
}
