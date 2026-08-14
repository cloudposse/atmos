package workdir

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// WorkdirMetadata stores metadata about a working directory.
// This is persisted to .atmos/metadata.json in each workdir.
type WorkdirMetadata struct {
	// Component is the component name.
	Component string `json:"component"`

	// Stack is the stack name (optional, for stack-specific workdirs).
	Stack string `json:"stack,omitempty"`

	// SourceType indicates the source type ("local" or "remote").
	SourceType SourceType `json:"source_type"`

	// Source is the original component source path.
	Source string `json:"source,omitempty"`

	// SourceURI is the remote source URI (for remote sources).
	SourceURI string `json:"source_uri,omitempty"`

	// SourceVersion is the remote source version (for remote sources).
	SourceVersion string `json:"source_version,omitempty"`

	// SourceResolved is the credential-free canonical source recorded after
	// materialization. It can differ from a mutable declared URI.
	SourceResolved string `json:"source_resolved,omitempty"`

	// SourceIdentity is immutable provenance (Git commit, OCI manifest digest,
	// or content-tree SHA-256) for the materialized source.
	SourceIdentity string `json:"source_identity,omitempty"`

	// CreatedAt is when the workdir was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the workdir was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// LastAccessed is when the workdir was last accessed (for TTL tracking).
	LastAccessed time.Time `json:"last_accessed,omitempty"`

	// ContentHash is a hash of the source content for change detection (local sources only).
	ContentHash string `json:"content_hash,omitempty"`
}

// SourceType indicates the type of component source.
type SourceType string

const (
	// SourceTypeLocal indicates the component source is a local path.
	SourceTypeLocal SourceType = "local"

	// SourceTypeRemote indicates the component source is a remote URI.
	SourceTypeRemote SourceType = "remote"
)

// WorkdirConfig holds configuration for the workdir provisioner.
type WorkdirConfig struct {
	// Enabled controls whether workdir provisioning is active.
	// Defaults to false; set provision.workdir.enabled: true to enable.
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
}

// WorkdirPath returns the standard workdir directory name.
const WorkdirPath = ".workdir"

// AtmosDir is the Atmos-specific directory within each workdir.
const AtmosDir = ".atmos"

// MetadataFile is the name of the metadata file within AtmosDir.
const MetadataFile = "metadata.json"

// WorkdirMetadataFile is the legacy name of the metadata file.
// Deprecated: Use MetadataPath() instead.
const WorkdirMetadataFile = ".workdir-metadata.json"

// MetadataPath returns the full path to the metadata file within a workdir.
func MetadataPath(workdirPath string) string {
	defer perf.Track(nil, "workdir.MetadataPath")()

	return filepath.Join(workdirPath, AtmosDir, MetadataFile)
}

// File permission constants.
const (
	// DirPermissions is the default permission for created directories (rwxr-xr-x).
	DirPermissions = 0o755

	// FilePermissionsSecure is the secure permission for sensitive files (rw-------).
	FilePermissionsSecure = 0o600

	// FilePermissionsStandard is the standard permission for regular files (rw-r--r--).
	FilePermissionsStandard = 0o644
)

// String literal constants.
const (
	// ComponentKey is the key used to access component name in configuration.
	ComponentKey = "component"

	// WorkdirPathKey is the key used to store/retrieve the workdir path in component configuration.
	// This is set by the workdir provisioner and checked by terraform execution to override
	// the component path with the workdir path.
	WorkdirPathKey = "_workdir_path"

	// WorkdirReprovisionedKey is set in componentConfig when the workdir was actually wiped
	// and re-provisioned during this command invocation (i.e. the source provisioner ran
	// vendorToTarget, not just touched LastAccessed). This is the correct signal to use when
	// deciding whether to pass -reconfigure to terraform init: a re-provisioned workdir has
	// no .terraform/ cache, so -reconfigure is safe; a preserved workdir has a valid cache
	// and -reconfigure causes a spurious "migrate all workspaces?" prompt.
	WorkdirReprovisionedKey = "_workdir_reprovisioned"
)

