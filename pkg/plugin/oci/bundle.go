package oci

import (
	"archive/tar"
	"bytes"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
)

const (
	metadataFileMode   = 0o644
	executableFileMode = 0o755
)

const maxMetadataSize = 8 << 20

func packageFiles(directory string) (map[string][]byte, error) {
	raw, err := manifest.ReadFile(directory, manifestFileName)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{manifestFileName: raw}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if info, err := root.Lstat(schemasDirectory); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, forbiddenEntry(schemasDirectory, "link")
	}
	err = fs.WalkDir(root.FS(), schemasDirectory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return collectSchemaFile(directory, path, entry, files)
	})
	return files, ignoreNotExist(err)
}

// openPackageFile opens a regular file inside the package without following
// links, so a symlinked path can never smuggle other content into a release.
func openPackageFile(directory, path string) (*os.File, int64, error) {
	if err := CheckBundlePath(path); err != nil {
		return nil, 0, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, 0, err
	}
	defer root.Close()
	segments := strings.Split(path, "/")
	for index := range segments {
		prefix := strings.Join(segments[:index+1], "/")
		info, err := root.Lstat(prefix)
		if err != nil {
			return nil, 0, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, 0, forbiddenEntry(prefix, "link")
		}
	}
	file, err := root.Open(path)
	if err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, 0, forbiddenEntry(path, "non-regular file")
	}
	return file, info.Size(), nil
}

func readPackageFile(directory, path string, limit int64) ([]byte, error) {
	file, size, err := openPackageFile(directory, path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if size > limit {
		return nil, errUtils.ErrPluginPayloadTooLarge
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errUtils.ErrPluginPayloadTooLarge
	}
	return raw, nil
}

func bundle(files map[string][]byte, executable string, binary []byte) ([]byte, error) {
	if err := CheckBundlePath(executable); err != nil {
		return nil, err
	}
	if _, exists := files[executable]; exists {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	contents := maps.Clone(files)
	contents[filepath.ToSlash(executable)] = binary
	paths := sortedPaths(contents)
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, path := range paths {
		mode := int64(metadataFileMode)
		if path == executable {
			mode = executableFileMode
		}
		if buffer.Len()+len(contents[path]) > maxBundleSize {
			return nil, errUtils.ErrPluginPayloadTooLarge
		}
		if err := writeBundleFile(writer, path, mode, contents[path]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if buffer.Len() > maxBundleSize {
		return nil, errUtils.ErrPluginPayloadTooLarge
	}
	return buffer.Bytes(), nil
}

func writeBundleFile(writer *tar.Writer, path string, mode int64, content []byte) error {
	if err := writer.WriteHeader(&tar.Header{Name: path, Mode: mode, Size: int64(len(content)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}); err != nil {
		return err
	}
	if _, err := writer.Write(content); err != nil {
		return err
	}
	return nil
}

func sortedPaths(contents map[string][]byte) []string {
	paths := make([]string, 0, len(contents))
	for path := range contents {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	return paths
}

func collectSchemaFile(directory, path string, entry fs.DirEntry, files map[string][]byte) error {
	if err := CheckBundlePath(path); err != nil {
		return err
	}
	if entry.IsDir() {
		return nil
	}
	if !entry.Type().IsRegular() {
		return forbiddenEntry(path, "link or special file")
	}
	if !strings.HasSuffix(path, ".json") {
		return errUtils.ErrPluginInvalidConfig
	}
	raw, err := manifest.ReadFile(directory, path)
	if err != nil {
		return err
	}
	files[path] = raw
	return nil
}
