// Package config reads plugin declarations and resolves locally built packages without executing code.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Selection locates the code one package alias runs: a verified cached release or a local build.
type Selection struct {
	// Path is the absolute package directory that holds plugin.yaml.
	Path string
	// Executable is the executable below Path. It is empty for a local build, whose executable is
	// read from the current manifest at activation so a rebuild is picked up without reinstalling.
	Executable string
	// Local marks a package selected by plugins.packages.<alias>.path, which bypasses the lock, the
	// cache and signature evidence.
	Local bool
}

// Selections is the set of packages one command activates.
type Selections struct {
	Packages map[string]Selection
}

// NewSelections returns an empty selection.
//
//nolint:lintroller // Trivial factory.
func NewSelections() *Selections { return &Selections{Packages: make(map[string]Selection)} }

// IsLocal reports whether an enabled declaration selects a local build. A disabled package is never
// local, so it never runs. The path wins over source and version, which stay declared but unused.
//
//nolint:lintroller // Pure lookup without I/O.
func IsLocal(declaration *schema.PluginPackage) bool {
	return strings.TrimSpace(declaration.Path) != "" && (declaration.Enabled == nil || *declaration.Enabled)
}

// ResolveLocalPath returns the absolute directory of a package path. A relative path is relative to
// the project directory, the same base as the release lock.
func ResolveLocalPath(project, path string) (string, error) {
	defer perf.Track(nil, "pluginconfig.ResolveLocalPath")()

	if !filepath.IsAbs(path) {
		path = filepath.Join(project, path)
	}
	return filepath.Abs(path)
}

// LocalSelections resolves every enabled package that declares a path.
func LocalSelections(project string, packages map[string]schema.PluginPackage) (*Selections, error) {
	defer perf.Track(nil, "pluginconfig.LocalSelections")()

	result := NewSelections()
	for alias := range packages {
		declaration := packages[alias]
		if !IsLocal(&declaration) {
			continue
		}
		directory, err := ResolveLocalPath(project, strings.TrimSpace(declaration.Path))
		if err != nil {
			return nil, err
		}
		result.Packages[alias] = Selection{Path: directory, Local: true}
	}
	return result, nil
}

// DisplayPath renders a directory for a status message: relative to the current directory when it
// lies below it, absolute otherwise, so a path is never ambiguous.
func DisplayPath(path string) string {
	defer perf.Track(nil, "pluginconfig.DisplayPath")()

	if !filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return path
	}
	relative, err := filepath.Rel(workingDirectory, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return path
	}
	return relative
}
