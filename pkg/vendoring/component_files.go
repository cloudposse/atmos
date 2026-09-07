package vendoring

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	goyaml "go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// componentManifestBaseNames are the supported component vendoring manifest filenames, in lookup
// preference order.
var componentManifestBaseNames = []string{"component.yaml", "component.yml"}

// componentManifestKind is the required "kind" field in a component vendoring manifest.
const componentManifestKind = "ComponentVendorConfig"

// FindComponentManifestFile locates a component vendoring manifest (component.yaml or
// component.yml) directly inside componentDir. Unlike vendor.yaml, component.yaml has no
// imports, so no recursion is needed.
func FindComponentManifestFile(componentDir string) (string, error) {
	defer perf.Track(nil, "vendoring.FindComponentManifestFile")()

	for _, name := range componentManifestBaseNames {
		candidate := filepath.Join(componentDir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: %s", errUtils.ErrComponentManifestNotFound, componentDir)
}

// ReadComponentManifest reads and decodes a component vendoring manifest, validating its "kind".
func ReadComponentManifest(file string) (*schema.VendorComponentConfig, error) {
	defer perf.Track(nil, "vendoring.ReadComponentManifest")()

	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrReadVendorFile, err)
	}

	var cfg schema.VendorComponentConfig
	if err := goyaml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrParseVendorFile, file, err)
	}
	if cfg.Kind != componentManifestKind {
		return nil, fmt.Errorf("%w: %q in %s", errUtils.ErrInvalidComponentManifestKind, cfg.Kind, file)
	}
	return &cfg, nil
}

// ResolveComponentPath resolves the absolute base directory for component of componentType,
// joining atmosConfig's global BasePath with the component type's own configured BasePath and the
// component name -- the layout every component.yaml source has always assumed
// ("components/terraform/<name>", etc.). The returned path is always absolute (though not
// symlink-resolved -- see the comment above its own EvalSymlinks containment re-check) so the
// vendor lock's target-containment check (pkg/vendoring/lockfile) can compare it against BasePath;
// a relative atmosConfig.BasePath would otherwise make it relative to CWD instead, which the lock
// validation cannot reliably relate back to BasePath. Returns an error when componentType isn't
// one of terraform/helmfile/packer, when component escapes the component-type base directory (e.g.
// an absolute path, a "../"-laden value, or a symlink that resolves outside it -- component can
// originate from stack-declared metadata.component, not just operator-typed CLI flags, so it isn't
// implicitly trusted), or when the resolved directory does not exist.
func ResolveComponentPath(atmosConfig *schema.AtmosConfiguration, component, componentType string) (string, error) {
	defer perf.Track(atmosConfig, "vendoring.ResolveComponentPath")()

	var componentBasePath string
	switch componentType {
	case cfg.TerraformComponentType:
		componentBasePath = atmosConfig.Components.Terraform.BasePath
	case cfg.HelmfileComponentType:
		componentBasePath = atmosConfig.Components.Helmfile.BasePath
	case cfg.PackerComponentType:
		componentBasePath = atmosConfig.Components.Packer.BasePath
	default:
		return "", fmt.Errorf("%s: %w", componentType, errUtils.ErrUnsupportedComponentType)
	}

	componentTypeBase, err := filepath.Abs(filepath.Join(atmosConfig.BasePath, componentBasePath))
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrResolveComponentPath, err)
	}

	componentPath, err := filepath.Abs(filepath.Join(componentTypeBase, component))
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrResolveComponentPath, err)
	}

	if err := requireContainedPath(componentTypeBase, componentPath, component); err != nil {
		return "", err
	}

	dirExists, err := u.IsDirectory(componentPath)
	if err != nil {
		// IsDirectory surfaces a non-nil error for a missing path, so a plain return here
		// would make the ErrComponentDirNotFound sentinel unreachable for the common
		// "directory does not exist" case.
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", errUtils.ErrComponentDirNotFound, componentPath)
		}
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrResolveComponentPath, err)
	}
	if !dirExists {
		return "", fmt.Errorf("%w: %s", errUtils.ErrComponentDirNotFound, componentPath)
	}

	// requireContainedPath above only checked the LEXICAL path. A component directory that is
	// lexically inside componentTypeBase can still be (or contain, via an intermediate segment) a
	// symlink that actually resolves outside componentTypeBase; u.IsDirectory above and the
	// manifest reads callers perform afterwards (FindComponentManifestFile/ReadComponentManifest)
	// follow symlinks, so re-check containment against the symlink-resolved paths now that we know
	// componentPath exists. The LEXICAL componentPath is still what's returned (not the resolved
	// one): the vendor lock's target-containment check (pkg/vendoring/lockfile) compares this
	// return value against the unresolved config.BasePath, so substituting a resolved path here
	// would make that comparison spuriously fail on any platform where the project root itself
	// sits behind a symlink (e.g. macOS's /var -> /private/var); once containment against the
	// resolved paths is confirmed, following the same symlink via the lexical path is safe.
	resolvedBase, err := filepath.EvalSymlinks(componentTypeBase)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrResolveComponentPath, err)
	}
	resolvedPath, err := filepath.EvalSymlinks(componentPath)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrResolveComponentPath, err)
	}
	if err := requireContainedPath(resolvedBase, resolvedPath, component); err != nil {
		return "", err
	}

	return componentPath, nil
}