// BuildPath constructs the canonical workdir path for a component instance and validates
// that it stays contained within basePath. It uses atmos_component (the instance name) from
// componentConfig when available, falling back to the provided component name. This ensures
// all provisioners (workdir, source, JIT) use the same directory for a given component
// instance. Both "/" and "\" in the resolved component name are encoded (see
// escapeComponentNameForPath) rather than left as real path segments, and the final path is
// verified to fall within basePath (see containWithinBase) -- component and stack names can
// both originate from user-controlled YAML, and either could otherwise resolve outside
// basePath via filepath.Join's implicit Clean(). Returns errUtils.ErrPathTraversal if the
// resolved path escapes basePath.
//
// Path format: <basePath>/.workdir/<componentType>/<stack>-<instanceName>.
func BuildPath(basePath, componentType, component, stack string, componentConfig map[string]any) (string, error) {
	defer perf.Track(nil, "workdir.BuildPath")()

	// Use atmos_component (instance name) for path isolation.
	// When metadata.component differs from the instance name (e.g., inherited components),
	// atmos_component contains the unique instance name while component/metadata.component
	// contains the shared base name.
	workdirComponent := component
	if atmosComponent, ok := componentConfig["atmos_component"].(string); ok && atmosComponent != "" {
		workdirComponent = atmosComponent
	}

	// A nested component name (e.g. "ecs/cluster") must not add an extra path
	// segment: filepath.Join would otherwise turn it into a real subdirectory,
	// making the workdir one level deeper than a flat component's at the same
	// stack. That silently changes how many ".." a relative backend path (or
	// any other path computed relative to the workdir) needs to reach the
	// same ancestor, so equivalent components would write state under
	// different roots solely because one component name contains "/". Mirror
	// the same sanitization already used for backend template context (see
	// internal/exec/terraform_generate_backends.go).
	//
	// "\" is sanitized identically and unconditionally (not just on Windows):
	// it is Windows' real path separator, so a crafted or copy-pasted
	// component name containing it (e.g. "..\\..\\evil") would otherwise let
	// filepath.Join/Clean treat it as real ".."-traversal segments there,
	// escaping the intended workdir root. Stripping it on every platform also
	// keeps a given component name's workdir path identical across OSes.
	workdirComponent = escapeComponentNameForPath(workdirComponent)

	workdirName := fmt.Sprintf("%s-%s", stack, workdirComponent)
	rawPath := filepath.Join(basePath, WorkdirPath, componentType, workdirName)

	return containWithinBase(rawPath, basePath)
}

// escapeComponentNameForPath encodes name so it can be used as a single filesystem path
// segment without colliding with a differently-named component. Each "/" and "\" is replaced
// with "--" (a doubled hyphen) rather than a single "-", so a name containing a path
// separator can never collide with an otherwise-identical name that uses a literal hyphen in
// its place -- e.g. "app/local" and "app-local" (which a naive "/" -> "-" substitution would
// both turn into "app-local", sharing one workdir -- and its files, metadata, and Terraform
// state -- between two distinct components) now encode to "app--local" and "app-local"
// respectively. "/" and "\" intentionally share the same encoding so a component name's
// workdir path stays identical across OSes (see
// docs/fixes/2026-08-05-workdir-nested-component-path-depth.md for that original rationale).
//
// This is not a fully injective encoding: a component name containing a literal "--" could
// still collide with a differently-placed "/" or "\". That's a deliberate trade-off -- a
// scheme that's collision-free for every possible input would have to also escape single "-"
// characters, which would change the on-disk workdir name for the overwhelming majority of
// real components (kebab-case, single-hyphen names are the standard Atmos naming
// convention), not just the rare ones containing a path separator.
func escapeComponentNameForPath(name string) string {
	replaced := strings.ReplaceAll(name, "/", "--")
	return strings.ReplaceAll(replaced, "\\", "--")
}

// containWithinBase verifies that path, once absolutized, is contained within base (equal to
// it, or nested under it, once base is also absolutized). Returns errUtils.ErrPathTraversal
// if not. On success it returns path unchanged (not the absolutized form), preserving
// BuildPath's existing return format for callers that pass a relative basePath. This is a
// best-effort, lexical-only guard -- symlinks are not resolved, matching the scope of the
// containment guards this replaces in pkg/terraform/output/config.go and
// internal/terraform_backend/terraform_backend_local.go.
func containWithinBase(path, base string) (string, error) {
	absPath, errPath := filepath.Abs(path)
	if errPath != nil {
		return "", fmt.Errorf("%w: failed to resolve workdir path %q: %w", errUtils.ErrPathTraversal, path, errPath)
	}
	absBase, errBase := filepath.Abs(base)
	if errBase != nil {
		return "", fmt.Errorf("%w: failed to resolve base path %q: %w", errUtils.ErrPathTraversal, base, errBase)
	}
	sep := string(filepath.Separator)
	if absPath == absBase || strings.HasPrefix(absPath, absBase+sep) {
		return path, nil
	}
	return "", fmt.Errorf("%w: workdir path %q escapes base path %q", errUtils.ErrPathTraversal, absPath, absBase)
}
