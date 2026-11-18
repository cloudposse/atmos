package configuration

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// manifestName records, per working directory, the files this host generated and the digest of
// the bytes it wrote. It lets a later run remove a file that is no longer generated and tell a
// file it wrote earlier from one that merely has the same name. It lives in the .atmos directory,
// which saved-plan digests and source refreshes already treat as bookkeeping.
const manifestName = ".atmos/generated-files.json"

// lockFileName is Terraform's dependency lock file. It is copied from beside the unit and then
// belongs to Terraform, which rewrites it during init.
const lockFileName = ".terraform.lock.hcl"

// manifest maps a generated path (slash separated) to the SHA-256 of the bytes written.
type manifest struct {
	Files map[string]string `json:"files"`
}

// readManifest returns the recorded digests, or none when the manifest is absent or unreadable.
// An unreadable manifest only costs stale-file cleanup for one run, so it is never an error.
func readManifest(root *os.Root) map[string]string {
	directory, err := destinationDirectory(root, filepath.Dir(manifestName), false)
	if err != nil {
		return map[string]string{}
	}
	defer directory.Close()
	file, err := openExistingGeneratedFile(directory, filepath.Base(manifestName))
	if err != nil {
		return map[string]string{}
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return map[string]string{}
	}
	var stored manifest
	if json.Unmarshal(data, &stored) != nil || stored.Files == nil {
		return map[string]string{}
	}
	// Clean recorded paths, so a manifest written before keys were cleaned still matches.
	files := make(map[string]string, len(stored.Files))
	for path, digest := range stored.Files {
		files[filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))] = digest
	}
	return files
}

func writeManifest(root *os.Root, files map[string]string) error {
	directory, err := destinationDirectory(root, filepath.Dir(manifestName), len(files) != 0)
	if len(files) == 0 && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer directory.Close()
	leaf := filepath.Base(manifestName)
	if err := validateDestination(directory, leaf); err != nil {
		return err
	}
	if len(files) == 0 {
		return removeFile(directory, leaf)
	}
	data, err := json.MarshalIndent(manifest{Files: files}, "", "  ")
	if err != nil {
		return err
	}
	return writeManifestContents(directory, leaf, data)
}

// writeManifestContents checks write access without truncating the existing file. Replacing
// its directory entry then isolates hard links, while a read-only manifest still fails and
// triggers generated-file rollback. The verified handle supplies the original permissions.
func writeManifestContents(directory *os.Root, leaf string, data []byte) error {
	file, err := openExistingRegularFile(directory, leaf, os.O_WRONLY)
	if errors.Is(err, os.ErrNotExist) {
		return createFile(directory, leaf, schema.ComponentConfigurationFile{Content: data})
	}
	if err != nil {
		return err
	}
	info, statErr := file.Stat()
	if err := errors.Join(statErr, file.Close()); err != nil {
		return err
	}
	return restoreContents(directory, leaf, &fileState{content: data, mode: info.Mode().Perm()})
}

// manifestExists also reports invalid metadata destinations so an empty batch still
// validates them. Missing metadata keeps the historical no-op behavior.
func manifestExists(directory string) bool {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return !errors.Is(err, os.ErrNotExist)
	}
	defer root.Close()
	parent, err := destinationDirectory(root, filepath.Dir(manifestName), false)
	if err != nil {
		return !errors.Is(err, os.ErrNotExist)
	}
	defer parent.Close()
	_, err = parent.Lstat(filepath.Base(manifestName))
	return !errors.Is(err, os.ErrNotExist)
}

func contentDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// fileDigest returns the digest of an existing regular file, or false when there is none.
func fileDigest(root *os.Root, path string) (string, bool) {
	info, err := root.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	file, err := root.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return "", false
	}
	return contentDigest(data), true
}

// removeStaleFiles deletes files an earlier run generated that the current run does not name.
// A file is removed only while it is still what was written (matching digest) or still carries
// the Terragrunt signature; anything a user or module changed since stays untouched. A file that
// cannot be removed stays recorded in kept, so a later run retries it, and the error is returned.
func removeStaleFiles(root *os.Root, previous map[string]string, current []string, kept map[string]string, undo restorePoint) error {
	var failures error
	for _, path := range slices.Sorted(mapKeys(previous)) {
		if slices.Contains(current, path) || !filepath.IsLocal(filepath.FromSlash(path)) {
			continue
		}
		if err := removeStaleFile(root, path, previous[path], undo); err != nil {
			kept[path] = previous[path]
			failures = errors.Join(failures, fmt.Errorf("%w: remove stale generated file %s: %w", errUtils.ErrPluginInvalidConfig, path, err))
		}
	}
	return failures
}