// requireContainedPath rejects a component name that resolves outside componentTypeBase (an
// absolute component, or one laden with "../" segments), so a value sourced from stack-declared
// metadata.component can't make ResolveComponentPath escape the component-type base directory.
func requireContainedPath(componentTypeBase, componentPath, component string) error {
	if filepath.IsAbs(component) {
		return fmt.Errorf("%w: component %q must not be an absolute path", errUtils.ErrPathTraversal, component)
	}

	rel, err := filepath.Rel(componentTypeBase, componentPath)
	if err != nil {
		return fmt.Errorf("%w: component %q: %w", errUtils.ErrPathTraversal, component, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: component %q resolves outside %s", errUtils.ErrPathTraversal, component, componentTypeBase)
	}

	return nil
}

// ComponentManifestSource converts a decoded component.yaml into the same schema.AtmosVendorSource
// shape vendor.yaml sources use, so vendoring.Diff and vendoring.Update's checkAndUpdateSource/
// isNewer/resolveLatest consume it unmodified. Targets is synthesized as a single
// "components/<type>/<component>" entry solely so --type filtering (sourceTargetsType) behaves
// identically regardless of which manifest shape the source came from.
func ComponentManifestSource(cfg *schema.VendorComponentConfig, component, componentType string) *schema.AtmosVendorSource {
	defer perf.Track(nil, "vendoring.ComponentManifestSource")()

	return &schema.AtmosVendorSource{
		Component:   component,
		Source:      cfg.Spec.Source.Uri,
		Version:     cfg.Spec.Source.Version,
		Targets:     schema.AtmosVendorTargets{{Path: filepath.Join("components", componentType, component)}},
		Constraints: cfg.Spec.Source.Constraints,
	}
}

// ComponentManifestVersionPath is the fixed dot-path for a component.yaml's pinned version
// (singular spec.source, unlike vendor.yaml's spec.sources[] array).
const ComponentManifestVersionPath = "spec.source.version"

// SetComponentManifestVersion sets spec.source.version in a component.yaml file, preserving
// comments, anchors, and templates, via the same generic format-preserving primitive vendor.yaml
// editing uses.
func SetComponentManifestVersion(file, ver string) error {
	defer perf.Track(nil, "vendoring.SetComponentManifestVersion")()

	_, err := atmosyaml.SetFileWithType(file, ComponentManifestVersionPath, ver, atmosyaml.TypeString)
	return err
}
