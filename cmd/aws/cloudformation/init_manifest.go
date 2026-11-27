package cloudformation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	y "github.com/cloudposse/atmos/pkg/yaml"
)

func registerTemplate(cmd *cobra.Command, config *schema.AtmosConfiguration, fresh bool, info *schema.ConfigAndStacksInfo, src templateSource) error {
	template, names := src.path, src.names
	file, inherited, err := existingManifest(config, info.Stack)
	if err == nil && file == "" {
		file, err = initManifest(config, info.Stack)
	}
	if err != nil {
		return err
	}
	if filepath.Clean(template) == filepath.Clean(file) {
		return fmt.Errorf("%w: template and Atmos manifest must be different files: %s", errUtils.ErrInvalidStackConfig, file)
	}
	m, err := templateManifestData(config, file, info.Stack)
	if err != nil {
		return err
	}
	m.template = template
	dir, err := filepath.Rel(config.CloudFormationDirAbsolutePath, filepath.Dir(template))
	if err != nil {
		return err
	}
	if err := m.addInstances(names, inherited, filepath.ToSlash(dir), filepath.Base(template)); err != nil {
		return err
	}
	if !info.DryRun {
		if err := m.persist(config, fresh, file); err != nil {
			return err
		}
	}
	reportTemplate(cmd, m, info, names)
	return nil
}

// Resolve the logical stack through the normal inventory before choosing a path
// for a new manifest. Names derived from imports need not match any filename.
func existingManifest(config *schema.AtmosConfiguration, name string) (string, map[string]bool, error) {
	if len(config.IncludeStackAbsolutePaths) == 0 {
		return "", nil, nil
	}
	files, err := cfg.FindAllStackConfigsInPathsForStack(config, name, config.IncludeStackAbsolutePaths, config.ExcludeStackAbsolutePaths)
	if errors.Is(err, errUtils.ErrNoStackManifestsFound) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	scoped := *config
	scoped.StackConfigFilesAbsolutePaths = files.AbsolutePaths
	scoped.StackConfigFilesRelativePaths = files.RelativePaths
	inventory, err := cfnDescribeStacks(&scoped, "", nil, nil, nil, false, false, false, true, nil, nil)
	if err != nil {
		return "", nil, err
	}
	manifest, names, err := inventoryManifest(inventory, name)
	if err != nil {
		return "", nil, err
	}
	if manifest == "" {
		return "", nil, nil
	}
	file, err := cfg.SearchConfigFile(filepath.Join(config.StacksBaseAbsolutePath, manifest), config)
	if err == nil && filepath.Ext(file) != ".yaml" && filepath.Ext(file) != ".yml" {
		return "", nil, fmt.Errorf("%w: cannot edit templated manifest %s", errUtils.ErrInvalidStackConfig, file)
	}
	return file, names, err
}

