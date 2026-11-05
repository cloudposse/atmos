package sops

import (
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

// templateOpen starts a Go-template action inside `spec.file`.
const templateOpen = "{{"

// currentDir is the widest folder: everything under the working directory.
const currentDir = "."

// possibleLocations reports where a SOPS provider definition can place its files without knowing
// the secret's coordinate, mirroring resolveFile and derivePath:
//
//   - `spec.file` without a template action is exactly one file;
//   - `spec.file` with template actions can only vary after its static prefix, so every file lives
//     under the directory of that prefix;
//   - without `spec.file`, files live under `spec.path` (default `secrets`);
//   - a `spec.file` or `spec.path` that is itself a selector is unknown, so the whole working
//     directory is covered.
func possibleLocations(spec map[string]any) providers.FileLocations {
	defer perf.Track(nil, "providers.sops.possibleLocations")()

	file, _ := spec["file"].(string)
	path, _ := spec["path"].(string)

	if file != "" {
		return fileLocations(file)
	}
	if providers.IsSelector(path) {
		return providers.FileLocations{Folders: []string{currentDir}}
	}
	if path == "" {
		path = defaultSopsPath
	}
	return providers.FileLocations{Folders: []string{filepath.Clean(path)}}
}

// fileLocations covers the files a `spec.file` value can render to.
func fileLocations(file string) providers.FileLocations {
	if providers.IsSelector(file) {
		return providers.FileLocations{Folders: []string{currentDir}}
	}
	prefix, _, templated := strings.Cut(file, templateOpen)
	if !templated {
		return providers.FileLocations{Files: []string{file}}
	}
	// The prefix may end mid-name (`secrets/prod-`), so only its directory is certain.
	return providers.FileLocations{Folders: []string{filepath.Dir(prefix)}}
}
