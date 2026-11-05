package catalog

import (
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// FromConfig loads one existing scaffold.templates entry, preserving local and remote behavior.
func FromConfig(name string, templateData interface{}, excludeTargetDir string) (templates.Configuration, error) {
	defer perf.Track(nil, "catalog.FromConfig")()

	return fromConfigAt(name, templateData, "", excludeTargetDir)
}

// fromConfigAt loads a scaffold.templates entry, resolving a relative local
// source from configDir. An empty configDir keeps current-directory resolution.
func fromConfigAt(name string, templateData interface{}, configDir, excludeTargetDir string) (templates.Configuration, error) {
	templateMap, ok := templateData.(map[string]interface{})
	if !ok {
		return templates.Configuration{}, errUtils.Build(errUtils.ErrInvalidTemplateData).
			WithExplanationf("Template `%s` has invalid structure in `atmos.yaml`", name).
			WithHint("Each template must be a map with configuration keys").
			WithExample("```yaml\nscaffold:\n  templates:\n    my-template:\n      description: My template\n      source: ./path/to/template\n      target_dir: ./output\n```").
			WithContext("template_name", name).
			WithExitCode(2).
			Err()
	}

	source, _ := templateMap["source"].(string)
	if source == "" {
		return templates.Configuration{}, errUtils.Build(errUtils.ErrInvalidTemplateData).
			WithExplanationf("Template `%s` has no `source` in `atmos.yaml`", name).
			WithHint("Set `source` to a local directory containing the template files").
			WithExample("```yaml\nscaffold:\n  templates:\n    my-template:\n      description: My template\n      source: ./scaffolds/my-template\n```").
			WithContext("template_name", name).
			WithExitCode(2).
			Err()
	}

	// Remote sources (git/https/s3) are advertised as stubs and fetched lazily
	// at generation time by the source package; local sources load eagerly so
	// `atmos scaffold list` can show their version and description.
	if !vendor.IsLocalPath(source) && !vendor.IsFileURI(source) {
		stub := templates.Configuration{Name: name, Source: source}
		stub.Description, _ = templateMap["description"].(string)
		return stub, nil
	}

	// Load the template files from the local source directory. The path is
	// resolved relative to the directory containing atmos.yaml (configDir), or
	// the current directory when configDir is empty.
	// WithExcludePath is a no-op when excludeTargetDir is "" or doesn't
	// resolve inside source.
	cfg, err := templates.LoadConfigurationFromDir(name, localSourceDir(configDir, source), templates.WithExcludePath(excludeTargetDir))
	if err != nil {
		return templates.Configuration{}, err
	}
	// Keep the source as declared in atmos.yaml; the files are already loaded.
	cfg.Source = source

	// An explicit description in atmos.yaml overrides the template's own metadata.
	desc, _ := templateMap["description"].(string)
	if desc != "" {
		cfg.Description = desc
	}

	// Extract target_dir if provided.
	if targetDir, ok := templateMap["target_dir"].(string); ok {
		cfg.TargetDir = targetDir
	}

	return *cfg, nil
}

// localSourceDir resolves a relative local template source from configDir.
func localSourceDir(configDir, source string) string {
	if configDir == "" || !vendor.IsLocalPath(source) || filepath.IsAbs(source) {
		return source
	}
	return filepath.Join(configDir, source)
}
