package create

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/datafetcher"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// scaffoldConfigDir returns the directory of the loaded atmos.yaml, which owns
// scaffold.templates. It falls back to the project root when the configuration
// came from several files or the directory has no atmos.yaml.
func scaffoldConfigDir(config *schema.AtmosConfiguration, root string) string {
	dir := config.CliConfigPath
	if dir == "" || strings.Contains(dir, ";") {
		return root
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "atmos.yaml")); err != nil {
		return root
	}
	return dir
}

func resolveManifest(config *schema.AtmosConfiguration, file string) (string, string, error) {
	if file == "" {
		return "", "", fmt.Errorf("%w: --file is required", errUtils.ErrInvalidFlag)
	}
	ext := filepath.Ext(file)
	if ext != ".yaml" && ext != ".yml" {
		return "", "", fmt.Errorf("%w: --file must end in .yaml or .yml", errUtils.ErrInvalidFlag)
	}
	root, err := filepath.Abs(config.BasePathAbsolute)
	if err != nil {
		return "", "", err
	}
	// --file is project-relative, so relative paths resolve from the base path, not the CWD.
	absolute := file
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(root, absolute)
	}
	absolute = filepath.Clean(absolute)
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("%w: stack manifest must be inside project base path %s", errUtils.ErrInvalidFlag, root)
	}
	if err := discoverable(config, absolute); err != nil {
		return "", "", err
	}
	return root, relative, nil
}

func discoverable(config *schema.AtmosConfiguration, file string) error {
	base := config.StacksBaseAbsolutePath
	if base == "" {
		base = filepath.Join(config.BasePathAbsolute, config.Stacks.BasePath)
	}
	relative, err := filepath.Rel(base, file)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: manifest must be under stacks.base_path", errUtils.ErrInvalidConfig)
	}
	included, err := matchesPatterns(config.Stacks.IncludedPaths, relative, true)
	if err != nil {
		return err
	}
	if !included {
		return fmt.Errorf("%w: %s does not match stacks.included_paths", errUtils.ErrInvalidConfig, relative)
	}
	excluded, err := matchesPatterns(config.Stacks.ExcludedPaths, relative, false)
	if err != nil {
		return err
	}
	if excluded {
		return fmt.Errorf("%w: %s matches stacks.excluded_paths", errUtils.ErrInvalidConfig, relative)
	}
	return nil
}

func matchesPatterns(patterns []string, file string, expand bool) (bool, error) {
	for _, pattern := range patterns {
		if expand && filepath.Ext(pattern) == "" {
			pattern += filepath.Ext(file)
		}
		matches, err := u.PathMatch(filepath.ToSlash(pattern), filepath.ToSlash(file))
		if err != nil {
			return false, err
		}
		if matches {
			return true, nil
		}
	}
	return false, nil
}

func validateManifest(config *schema.AtmosConfiguration, files []engine.File, manifest string) error {
	for _, file := range files {
		if filepath.Clean(file.Path) != filepath.Clean(manifest) {
			continue
		}
		var document map[string]any
		decoder := yaml.NewDecoder(strings.NewReader(file.Content))
		if err := decoder.Decode(&document); err != nil {
			return engine.ExplainUnmarkedTemplate(manifest, file.Content, file.Delimiters, fmt.Errorf("%w: %w", errUtils.ErrInvalidConfig, err))
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: a stack manifest must contain one YAML document", errUtils.ErrInvalidConfig)
		}
		schemaBytes, err := datafetcher.NewDataFetcher(config).GetData("atmos://schema/atmos/manifest/1.0")
		if err != nil {
			return err
		}
		result, err := gojsonschema.Validate(gojsonschema.NewBytesLoader(schemaBytes), gojsonschema.NewGoLoader(document))
		if err != nil {
			return err
		}
		if !result.Valid() {
			return fmt.Errorf("%w: generated stack %s: %v", errUtils.ErrInvalidConfig, manifest, result.Errors())
		}
		return nil
	}
	return fmt.Errorf("%w: scaffold did not generate stack manifest %s", errUtils.ErrInvalidConfig, manifest)
}
