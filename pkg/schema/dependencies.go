package schema

import (
	"errors"
	"fmt"
)

// Canonical kind values for path-based dependency entries used internally on
// ComponentDependency. These exist so describe-affected and related code paths
// can detect file/folder dependencies regardless of whether they were declared
// inline (legacy `kind: file` / `kind: folder` inside `dependencies.components[]`)
// or via the cleaner sibling keys (`dependencies.files` / `dependencies.folders`).
const (
	dependencyKindFile   = "file"
	dependencyKindFolder = "folder"
)

// Sentinel errors returned by Dependencies.Normalize. Defined locally to avoid
// an import cycle with the pkg/perf-anchored errors package.
var (
	// ErrComponentDependencyNameConflict is returned when a single dependency
	// entry sets both `name` (v2 alias) and `component` (canonical) to
	// different non-empty values.
	ErrComponentDependencyNameConflict = errors.New("component dependency has both 'name' and 'component' set with different values")
	// ErrComponentDependencyMissingPath is returned when an inline path-based
	// dependency entry (`kind: file` or `kind: folder`) lacks the `path` field.
	ErrComponentDependencyMissingPath = errors.New("path-based component dependency is missing 'path'")
)

// ComponentDependency represents a single dependency entry. It supports two
// surfaces simultaneously:
//
//   - The canonical (recommended) shape uses `name` for the component instance
//     name and an optional `kind` for cross-type dependencies (helmfile, packer,
//     plugin). Path-based dependencies should be declared using the sibling
//     keys `dependencies.files` / `dependencies.folders` rather than entries
//     here.
//
//   - The legacy shape uses `component` (instead of `name`) and supports
//     `kind: file` / `kind: folder` with `path:` to declare path-based
//     dependencies inline. This shape continues to parse for backward
//     compatibility with stack configurations from Atmos < v1.211.0.
//
// Use the helpers IsFileDependency / IsFolderDependency / IsComponentDependency
// to discriminate; do not branch on `Kind` directly.
type ComponentDependency struct {
	// Component instance name (canonical). This is the name under
	// components.<kind>.<name>, not the Terraform module path.
	Component string `yaml:"component,omitempty" json:"component,omitempty" mapstructure:"component"`
	// Name is an input-side alias for Component introduced in the v2 surface.
	// It is normalized into Component by Dependencies.Normalize. Code should
	// always read from Component, never from Name.
	Name string `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	// Stack name (optional, defaults to current stack). Supports Go templates.
	Stack string `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
	// Kind specifies the dependency type: terraform, helmfile, packer, plugin
	// type, or — in the legacy inline shape — file/folder. Defaults to the
	// declaring component's type for component dependencies.
	Kind string `yaml:"kind,omitempty" json:"kind,omitempty" mapstructure:"kind"`
	// Path for file or folder dependencies (legacy inline shape). For new
	// configurations, prefer the sibling keys `dependencies.files` /
	// `dependencies.folders`.
	Path string `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`

	// Legacy context fields from settings.depends_on format.
	// These are only populated when reading from the deprecated settings.depends_on format.
	// For new dependencies.components format, use the stack field with templates instead.
	Namespace   string `yaml:"-" json:"-" mapstructure:"namespace"`
	Tenant      string `yaml:"-" json:"-" mapstructure:"tenant"`
	Environment string `yaml:"-" json:"-" mapstructure:"environment"`
	Stage       string `yaml:"-" json:"-" mapstructure:"stage"`
}

// IsFileDependency returns true if this is a file dependency.
func (d *ComponentDependency) IsFileDependency() bool {
	return d.Kind == dependencyKindFile
}

// IsFolderDependency returns true if this is a folder dependency.
func (d *ComponentDependency) IsFolderDependency() bool {
	return d.Kind == dependencyKindFolder
}

// IsComponentDependency returns true if this is a component dependency (not file or folder).
func (d *ComponentDependency) IsComponentDependency() bool {
	return d.Kind != dependencyKindFile && d.Kind != dependencyKindFolder
}

// Dependencies declares required tools and component dependencies.
//
// The canonical (v2) input surface has four sibling keys: `tools`,
// `components`, `files`, `folders`. The legacy v1 surface (which still
// parses) puts file/folder entries inside `components[]` discriminated by
// `kind: file` / `kind: folder`. After unmarshaling from a stack manifest,
// callers MUST invoke Normalize so both surfaces are reconciled into a
// single internal representation: `Components` ends up holding every
// dependency entry (component, file, and folder kinds), which is what the
// downstream describe-affected / describe-dependents pipelines consume.
type Dependencies struct {
	// Tools maps tool names to version constraints (e.g., "terraform": "1.5.0" or "latest").
	Tools map[string]string `yaml:"tools,omitempty" json:"tools,omitempty" mapstructure:"tools"`
	// Components lists component dependencies that must be applied before this component.
	// Uses list format with always-append merge behavior (child lists extend parent lists).
	// After Normalize, this slice also contains synthetic entries for any sibling
	// `files` / `folders` declarations (Kind set to "file" / "folder").
	Components []ComponentDependency `yaml:"components,omitempty" json:"components,omitempty" mapstructure:"components"`
	// Files lists file paths whose changes mark the declaring component as
	// affected by `atmos describe affected`. Each entry may be a plain string
	// (the file path) or, in the future, an object with additional options.
	Files []string `yaml:"files,omitempty" json:"files,omitempty" mapstructure:"files"`
	// Folders lists folder paths whose changes mark the declaring component as
	// affected by `atmos describe affected`. Each entry may be a plain string
	// (the folder path) or, in the future, an object with additional options.
	Folders []string `yaml:"folders,omitempty" json:"folders,omitempty" mapstructure:"folders"`
}

