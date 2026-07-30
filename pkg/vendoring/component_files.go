package vendoring

import (
	"fmt"
	"os"
	"path/filepath"

	goyaml "go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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
