package vendoring

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// DefaultVendorFile is the default vendor manifest filename considered when no explicit --file
// override is given.
const DefaultVendorFile = "vendor.yaml"

// ComponentDirResolver resolves a component's on-disk directory, used to locate its component.yaml
// fallback. An interface so this is unit-testable without loading a real atmos.yaml (same DI
// pattern as GitDiffer/version.RemoteLister).
type ComponentDirResolver interface {
	ComponentDir(componentType, component string) (string, error)
}

// DefaultComponentDirResolver resolves via a real, lazily-loaded atmosConfig
// (cfg.InitCliConfig), matching `atmos vendor pull`'s BasePath resolution
// (Components.<Type>.BasePath, including per-type env var overrides).
type DefaultComponentDirResolver struct{}

// ComponentDir resolves componentType/component to an absolute directory path.
func (DefaultComponentDirResolver) ComponentDir(componentType, component string) (string, error) {
	defer perf.Track(nil, "vendoring.DefaultComponentDirResolver.ComponentDir")()

	if err := validateComponentType(componentType); err != nil {
		return "", err
	}
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return "", err
	}
	return u.GetComponentPath(&atmosConfig, componentType, "", component)
}

func validateComponentType(componentType string) error {
	switch componentType {
	case cfg.TerraformComponentType, cfg.HelmfileComponentType, cfg.PackerComponentType:
		return nil
	default:
		return fmt.Errorf("%w: %s", errUtils.ErrUnsupportedComponentType, componentType)
	}
}

// ResolveSourceParams configures ResolveComponentSource.
type ResolveSourceParams struct {
	// VendorFile is the --file override; empty means "look for ./vendor.yaml".
	VendorFile string
	// Component is the component name to resolve.
	Component string
	// ComponentType defaults to cfg.TerraformComponentType when empty.
	ComponentType string
	// Resolver defaults to DefaultComponentDirResolver{} when nil.
	Resolver ComponentDirResolver
}

// ResolvedSource is the outcome of ResolveComponentSource (or a component.yaml discovered via
// DiscoverComponentManifests).
type ResolvedSource struct {
	Source *schema.AtmosVendorSource
	// File is the physical manifest that declares Source (a vendor.yaml/import or a
	// component.yaml) — the file version updates are written back to.
	File string
	// FromComponentManifest is true when Source was resolved from a component.yaml fallback
	// rather than a vendor.yaml source.
	FromComponentManifest bool
	// ComponentType is the component type ("terraform", "helmfile", "packer") this source was
	// resolved under, set only alongside FromComponentManifest. A repo-wide sweep
	// (DiscoverAllComponentManifests) can mix types in one run, so this travels with each
	// individual source rather than being assumed from a single CLI --type flag; see
	// SourceUpdateResult.ComponentType, which carries it into an update report.
	ComponentType string
}

// ResolveComponentSource finds a component's vendor source, preferring vendor.yaml (--file
// override, else ./vendor.yaml, following imports) and falling back to
// <BasePath>/<component>/component.yaml when vendor.yaml doesn't exist, or exists but doesn't
// declare the component. Mirrors `atmos vendor pull`'s existing vendor.yaml-wins precedence,
// generalized to the per-component granularity diff/update need.
func ResolveComponentSource(params *ResolveSourceParams) (*ResolvedSource, error) {
	defer perf.Track(nil, "vendoring.ResolveComponentSource")()

	componentType := params.ComponentType
	if componentType == "" {
		componentType = cfg.TerraformComponentType
	}

	vendorFile, vendorFileOk := VendorFilePresent(params.VendorFile)
	if vendorFileOk {
		files, err := CollectManifestFiles(vendorFile)
		if err != nil {
			// Broken/explicit --file: surface as-is, don't mask with a fallback attempt.
			return nil, err
		}
		src, declaredIn, err := FindSource(files, params.Component)
		switch {
		case err == nil:
			return &ResolvedSource{Source: src, File: declaredIn}, nil
		case !errors.Is(err, errUtils.ErrVendorSourceNotFound):
			return nil, err
		}
		// vendor.yaml exists but doesn't declare this component -> fall through to component.yaml.
	}

	resolver := params.Resolver
	if resolver == nil {
		resolver = DefaultComponentDirResolver{}
	}
	componentDir, err := resolver.ComponentDir(componentType, params.Component)
	if err != nil {
		return nil, err
	}
	manifestFile, err := FindComponentManifestFile(componentDir)
	if err != nil {
		return nil, notFoundError(params.Component, vendorFile, vendorFileOk, componentDir)
	}
	compCfg, err := ReadComponentManifest(manifestFile)
	if err != nil {
		return nil, err
	}
	return &ResolvedSource{
		Source:                ComponentManifestSource(compCfg, params.Component, componentType),
		File:                  manifestFile,
		FromComponentManifest: true,
		ComponentType:         componentType,
	}, nil
}

