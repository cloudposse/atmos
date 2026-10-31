package configuration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	file, err := root.Open(manifestName)
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
	return stored.Files
}

func writeManifest(root *os.Root, files map[string]string) error {
	if len(files) == 0 {
		err := root.Remove(manifestName)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	data, err := json.MarshalIndent(manifest{Files: files}, "", "  ")
	if err != nil {
		return err
	}
	if err := root.MkdirAll(filepath.Dir(manifestName), directoryMode); err != nil {
		return err
	}
	return root.WriteFile(manifestName, data, fileMode)
}

// manifestExists reports whether the working directory has a manifest, without opening a root.
func manifestExists(directory string) bool {
	_, err := os.Stat(filepath.Join(directory, filepath.FromSlash(manifestName)))
	return err == nil
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
// the Terragrunt signature; anything a user or module changed since stays untouched.
func removeStaleFiles(root *os.Root, previous map[string]string, current []string) {
	for _, path := range slices.Sorted(mapKeys(previous)) {
		if slices.Contains(current, path) || !filepath.IsLocal(filepath.FromSlash(path)) {
			continue
		}
		local := filepath.FromSlash(path)
		digest, exists := fileDigest(root, local)
		if !exists || (digest != previous[path] && verifyTerragruntSignature(root, local) != nil) {
			continue
		}
		if root.Remove(local) == nil {
			removeEmptyParents(root, filepath.Dir(local))
		}
	}
}

// removeEmptyParents tidies directories that only existed for a removed generated file.
func removeEmptyParents(root *os.Root, directory string) {
	for directory != "." && directory != string(filepath.Separator) && root.Remove(directory) == nil {
		directory = filepath.Dir(directory)
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
