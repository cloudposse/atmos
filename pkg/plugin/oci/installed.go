package oci

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
)

const (
	permissionMask  = fs.ModePerm
	specialModeBits = fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
)

// VerifyInstalled compares executable and metadata files to the digest-verified
// cached payload. A mutable receipt cannot substitute for artifact integrity.
func (r *Release) VerifyInstalled(platform, directory string) error {
	defer perf.Track(nil, "pluginoci.Release.VerifyInstalled")()

	descriptor, found := r.Platforms[platform]
	if !found {
		return errUtils.ErrPluginNotInstalled
	}
	raw, err := r.verifiedBundle(&descriptor)
	if err != nil {
		// Blobs the cached layout no longer serves intact are cache corruption, not a registry fault.
		if errors.Is(err, errUtils.ErrOCIIntegrity) || errors.Is(err, fs.ErrNotExist) {
			return corruption(err)
		}
		return err
	}
	return verifyArchiveFiles(directory, raw)
}

// corruption marks a payload that no longer matches the digest-verified archive. It
// also satisfies ErrOCIIntegrity so existing integrity checks keep matching.
func corruption(causes ...error) error {
	return errors.Join(append([]error{errUtils.ErrPluginCacheCorrupt, errUtils.ErrOCIIntegrity}, causes...)...)
}

// payloadSurface is the exact set of files, with their modes, the verified archive
// describes. Anything else found in the payload directory is tampering.
type payloadSurface struct {
	files       map[string]fs.FileMode
	directories map[string]bool
}

func (r *Release) verifiedBundle(descriptor *v1.Descriptor) ([]byte, error) {
	img, err := r.Index.Image(descriptor.Digest)
	if err != nil {
		return nil, err
	}
	imageManifest, err := verifiedManifest(img, descriptor)
	if err != nil {
		return nil, err
	}
	layer, err := img.LayerByDigest(imageManifest.Layers[0].Digest)
	if err != nil {
		return nil, err
	}
	reader, err := layer.Compressed()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return atmosoci.ReadVerifiedBlob(reader, &imageManifest.Layers[0], maxBundleSize)
}

func verifyArchiveFiles(directory string, raw []byte) error {
	root, err := os.OpenRoot(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return corruption(err)
	}
	if err != nil {
		return err
	}
	defer root.Close()
	archive := tar.NewReader(bytes.NewReader(raw))
	surface := &payloadSurface{files: make(map[string]fs.FileMode), directories: map[string]bool{".": true}}
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return surface.verifyNoExtras(root)
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			return errUtils.ErrOCIIntegrity
		}
		if err := surface.verifyFile(root, header, archive); err != nil {
			return err
		}
	}
}

func (s *payloadSurface) verifyFile(root *os.Root, header *tar.Header, archive io.Reader) error {
	name := path.Clean(header.Name)
	if !manifest.LocalPath(name) {
		return errUtils.ErrPluginInvalidConfig
	}
	expectedMode := fs.FileMode(header.Mode & int64(permissionMask))
	actual, err := readPayloadFile(root, name, expectedMode)
	if err != nil {
		return err
	}
	if int64(len(actual)) != header.Size {
		return corruption()
	}
	expected, err := io.ReadAll(io.LimitReader(archive, maxBundleSize+1))
	if err != nil {
		return err
	}
	if !bytes.Equal(expected, actual) {
		return corruption()
	}
	s.files[name] = expectedMode
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		s.directories[parent] = true
	}
	return nil
}

// readPayloadFile reads one regular file confined to the payload root and compares
// its permission bits. Symlinks, special files, and oversized files are corruption.
func readPayloadFile(root *os.Root, name string, expectedMode fs.FileMode) ([]byte, error) {
	info, err := root.Lstat(filepath.FromSlash(name))
	if err != nil {
		return nil, payloadError(err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxBundleSize {
		return nil, corruption()
	}
	if runtime.GOOS != "windows" && info.Mode()&(permissionMask|specialModeBits) != expectedMode {
		return nil, corruption()
	}
	file, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return nil, payloadError(err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBundleSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBundleSize {
		return nil, corruption()
	}
	return raw, nil
}

func payloadError(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return corruption(err)
	}
	return err
}

// verifyNoExtras walks the payload for files, links, or directories the verified
// archive does not contain, such as a dropped-in shared library or helper binary.
func (s *payloadSurface) verifyNoExtras(root *os.Root) error {
	return fs.WalkDir(root.FS(), ".", func(entry string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dirEntry.IsDir() {
			if !s.directories[entry] {
				return corruption()
			}
			return nil
		}
		if _, found := s.files[entry]; !found {
			return corruption()
		}
		return nil
	})
}
