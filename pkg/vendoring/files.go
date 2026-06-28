package vendoring

import (
	"fmt"
	"os"
	"path/filepath"

	goyaml "go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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