// ListDeclaredSources returns every source declared across vendorFile (--file override, else
// ./vendor.yaml, following imports) -- for callers that filter across a whole manifest by a
// declared field (e.g. tags), rather than resolving one named component the way
// ResolveComponentSource does. The returned bool mirrors VendorFilePresent: false means no vendor
// manifest exists at all (not an error -- --tags-based selection over a component.yaml-only repo,
// like `atmos vendor pull --tags`, simply has nothing to match against).
func ListDeclaredSources(vendorFile string) ([]schema.AtmosVendorSource, bool, error) {
	defer perf.Track(nil, "vendoring.ListDeclaredSources")()

	resolved, ok := VendorFilePresent(vendorFile)
	if !ok {
		return nil, false, nil
	}

	files, err := CollectManifestFiles(resolved)
	if err != nil {
		return nil, true, err
	}

	var sources []schema.AtmosVendorSource
	for _, file := range files {
		fileSources, err := readVendorSources(file)
		if err != nil {
			return nil, true, err
		}
		sources = append(sources, fileSources...)
	}
	return sources, true, nil
}

// VendorFilePresent reports whether a vendor manifest is available: override if non-empty,
// otherwise the location configured via atmos.yaml (vendor.base_path, falling back to
// <BasePath>/vendor.yaml), matching `atmos vendor pull`'s existing resolution
// (internal/exec/vendor_utils.go:resolveVendorConfigFilePath). This is what makes --chdir and
// atmos.yaml's vendor.base_path setting take effect for update/diff/get/set, instead of only ever
// checking the process's cwd.
func VendorFilePresent(override string) (string, bool) {
	defer perf.Track(nil, "vendoring.VendorFilePresent")()

	if override != "" {
		return override, true
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		// No usable atmos.yaml/config context - fall back to a bare cwd check.
		if _, statErr := os.Stat(DefaultVendorFile); statErr == nil {
			return DefaultVendorFile, true
		}
		return "", false
	}

	if atmosConfig.Vendor.BasePath != "" {
		vendorPath := atmosConfig.Vendor.BasePath
		if !filepath.IsAbs(vendorPath) {
			vendorPath = filepath.Join(atmosConfig.BasePath, vendorPath)
		}
		if _, statErr := os.Stat(vendorPath); statErr == nil {
			return vendorPath, true
		}
		return "", false
	}

	if found, ok := u.SearchConfigFile(DefaultVendorFile); ok {
		return found, true
	}
	if found, ok := u.SearchConfigFile(filepath.Join(atmosConfig.BasePath, DefaultVendorFile)); ok {
		return found, true
	}
	return "", false
}

// notFoundError builds an error enumerating exactly what was checked when a component can't be
// resolved from either manifest shape.
func notFoundError(component, vendorFile string, vendorFileChecked bool, componentDir string) error {
	if vendorFileChecked {
		return fmt.Errorf("%w: component %q not declared in %s, and no component.yaml/component.yml found in %s",
			errUtils.ErrVendorSourceNotFound, component, vendorFile, componentDir)
	}
	return fmt.Errorf("%w: component %q: no %s found, and no component.yaml/component.yml found in %s",
		errUtils.ErrVendorSourceNotFound, component, DefaultVendorFile, componentDir)
}

