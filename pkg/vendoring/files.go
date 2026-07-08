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

// CollectManifestFiles returns the ordered list of physical vendor manifest
// files reachable from vendorFile: the file itself followed by the files it
// imports (resolved relative to each importing file's directory, recursively and
// de-duplicated). Editing operates on these concrete files so changes land in
// the file that declares each source.
func CollectManifestFiles(vendorFile string) ([]string, error) {
	defer perf.Track(nil, "vendoring.CollectManifestFiles")()

	seen := map[string]bool{}
	var ordered []string
	if err := collectManifestFiles(vendorFile, seen, &ordered); err != nil {
		return nil, err
	}
	return ordered, nil
}

func collectManifestFiles(file string, seen map[string]bool, ordered *[]string) error {
	abs, err := filepath.Abs(file)
	if err != nil {
		abs = file
	}
	if seen[abs] {
		return nil
	}
	seen[abs] = true
	*ordered = append(*ordered, file)

	cfg, err := decodeVendorManifest(file)
	if err != nil {
		return err
	}
	dir := filepath.Dir(file)
	for _, imp := range cfg.Spec.Imports {
		importPath := imp
		if !filepath.IsAbs(importPath) {
			importPath = filepath.Join(dir, imp)
		}
		if err := collectManifestFiles(importPath, seen, ordered); err != nil {
			return err
		}
	}
	return nil
}

// FindSource scans the given manifest files for a source declaring component and
// returns it along with the file that declares it.
func FindSource(files []string, component string) (*schema.AtmosVendorSource, string, error) {
	defer perf.Track(nil, "vendoring.FindSource")()

	for _, file := range files {
		sources, err := readVendorSources(file)
		if err != nil {
			return nil, "", err
		}
		for i := range sources {
			if sources[i].Component == component {
				return &sources[i], file, nil
			}
		}
	}
	return nil, "", fmt.Errorf("%w: component %q", errUtils.ErrVendorSourceNotFound, component)
}

// minimalVendorConfig captures only the fields update/diff need from a vendor
// manifest.
type minimalVendorConfig struct {
	Spec struct {
		Imports []string              `yaml:"imports"`
		Sources []minimalVendorSource `yaml:"sources"`
	} `yaml:"spec"`
}

type minimalVendorSource struct {
	Component   string                    `yaml:"component"`
	Source      string                    `yaml:"source"`
	Version     string                    `yaml:"version"`
	Targets     schema.AtmosVendorTargets `yaml:"targets"`
	Tags        []string                  `yaml:"tags"`
	Constraints *schema.VendorConstraints `yaml:"constraints"`
}

// decodeVendorManifest plain-decodes a vendor manifest into the minimal shape
// used by update/diff.
func decodeVendorManifest(file string) (*minimalVendorConfig, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrReadVendorFile, err)
	}
	var cfg minimalVendorConfig
	if err := goyaml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrParseVendorFile, file, err)
	}
	return &cfg, nil
}

// ComponentVersionPath resolves the dot-notation path addressing a component's
// pinned version in a vendor manifest file (e.g. "spec.sources[2].version").
// The source is matched by component name, so manifest ordering does not
// matter; the returned path reflects the component's current array index.
// Returns an error wrapping atmosyaml.ErrYAMLPathNotFound if the component is
// not declared in the manifest.
func ComponentVersionPath(vendorFile, component string) (string, error) {
	defer perf.Track(nil, "vendoring.ComponentVersionPath")()

	// Surface a read failure the same way atmosyaml.GetFile/QueryFile do
	// (wrapping atmosyaml.ErrReadFile) so callers see a consistent sentinel
	// regardless of which underlying primitive resolved the file.
	if _, err := os.ReadFile(vendorFile); err != nil {
		return "", fmt.Errorf("%w: %w", atmosyaml.ErrReadFile, err)
	}

	sources, err := readVendorSources(vendorFile)
	if err != nil {
		return "", err
	}
	for i := range sources {
		if sources[i].Component == component {
			return fmt.Sprintf("spec.sources[%d].version", i), nil
		}
	}
	return "", fmt.Errorf("%w: component %q not found in %s", atmosyaml.ErrYAMLPathNotFound, component, vendorFile)
}

// SetComponentVersion sets the version for a component in a vendor manifest
// file, preserving comments/anchors/formatting. The edit targets the matching
// source by component name (not by index), so reordering the manifest is
// safe. Returns an error wrapping atmosyaml.ErrYAMLPathNotFound if the
// component is not declared in the manifest.
func SetComponentVersion(vendorFile, component, version string) error {
	defer perf.Track(nil, "vendoring.SetComponentVersion")()

	path, err := ComponentVersionPath(vendorFile, component)
	if err != nil {
		return err
	}
	return atmosyaml.SetFileWithType(vendorFile, path, version, atmosyaml.TypeString)
}

// readVendorSources returns the sources declared directly in a manifest file
// (not merged across imports), as schema.AtmosVendorSource values.
func readVendorSources(file string) ([]schema.AtmosVendorSource, error) {
	cfg, err := decodeVendorManifest(file)
	if err != nil {
		return nil, err
	}
	sources := make([]schema.AtmosVendorSource, 0, len(cfg.Spec.Sources))
	for _, s := range cfg.Spec.Sources {
		sources = append(sources, schema.AtmosVendorSource{
			Component:   s.Component,
			Source:      s.Source,
			Version:     s.Version,
			Targets:     s.Targets,
			Tags:        s.Tags,
			Constraints: s.Constraints,
		})
	}
	return sources, nil
}
