package vendoring

import (
	"errors"
	"fmt"
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

// DiscoverComponentManifests finds every component.yaml/component.yml declared directly under a
// component type's base directory (one level deep: <basePath>/<component>/component.yaml), for
// the opt-in repo-wide component-manifest sweep ("vendor update --component-manifests").
// Directories without a manifest are silently skipped — not every component vendors this way; a
// manifest that IS found but is malformed is a hard error.
func DiscoverComponentManifests(basePath, componentType string) ([]*ResolvedSource, error) {
	defer perf.Track(nil, "vendoring.DiscoverComponentManifests")()

	entries, err := os.ReadDir(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %w", errUtils.ErrReadVendorFile, err)
	}

	var sources []*ResolvedSource
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		componentDir := filepath.Join(basePath, entry.Name())
		manifestFile, err := FindComponentManifestFile(componentDir)
		if err != nil {
			continue
		}
		compCfg, err := ReadComponentManifest(manifestFile)
		if err != nil {
			return nil, err
		}
		sources = append(sources, &ResolvedSource{
			Source:                ComponentManifestSource(compCfg, entry.Name(), componentType),
			File:                  manifestFile,
			FromComponentManifest: true,
			ComponentType:         componentType,
		})
	}
	return sources, nil
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