func initManifest(config *schema.AtmosConfiguration, name string) (string, error) {
	candidates := manifestCandidates(config, name)
	var existing []string
	for _, file := range candidates {
		if _, err := os.Stat(file); err == nil {
			existing = append(existing, file)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	if len(existing) == 1 {
		return checkManifestName(existing[0], name, &config.Stacks)
	}
	if len(existing) > 1 || len(candidates) == 0 {
		return "", fmt.Errorf("%w: cannot select a unique manifest for %q under stacks.included_paths", errUtils.ErrInvalidStackConfig, name)
	}
	return candidates[0], nil
}

func matchesInitPath(pattern, path string) bool {
	pattern = filepath.ToSlash(pattern)
	if filepath.Ext(pattern) == "" {
		pattern += filepath.Ext(path)
	}
	match, _ := u.PathMatch(pattern, path)
	return match
}

func addInstance(data []byte, name, dir, template string) ([]byte, error) {
	key := stack.BuildComponentYqPath(cfg.CloudFormationComponentType, name, "")
	current, err := y.GetFormatted(data, key, "json")
	if err == nil {
		var entry struct {
			Metadata  struct{ Component string }
			Path      string
			StackName string `json:"stack_name"`
		}
		if json.Unmarshal([]byte(current), &entry) == nil && entry.Metadata.Component == dir && entry.Path == template && entry.StackName == name {
			return data, nil
		}
		return nil, fmt.Errorf("%w: component %q already exists with different configuration", errUtils.ErrInvalidStackConfig, name)
	}
	if !errors.Is(err, y.ErrYAMLPathNotFound) {
		return nil, err
	}
	for _, field := range []struct{ key, value string }{{"metadata.component", dir}, {"path", template}, {"stack_name", name}} {
		data, err = y.Set(data, key+"."+field.key, field.value)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func ensureStarter(config *schema.AtmosConfiguration) error {
	path := filepath.Join(config.BasePathAbsolute, "atmos.yaml")
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := filesystem.NewOSFileSystem().WriteFileAtomic(path, []byte(starterConfig), configFileMode); err != nil {
			return err
		}
	case err != nil:
		return err
	case string(existing) != starterConfig:
		return fmt.Errorf("%w: existing project configuration at %s; retry with its configuration", errUtils.ErrInvalidStackConfig, path)
	}
	return nil
}

func (m *templateManifest) persist(config *schema.AtmosConfiguration, fresh bool, file string) error {
	if fresh {
		if err := ensureStarter(config); err != nil {
			return err
		}
	}
	if !bytes.Equal(m.original, m.data) {
		if err := os.MkdirAll(filepath.Dir(file), configDirMode); err != nil {
			return err
		}
		if err := filesystem.NewOSFileSystem().WriteFileAtomic(file, m.data, m.mode); err != nil {
			return err
		}
	}
	return nil
}

func inventoryManifest(inventory map[string]any, name string) (string, map[string]bool, error) {
	target, _ := inventory[name].(map[string]any)
	types, _ := target["components"].(map[string]any)
	var manifest string
	names := map[string]bool{}
	for kind, section := range types {
		instances, _ := section.(map[string]any)
		for instance, raw := range instances {
			component, _ := raw.(map[string]any)
			source, _ := component["atmos_stack_file"].(string)
			if source == "" {
				continue
			}
			if manifest != "" && source != manifest {
				return "", nil, fmt.Errorf("%w: Atmos stack %q spans multiple manifests; cannot select one to edit", errUtils.ErrInvalidStackConfig, name)
			}
			manifest = source
			if kind == cfg.CloudFormationComponentType {
				names[instance] = true
			}
		}
	}
	return manifest, names, nil
}

func manifestCandidates(config *schema.AtmosConfiguration, name string) []string {
	var candidates []string
	for _, pattern := range config.Stacks.IncludedPaths {
		prefix, _, _ := strings.Cut(filepath.ToSlash(pattern), "*")
		candidate := filepath.Join(filepath.Dir(prefix), name+".yaml")
		if strings.HasSuffix(prefix, "/") {
			candidate = filepath.Join(prefix, name+".yaml")
		}
		for _, extension := range []string{".yaml", ".yml"} {
			relative := strings.TrimSuffix(filepath.ToSlash(candidate), ".yaml") + extension
			if !matchesInitPath(pattern, relative) {
				continue
			}
			if slices.ContainsFunc(config.Stacks.ExcludedPaths, func(ex string) bool { return matchesInitPath(ex, relative) }) {
				continue
			}
			path := filepath.Join(config.StacksBaseAbsolutePath, relative)
			if !slices.Contains(candidates, path) {
				candidates = append(candidates, path)
			}
		}
	}
	return candidates
}

func checkManifestName(file, name string, stacks *schema.Stacks) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	explicit, err := y.Get(data, "name")
	if err == nil && explicit != name {
		return "", fmt.Errorf("%w: manifest %s declares stack %q", errUtils.ErrInvalidStackConfig, file, explicit)
	}
	if err != nil && !errors.Is(err, y.ErrYAMLPathNotFound) {
		return "", err
	}
	if explicit == "" && (stacks.NameTemplate != "" || stacks.NamePattern != "") {
		return "", fmt.Errorf("%w: cannot infer stack %q from the naming convention in existing manifest %s", errUtils.ErrInvalidStackConfig, name, file)
	}
	return file, nil
}

func templateManifestData(config *schema.AtmosConfiguration, file, stackName string) (*templateManifest, error) {
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	original := data
	mode := os.FileMode(configFileMode)
	if err == nil {
		stat, err := os.Lstat(file)
		if err != nil {
			return nil, err
		}
		if !stat.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: expected a regular manifest: %s", errUtils.ErrInvalidStackConfig, file)
		}
		mode = stat.Mode().Perm()
	}
	if len(data) == 0 && (config.Stacks.NameTemplate != "" || config.Stacks.NamePattern != "") {
		data, err = y.SetWithType(data, "name", stackName, "string")
		if err != nil {
			return nil, err
		}
	}
	return &templateManifest{file: file, original: original, data: data, mode: mode}, nil
}

type templateManifest struct {
	template, file string
	original, data []byte
	mode           os.FileMode
}

func (m *templateManifest) addInstances(names []string, inherited map[string]bool, dir, filename string) error {
	for _, name := range names {
		if inherited[name] {
			if _, err := y.Get(m.data, stack.BuildComponentYqPath(cfg.CloudFormationComponentType, name, "")); errors.Is(err, y.ErrYAMLPathNotFound) {
				return fmt.Errorf("%w: component %q is already defined by an import", errUtils.ErrInvalidStackConfig, name)
			}
		}
		data, err := addInstance(m.data, name, dir, filename)
		if err != nil {
			return err
		}
		m.data = data
	}
	return nil
}

func reportTemplate(cmd *cobra.Command, m *templateManifest, info *schema.ConfigAndStacksInfo, names []string) {
	fmt.Fprintf(cmd.OutOrStdout(), "Atmos stack: %s\nTemplate: %s\nManifest: %s\n", info.Stack, y.DisplayPath(m.template), y.DisplayPath(m.file))
	if bytes.Equal(m.original, m.data) {
		fmt.Fprintln(cmd.OutOrStdout(), "Already configured.")
	}
	if info.DryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "Preview only.")
	}
	for _, name := range names {
		fmt.Fprintf(cmd.OutOrStdout(), "AWS stack: %s\n  atmos aws cfn apply %s -s %s\n", name, name, info.Stack)
	}
}

type templateSource struct {
	path  string
	names []string
}
