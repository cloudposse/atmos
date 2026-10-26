package oci

import (
	"archive/tar"
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
	whiteoutPrefix = ".wh."
	opaqueWhiteout = ".wh..wh..opq"
)

type tarWhiteout struct {
	target string
	opaque bool
}

// prepareTarLayer spools a layer so whiteouts affect only lower layers, even when
// their headers follow same-layer files. Payloads stay on disk rather than in memory.
func prepareTarLayer(reader io.Reader, root *os.Root, extractPath string) (*os.File, error) {
	layer, err := os.CreateTemp("", "atmos-oci-layer-*.tar")
	if err != nil {
		return nil, err
	}
	ready := false
	defer func() {
		if !ready {
			removeTarLayer(layer)
		}
	}()
	whiteouts, err := scanTarWhiteouts(tar.NewReader(io.TeeReader(reader, layer)), extractPath)
	if err != nil {
		return nil, err
	}
	for _, whiteout := range whiteouts {
		if err := applyTarWhiteout(root, whiteout); err != nil {
			return nil, err
		}
	}
	if _, err := layer.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	ready = true
	return layer, nil
}

func removeTarLayer(layer *os.File) {
	_ = layer.Close()
	_ = os.Remove(layer.Name())
}

// scanTarWhiteouts validates every header before any deletion or extraction.
func scanTarWhiteouts(reader *tar.Reader, extractPath string) ([]tarWhiteout, error) {
	var whiteouts []tarWhiteout
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return whiteouts, nil
		}
		if err != nil {
			return nil, err
		}
		if _, err := filesystem.SafeJoin(extractPath, header.Name); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidFilePath, header.Name)
		}
		for _, parent := range strings.Split(path.Dir(header.Name), "/") {
			if strings.HasPrefix(parent, whiteoutPrefix) {
				return nil, fmt.Errorf("%w: whiteout cannot be a parent directory: %s", ErrInvalidFilePath, header.Name)
			}
		}
		if !strings.HasPrefix(path.Base(header.Name), whiteoutPrefix) {
			continue
		}
		whiteout, err := parseTarWhiteout(header)
		if err != nil {
			return nil, err
		}
		whiteouts = append(whiteouts, whiteout)
	}
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

func applyTarWhiteout(root *os.Root, whiteout tarWhiteout) error {
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
	directory, err := root.Open(whiteout.target)
	if err != nil {
		return err
	}
	defer directory.Close()
	children, err := directory.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := root.RemoveAll(filepath.Join(whiteout.target, child.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Whiteouts may unlink a symlink itself, but never traverse a symlink parent.
// Missing parents mean there is no lower-layer entry to remove.
func whiteoutDirectoryExists(root *os.Root, directory string) (bool, error) {
	current := "."
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
