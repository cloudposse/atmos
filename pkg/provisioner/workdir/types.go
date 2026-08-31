package workdir

import (
	"crypto/sha256"
	"encoding/hex"
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

// workdirHashLength is the number of hex characters kept from the sha256 hash used to
// disambiguate two different (stack, component) pairs whose sanitized display prefixes
// collide (see workdirPathHash). 8 hex characters is 32 bits: a collision is only possible
// between the rare pairs that already share a sanitized prefix (see
// sanitizeComponentNameForPath), and createWorkdirDirectory's identity check (see
// verifyWorkdirIdentity) fails closed rather than silently reusing another identity's workdir
// in the vanishingly unlikely event one occurs.
const workdirHashLength = 8

// BuildPath constructs the canonical workdir path for a component instance and validates
// that it stays contained within basePath. It uses atmos_component (the instance name) from
// componentConfig when available, falling back to the provided component name. This ensures
// all provisioners (workdir, source, JIT) use the same directory for a given component
// instance.
//
// The final path segment is "<stack>-<sanitized-component>-<hash>": sanitized-component is a
// purely cosmetic, lossy rendering of the resolved component name (see
// sanitizeComponentNameForPath) -- "/" and "\" become "-", nothing else changes -- and hash is
// a short content hash of stack+component (see workdirPathHash) that guarantees uniqueness
// even when two different component names sanitize to the same display prefix.
//
// Component name is kept to a single path segment rather than a real nested directory even
// when it contains "/" (unlike stack -- see validateStackForPath) so every workdir sits at the
// same fixed depth under basePath regardless of a component's own naming: some components use
// a relative Terraform module `source` (e.g. `../../account-map/modules/...`) that assumes a
// fixed number of ".." hops from the component's own directory, and a workdir whose depth
// varied based on an unrelated component-naming choice would silently break that for exactly
// the components whose name happens to contain "/".
//
// The final path is verified to fall within basePath (see containWithinBase) -- component and
// stack names can both originate from user-controlled YAML, and either could otherwise resolve
// outside basePath (or alias another stack's workdir) via filepath.Join's implicit Clean().
// Returns errUtils.ErrPathTraversal if stack contains a "."/".." segment or the resolved path
// escapes basePath.
//
// Path format: <basePath>/.workdir/<componentType>/<stack>-<sanitized-component>-<hash>.
func BuildPath(basePath, componentType, component, stack string, componentConfig map[string]any) (string, error) {
	defer perf.Track(nil, "workdir.BuildPath")()

	if err := validateStackForPath(stack); err != nil {
		return "", err
	}

	rawComponent := resolveWorkdirComponentName(component, componentConfig)
	workdirName := fmt.Sprintf("%s-%s-%s", stack, sanitizeComponentNameForPath(rawComponent), workdirPathHash(stack, rawComponent))
	rawPath := filepath.Join(basePath, WorkdirPath, componentType, workdirName)

	// containWithinBase is kept as a defense-in-depth check against typeRoot
	// (itself always safely nested under basePath, since componentType is a
	// fixed, trusted caller-supplied constant such as "terraform", not
	// user-controlled input) even though rejecting "."/".." segments in
	// stack and stripping "/"/"\" out of the component's sanitized prefix
	// above should already make escape impossible -- it costs nothing and
	// protects against a future change to either guard reintroducing a real
	// traversal segment.
	typeRoot := filepath.Join(basePath, WorkdirPath, componentType)

	return containWithinBase(rawPath, typeRoot)
}

// resolveWorkdirComponentName resolves the instance-name override (atmos_component) from
// componentConfig, falling back to component. The returned value is intentionally
// unsanitized -- BuildPath is responsible for turning it into a safe, unique path segment (see
// sanitizeComponentNameForPath and workdirPathHash) -- so this function only decides which name
// wins, not how it gets encoded.
//
// A nested or otherwise separator-bearing name (e.g. "ecs/cluster") must not add an extra path
// segment or otherwise change the workdir's depth under basePath -- see BuildPath's doc comment
// for why that matters.
func resolveWorkdirComponentName(component string, componentConfig map[string]any) string {
	// Use atmos_component (instance name) for path isolation.
	// When metadata.component differs from the instance name (e.g., inherited components),
	// atmos_component contains the unique instance name while component/metadata.component
	// contains the shared base name.
	if atmosComponent, ok := componentConfig["atmos_component"].(string); ok && atmosComponent != "" {
		return atmosComponent
	}

	return component
}

// validateStackForPath rejects a stack name containing a "." or ".." path segment, an empty
// segment produced by a leading or repeated "/", or any "\" character, instead of escaping the
// whole value the way resolveWorkdirComponentName does for the component name.
//
// A stack name is just as user-controlled as a component name, and without the "."/".." check, a
// value such as "team/../prod" still contains a real ".." segment when workdirName is built --
// filepath.Join's implicit Clean() then folds "team/../" away, silently aliasing stack
// "team/../prod" onto the same workdir as stack "prod", so two distinct stack configurations
// would share one workdir (and therefore its files, metadata, and Terraform state).
//
// A stack containing "/" without a "."/".." segment -- e.g. "deploy/test", a real,
// already-tested naming convention in this codebase -- is not rejected: it becomes a real
// nested directory ("<typeRoot>/deploy/test-<component>") exactly as it always has (stack was
// never escaped like the component name; only ".."'s Clean()-folding is a distinct-path-alias
// risk, plain "/" nesting is not). Rejecting only dot segments, rather than every "/" (this
// function's earlier revision), avoids breaking that existing convention while still closing
// the collision: "deploy/test" and "deploy" can never alias each other through Clean(), only
// "x/../y" and "y" can.
//
// A leading or repeated "/" is rejected too, for the same Clean()-folding reason: this
// function's earlier revision split stack on strings.FieldsFunc, which silently drops empty
// segments, so "/deploy/test" and "deploy//test" both produced the same two-element segment
// list ["deploy", "test"] that "deploy/test" itself does -- neither a leading nor a doubled "/"
// was ever individually visible to the loop below. That matters because filepath.Join's
// implicit Clean() collapses a leading or doubled "/" the same way BuildPath's workdirName
// does: stack "/deploy/test" and stack "deploy//test" both clean down to the identical path
// "deploy/test-<component>" that stack "deploy/test" itself produces -- the very
// silent-aliasing collision this whole function exists to reject, just triggered by a
// different Clean()-significant construct than "."/"..". A single *trailing* "/" is not
// affected the same way and is deliberately left alone: BuildPath always appends
// "-<component>" directly onto stack with no separator in between, so a trailing "/" becomes
// its own real path segment (".../test/-<component>") distinct from the non-trailing form
// (".../test-<component>") -- it does not alias its non-trailing counterpart through Clean(),
// so there is nothing to close for it.
//
// "\" is rejected outright rather than merely split on for dot-detection, as this function's
// earlier revision did: unlike "/", the one nesting notation this package documents and tests
// as supported (see above), "\" was never adopted as an equivalent second notation -- splitting
// on it existed only to catch a dot segment however it was delimited (e.g. `team\..\prod`).
// But filepath.Join/Clean on Windows treat "\" and "/" as fully interchangeable separators, so
// a stack such as `deploy\test`, left unrejected, would alias the very same workdir as stack
// "deploy/test" produces on that platform -- again the same collision class this function
// exists to close, just Windows-specific. Rejecting it unconditionally (not only on Windows)
// keeps a given stack name's validity independent of which OS Atmos runs on.
func validateStackForPath(stack string) error {
	if strings.ContainsRune(stack, '\\') {
		return errUtils.Build(errUtils.ErrPathTraversal).
			WithExplanationf("Stack name `%s` must not contain a '\\' character", stack).
			WithHint("Use '/' (not '\\') to nest a stack name").
			WithContext("stack", stack).
			Err()
	}

	segments := strings.Split(stack, "/")
	for i, segment := range segments {
		// An empty segment from a leading or repeated "/" is Clean()-significant the same way
		// "."/".." are (see doc comment above) -- but only when it is not the final segment: a
		// single trailing "/" does not alias its non-trailing counterpart, so it is allowed
		// through.
		if segment == "" && i != len(segments)-1 {
			return errUtils.Build(errUtils.ErrPathTraversal).
				WithExplanationf("Stack name `%s` must not contain a leading or repeated '/' path separator", stack).
				WithHint("Remove any leading or doubled '/' characters from the stack name").
				WithContext("stack", stack).
				Err()
		}
		if segment == "." || segment == ".." {
			return errUtils.Build(errUtils.ErrPathTraversal).
				WithExplanationf("Stack name `%s` must not contain a '.' or '..' path segment", stack).
				WithHint("Remove any '.' or '..' segments from the stack name").
				WithContext("stack", stack).
				Err()
		}
	}

	return nil
}

// sanitizeComponentNameForPath produces a purely cosmetic, lossy rendering of name for use in
// a workdir directory name: "/" and "\" become "-", every other character (including "-"
// itself) passes through unchanged. Unlike the injective character-escaping scheme this
// replaced, the result is not reversible and is not relied on for uniqueness -- two different
// inputs can sanitize to the same output (e.g. "eks-ingress/1" and the unrelated literal
// "eks-ingress-1" both become "eks-ingress-1"). Uniqueness comes entirely from
// workdirPathHash, computed from the unsanitized name, so a colliding display prefix does not
// produce a colliding path.
func sanitizeComponentNameForPath(name string) string {
	replacer := strings.NewReplacer("/", "-", `\`, "-")
	return replacer.Replace(name)
}

// workdirPathHash returns a short, deterministic hex hash suffix that disambiguates two
// different (stack, component) pairs whose sanitized display prefixes collide (see
// sanitizeComponentNameForPath). The stack and component arguments are joined with an
// explicit "\x00" separator before hashing -- never exposed in the visible directory name --
// so the hash input itself is unambiguous even though the visible "-" join is not: stack
// "dev-a" + component "b" and stack "dev" + component "a-b" hash differently despite
// producing the identical visible prefix "dev-a-b".
func workdirPathHash(stack, component string) string {
	// codeql[go/weak-sensitive-data-hashing] -- hashes a stack/component name for a workdir directory-name uniqueness suffix, never a password or credential.
	sum := sha256.Sum256([]byte(stack + "\x00" + component))
	return hex.EncodeToString(sum[:])[:workdirHashLength]
}

// containWithinBase verifies that path, once absolutized, is contained within base (equal to
// it, or nested under it, once base is also absolutized). Returns errUtils.ErrPathTraversal
// if not. On success it returns path unchanged (not the absolutized form), preserving
// BuildPath's existing return format for callers that pass a relative basePath. This is a
// best-effort, lexical-only guard -- symlinks are not resolved, matching the scope of the
// containment guards this replaces in pkg/terraform/output/config.go and
// internal/terraform_backend/terraform_backend_local.go.
//
// Uses filepath.Rel rather than an absBase+separator prefix check: when base resolves to a
// filesystem root (e.g. "/" on Unix, `C:\` on Windows), absBase already ends in the
// separator, so absBase+sep produces a doubled separator ("//", `C:\\`) that no real path
// under that root ever has -- a prefix check would then reject every legitimate path. Mirrors
// pkg/provisioner/source/source.go's isWithinBase, the same pattern already established
// elsewhere in this codebase for the same problem.
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
	rel, errRel := filepath.Rel(absBase, absPath)
	if errRel == nil && rel != ".." && !strings.HasPrefix(rel, ".."+sep) {
		return path, nil
	}
	return "", fmt.Errorf("%w: workdir path %q escapes base path %q", errUtils.ErrPathTraversal, absPath, absBase)
}