// DiscoverComponentManifests finds every component.yaml/component.yml under a component type's
// base directory, at any nesting depth (e.g. <basePath>/eks/cluster/component.yaml, not just
// <basePath>/<component>/component.yaml), for the opt-in repo-wide component-manifest sweep
// ("vendor update --component-manifests"). Component identity for a nested manifest is the
// slash-joined path relative to basePath (e.g. "eks/cluster"), matching the existing convention
// that component names are already path-like. The moment a manifest is found in a directory, its
// own subtree (modules, examples, .terraform) stops being descended into — a component's internals
// are never treated as containing further components. Directories without a manifest are silently
// skipped — not every component vendors this way; a manifest that IS found but is malformed is a
// hard error. ".git"/".terraform" are never descended into, for --everything performance on large
// repos.
func DiscoverComponentManifests(basePath, componentType string) ([]*ResolvedSource, error) {
	defer perf.Track(nil, "vendoring.DiscoverComponentManifests")()

	info, err := os.Stat(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %w", errUtils.ErrReadVendorFile, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", errUtils.ErrReadVendorFile, basePath)
	}

	var sources []*ResolvedSource
	walkErr := filepath.WalkDir(basePath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path != basePath && skipComponentDiscoveryDir(entry.Name()) {
			return filepath.SkipDir
		}

		manifestFile, findErr := FindComponentManifestFile(path)
		if findErr != nil {
			if errors.Is(findErr, errUtils.ErrComponentManifestNotFound) {
				// No manifest directly in this directory - keep descending into it.
				return nil
			}
			return findErr
		}
		compCfg, readErr := ReadComponentManifest(manifestFile)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(basePath, path)
		if relErr != nil {
			return relErr
		}
		component := filepath.ToSlash(rel)
		sources = append(sources, &ResolvedSource{
			Source:                ComponentManifestSource(compCfg, component, componentType),
			File:                  manifestFile,
			FromComponentManifest: true,
			ComponentType:         componentType,
		})
		// A component's own subtree is never treated as containing further components.
		return filepath.SkipDir
	})
	if walkErr != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrReadVendorFile, walkErr)
	}
	return sources, nil
}

// skipComponentDiscoveryDir reports whether name should never be descended into while discovering
// component.yaml manifests: version control metadata and Terraform's local cache/state directory
// can both be large and never contain a real component.
func skipComponentDiscoveryDir(name string) bool {
	switch name {
	case ".git", ".terraform":
		return true
	default:
		return false
	}
}

// DiscoverAllComponentManifests sweeps every configured component type (or just componentType,
// when onlyType is true) for component.yaml manifests, used by the opt-in
// "vendor update --component-manifests" repo-wide sweep.
func DiscoverAllComponentManifests(componentType string, onlyType bool) ([]*ResolvedSource, error) {
	defer perf.Track(nil, "vendoring.DiscoverAllComponentManifests")()

	types := []string{cfg.TerraformComponentType, cfg.HelmfileComponentType, cfg.PackerComponentType}
	if onlyType {
		types = []string{componentType}
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, err
	}

	var all []*ResolvedSource
	for _, t := range types {
		// A type with no configured base_path has no dedicated components directory to sweep --
		// GetComponentBasePath would otherwise silently fall back to the atmos base_path itself
		// (the repo root), causing the sweep to re-walk the whole repository under this type's
		// label and rediscover every other type's components a second time, mislabeled. Only skip
		// this for the multi-type default sweep (len(types) > 1); an explicit single --type stays
		// as-is, matching every other single-type command's existing fallback behavior.
		if len(types) > 1 && !componentTypeBasePathConfigured(&atmosConfig, t) {
			continue
		}
		basePath, err := u.GetComponentBasePath(&atmosConfig, t)
		if err != nil {
			return nil, err
		}
		found, err := DiscoverComponentManifests(basePath, t)
		if err != nil {
			return nil, err
		}
		all = append(all, found...)
	}
	return all, nil
}

// componentTypeBasePathConfigured reports whether componentType has a base_path configured, via
// atmos.yaml or its per-type env var override, so DiscoverAllComponentManifests's multi-type sweep
// can skip types that have neither (see the call site above). Mirrors the same two sources
// getBasePathForComponentType itself resolves from (pkg/utils/component_path_utils.go), so a type
// configured only via env var is not incorrectly treated as unconfigured.
func componentTypeBasePathConfigured(atmosConfig *schema.AtmosConfiguration, componentType string) bool {
	var configBasePath, envVarName string
	switch componentType {
	case cfg.TerraformComponentType:
		configBasePath, envVarName = atmosConfig.Components.Terraform.BasePath, "ATMOS_COMPONENTS_TERRAFORM_BASE_PATH"
	case cfg.HelmfileComponentType:
		configBasePath, envVarName = atmosConfig.Components.Helmfile.BasePath, "ATMOS_COMPONENTS_HELMFILE_BASE_PATH"
	case cfg.PackerComponentType:
		configBasePath, envVarName = atmosConfig.Components.Packer.BasePath, "ATMOS_COMPONENTS_PACKER_BASE_PATH"
	default:
		return true
	}
	return configBasePath != "" || os.Getenv(envVarName) != "" //nolint:forbidigo // Mirrors getBasePathForComponentType's own env-var precedence check.
}
