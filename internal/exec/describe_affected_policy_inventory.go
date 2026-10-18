package exec

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Collect static, merged configuration under the active discovery configuration.
// Templates and YAML functions remain literal: evaluating BASE must not execute
// commands or fetch live values. Filters and display degradation never apply.
func collectAffectedPolicyInventory(config *schema.AtmosConfiguration, root, checkout string) (policy.InventorySnapshot, error) {
	copyConfig, err := affectedPolicyConfig(config, root, checkout)
	if err != nil {
		return policy.InventorySnapshot{}, err
	}
	ClearBaseComponentConfigCache()
	raw, _, _, err := FindStacksMap(copyConfig, false)
	if err != nil {
		return policy.InventorySnapshot{}, err
	}
	expected, err := affectedPolicyInstanceCount(raw)
	if err != nil {
		return policy.InventorySnapshot{}, err
	}
	resolved, err := ExecuteDescribeStacksWithOptions(copyConfig, "", nil, nil, nil,
		false, false, false, true, nil, nil, true, DescribeStacksErrorOptions{})
	if err != nil {
		return policy.InventorySnapshot{}, err
	}
	actual, err := affectedPolicyInstanceCount(resolved)
	if err != nil {
		return policy.InventorySnapshot{}, err
	}
	if actual != expected {
		return policy.InventorySnapshot{}, fmt.Errorf("%w: instance identity collisions or omitted instances", errUtils.ErrPolicyInputUnavailable)
	}
	return affectedPolicySnapshot(resolved)
}

func affectedPolicyConfig(config *schema.AtmosConfiguration, root, checkout string) (*schema.AtmosConfiguration, error) {
	copyConfig := *config
	paths := []*string{
		&copyConfig.BasePathAbsolute, &copyConfig.StacksBaseAbsolutePath,
		&copyConfig.TerraformDirAbsolutePath, &copyConfig.HelmfileDirAbsolutePath,
		&copyConfig.PackerDirAbsolutePath, &copyConfig.AnsibleDirAbsolutePath,
		&copyConfig.HelmDirAbsolutePath, &copyConfig.KubernetesDirAbsolutePath,
		&copyConfig.ContainerDirAbsolutePath,
	}
	for _, path := range paths {
		if *path == "" {
			continue
		}
		rebased, err := rebaseOnePathOntoWorktree(root, *path, checkout)
		if err != nil {
			return nil, err
		}
		*path = rebased
	}
	copyConfig.BasePath = copyConfig.BasePathAbsolute
	includeEmpty := true
	copyConfig.Describe.Settings.IncludeEmpty = &includeEmpty
	include, err := u.JoinPaths(copyConfig.StacksBaseAbsolutePath, copyConfig.Stacks.IncludedPaths)
	if err != nil {
		return nil, err
	}
	exclude, err := u.JoinPaths(copyConfig.StacksBaseAbsolutePath, copyConfig.Stacks.ExcludedPaths)
	if err != nil {
		return nil, err
	}
	pathsFound, _, _, err := cfg.FindAllStackConfigsInPathsForStack(copyConfig, "", include, exclude)
	if err != nil && !errors.Is(err, errUtils.ErrNoStackManifestsFound) {
		return nil, err
	}
	// A successful scan with no manifests is a known empty inventory, not a
	// missing-import fallback. Import and parsing failures remain fatal.
	copyConfig.StackConfigFilesAbsolutePaths = pathsFound
	return &copyConfig, nil
}

func affectedPolicyInstanceCount(stacks map[string]any) (int, error) {
	count := 0
	for _, value := range stacks {
		stack, ok := value.(map[string]any)
		if !ok {
			return 0, fmt.Errorf("%w: malformed stack", errUtils.ErrPolicyInputInvalid)
		}
		components, ok := stack["components"].(map[string]any)
		if !ok && stack["components"] != nil {
			return 0, fmt.Errorf("%w: malformed components", errUtils.ErrPolicyInputInvalid)
		}
		for kind, value := range components {
			if !slices.Contains([]string{"terraform", "helmfile", "packer", "ansible", "helm", "kubernetes", "container", "emulator"}, kind) {
				return 0, fmt.Errorf("%w: unsupported affected inventory component type %q", errUtils.ErrPolicyInputUnavailable, kind)
			}
			instances, ok := value.(map[string]any)
			if !ok {
				return 0, fmt.Errorf("%w: malformed instances", errUtils.ErrPolicyInputInvalid)
			}
			count += len(instances)
		}
	}
	return count, nil
}

func affectedPolicySnapshot(stacks map[string]any) (policy.InventorySnapshot, error) {
	snapshot := policy.InventorySnapshot{Stacks: map[string]map[string]map[string]json.RawMessage{}}
	for name, value := range stacks {
		data, err := json.Marshal(value)
		if err != nil {
			return snapshot, fmt.Errorf("%w: encode stack", errUtils.ErrPolicyInputInvalid)
		}
		var stack struct {
			Components map[string]map[string]json.RawMessage `json:"components"`
		}
		if err := json.Unmarshal(data, &stack); err != nil {
			return snapshot, fmt.Errorf("%w: decode stack", errUtils.ErrPolicyInputInvalid)
		}
		if stack.Components == nil {
			stack.Components = map[string]map[string]json.RawMessage{}
		}
		snapshot.Stacks[name] = stack.Components
	}
	return snapshot, nil
}

func affectedPolicySourceChanges(config *schema.AtmosConfiguration, root string, before, after policy.InventorySnapshot, files []policy.FileChange) ([]policy.ObjectIdentity, error) {
	paths := map[string]string{
		"terraform": config.TerraformDirAbsolutePath, "helmfile": config.HelmfileDirAbsolutePath,
		"packer": config.PackerDirAbsolutePath, "ansible": config.AnsibleDirAbsolutePath,
		"helm": config.HelmDirAbsolutePath, "kubernetes": config.KubernetesDirAbsolutePath, "container": config.ContainerDirAbsolutePath,
	}
	identities := []policy.ObjectIdentity{}
	for _, snapshot := range []policy.InventorySnapshot{before, after} {
		for stack, kinds := range snapshot.Stacks {
			for kind, instances := range kinds {
				for name, data := range instances {
					changed, err := affectedPolicyComponentFiles(root, paths[kind], name, data, files)
					if err != nil {
						return nil, err
					}
					if changed {
						identities = append(identities, policy.ObjectIdentity{Name: name, Stack: stack, ComponentType: kind})
					}
				}
			}
		}
	}
	return identities, nil
}

func affectedPolicyComponentFiles(root, directory, name string, data json.RawMessage, files []policy.FileChange) (bool, error) {
	if directory == "" {
		return false, nil
	}
	var instance struct {
		Component string `json:"component"`
	}
	if err := json.Unmarshal(data, &instance); err != nil {
		return false, fmt.Errorf("%w: component source path", errUtils.ErrPolicyInputInvalid)
	}
	if instance.Component != "" {
		name = instance.Component
	}
	path, err := rebaseOnePathOntoWorktree(root, filepath.Join(directory, name), root)
	if err != nil {
		return false, err
	}
	relative, err := filepath.Rel(evalSymlinksBestEffort(root), evalSymlinksBestEffort(path))
	if err != nil {
		return false, err
	}
	prefix := filepath.ToSlash(relative) + "/"
	for _, file := range files {
		if strings.HasPrefix(file.Path, prefix) {
			return true, nil
		}
	}
	return false, nil
}