// removeStaleFile checks ownership and removes the leaf through one pinned ordinary parent.
func removeStaleFile(root *os.Root, path, recorded string, undo restorePoint) error {
	local := filepath.FromSlash(path)
	directory, err := destinationDirectory(root, filepath.Dir(local), false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer directory.Close()
	leaf := filepath.Base(local)
	digest, exists := fileDigest(directory, leaf)
	if !exists || (digest != recorded && verifyTerragruntSignature(directory, leaf) != nil) {
		return nil
	}
	if err := undo.captureFrom(directory, path, leaf); err != nil {
		return err
	}
	if err := removeFile(directory, leaf); err != nil {
		return err
	}
	removeEmptyParents(root, filepath.Dir(local))
	return nil
}

// restorePoint remembers the files a run changes, so a run whose manifest cannot be written leaves
// the working directory as it found it instead of with changed files that nobody owns.
type restorePoint map[string]*fileState

// fileState is a file as it was before the run touched it.
type fileState struct {
	exists  bool
	content []byte
	mode    os.FileMode
}

// capture remembers a path's state the first time the run is about to change it. Only regular
// files and absent paths are recorded; unreadable or aliased paths fail before mutation.
func (r restorePoint) capture(root *os.Root, path string) error {
	if _, seen := r[path]; seen {
		return nil
	}
	local := filepath.FromSlash(path)
	directory, err := destinationDirectory(root, filepath.Dir(local), false)
	if errors.Is(err, os.ErrNotExist) {
		r[path] = &fileState{}
		return nil
	}
	if err != nil {
		return err
	}
	defer directory.Close()
	return r.captureFrom(directory, path, filepath.Base(local))
}

// captureFrom reads a regular leaf by handle in the directory that the mutation uses.
func (r restorePoint) captureFrom(directory *os.Root, path, leaf string) error {
	if _, seen := r[path]; seen {
		return nil
	}
	file, err := openExistingGeneratedFile(directory, leaf)
	if errors.Is(err, os.ErrNotExist) {
		r[path] = &fileState{}
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	r[path] = &fileState{exists: true, content: content, mode: info.Mode().Perm()}
	return nil
}

// restore puts every captured file back, removing the ones the run created.
func (r restorePoint) restore(root *os.Root) error {
	var failures error
	for _, path := range slices.Sorted(maps.Keys(r)) {
		if err := r.restoreFile(root, path); err != nil {
			failures = errors.Join(failures, fmt.Errorf("%w: restore generated file %s: %w", errUtils.ErrPluginInvalidConfig, path, err))
		}
	}
	return failures
}

func (r restorePoint) restoreFile(root *os.Root, path string) error {
	state, local := r[path], filepath.FromSlash(path)
	directory, err := destinationDirectory(root, filepath.Dir(local), state.exists)
	if !state.exists && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer directory.Close()
	leaf := filepath.Base(local)
	if err := validateDestination(directory, leaf); err != nil {
		return err
	}
	if !state.exists {
		return removeFile(directory, leaf)
	}
	return restoreContents(directory, leaf, state)
}

// restoreContents replaces the directory entry instead of truncating a possible hard link.
// A private exclusive temporary file keeps the original bytes and mode together, and both
// writes and permission changes use its handle rather than following a substituted leaf.
func restoreContents(directory *os.Root, leaf string, state *fileState) (result error) {
	temporary := "." + leaf + "." + rand.Text() + ".tmp"
	file, err := directory.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, state.mode)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, removeFile(directory, temporary)) }()
	_, writeErr := file.Write(state.content)
	var modeErr error
	if runtime.GOOS != "windows" {
		modeErr = file.Chmod(state.mode)
	}
	if err := errors.Join(writeErr, modeErr, file.Close()); err != nil {
		return err
	}
	if err := validateDestination(directory, leaf); err != nil {
		return err
	}
	return directory.Rename(temporary, leaf)
}

// removeEmptyParents tidies directories that only existed for a removed generated file.
func removeEmptyParents(root *os.Root, path string) {
	for path != "." && path != string(filepath.Separator) {
		parent, err := destinationDirectory(root, filepath.Dir(path), false)
		if err != nil {
			return
		}
		leaf := filepath.Base(path)
		info, statErr := parent.Lstat(leaf)
		if statErr != nil || !info.IsDir() {
			_ = parent.Close()
			return
		}
		err = parent.Remove(leaf)
		_ = parent.Close()
		if err != nil {
			return
		}
		path = filepath.Dir(path)
	}
}

func mapKeys(values map[string]string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for key := range values {
			if !yield(key) {
				return
			}
		}
	}
}