// Normalize reconciles the v2 (`name`, `dependencies.files`,
// `dependencies.folders`) and v1 (`component`, inline `kind: file/folder`)
// surfaces into a single internal representation.
//
// After Normalize:
//   - Every entry in Components has Component populated (not Name).
//   - Every Files entry is mirrored into Components as
//     `{Kind: "file", Path: ...}` so existing downstream filters that look at
//     Components keep working.
//   - Every Folders entry is mirrored into Components as
//     `{Kind: "folder", Path: ...}`.
//   - Files and Folders are kept populated so future code paths can read from
//     the typed slices directly.
//
// Returns an error if any entry has both `component` and `name` set to
// different non-empty values, or if a path-based entry is missing `path`.
func (d *Dependencies) Normalize() error {
	if d == nil {
		return nil
	}
	if err := d.normalizeComponentEntries(); err != nil {
		return err
	}
	d.mirrorSiblingsIntoComponents()
	d.backfillTypedSlicesFromInline()
	return nil
}

// normalizeComponentEntries resolves the name↔component alias on every entry
// and validates that any inline path-based entry has a non-empty path.
func (d *Dependencies) normalizeComponentEntries() error {
	for i := range d.Components {
		entry := &d.Components[i]
		if err := normalizeNameAlias(entry, i); err != nil {
			return err
		}
		if (entry.IsFileDependency() || entry.IsFolderDependency()) && entry.Path == "" {
			return fmt.Errorf("%w (entry %d, kind=%q)", ErrComponentDependencyMissingPath, i, entry.Kind)
		}
	}
	return nil
}

// mirrorSiblingsIntoComponents appends synthetic ComponentDependency entries
// for every Files/Folders sibling-key entry so downstream code that filters
// Components[] by Kind keeps working unchanged.
func (d *Dependencies) mirrorSiblingsIntoComponents() {
	for _, p := range d.Files {
		if p == "" {
			continue
		}
		d.Components = append(d.Components, ComponentDependency{Kind: dependencyKindFile, Path: p})
	}
	for _, p := range d.Folders {
		if p == "" {
			continue
		}
		d.Components = append(d.Components, ComponentDependency{Kind: dependencyKindFolder, Path: p})
	}
}

// backfillTypedSlicesFromInline promotes any inline file/folder entries from
// Components[] into the typed Files/Folders slices, deduplicating against
// paths already declared via the sibling keys.
func (d *Dependencies) backfillTypedSlicesFromInline() {
	existingFiles := pathSet(d.Files)
	existingFolders := pathSet(d.Folders)
	for i := range d.Components {
		entry := &d.Components[i]
		switch {
		case entry.IsFileDependency():
			if _, ok := existingFiles[entry.Path]; !ok {
				d.Files = append(d.Files, entry.Path)
				existingFiles[entry.Path] = struct{}{}
			}
		case entry.IsFolderDependency():
			if _, ok := existingFolders[entry.Path]; !ok {
				d.Folders = append(d.Folders, entry.Path)
				existingFolders[entry.Path] = struct{}{}
			}
		}
	}
}

// pathSet returns a set keyed by string for cheap dedup membership checks.
func pathSet(paths []string) map[string]struct{} {
	out := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		out[p] = struct{}{}
	}
	return out
}

// normalizeNameAlias resolves the `name` ↔ `component` alias on a single
// dependency entry. It returns an error when both are set to different
// non-empty values; equal values (or only one set) succeed and leave Component
// populated.
func normalizeNameAlias(entry *ComponentDependency, index int) error {
	switch {
	case entry.Name == "":
		// Nothing to do; Component (if any) is already canonical.
		return nil
	case entry.Component == "":
		entry.Component = entry.Name
		entry.Name = ""
		return nil
	case entry.Component == entry.Name:
		// Both set to the same value — accept and clear the alias.
		entry.Name = ""
		return nil
	default:
		return fmt.Errorf(
			"%w (entry %d: name=%q, component=%q)",
			ErrComponentDependencyNameConflict,
			index,
			entry.Name,
			entry.Component,
		)
	}
}
