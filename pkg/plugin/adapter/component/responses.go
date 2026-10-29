package component

import (
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func validAddress(address *pluginv1.ComponentAddress) bool {
	return address != nil && address.Stack != "" && address.Component != "" && address.ComponentType != ""
}

func validState(state *pluginv1.ComponentStateIdentity) bool {
	return state != nil && state.BackendType != "" && state.Workspace != "" && filepath.IsAbs(state.WorkingDirectory)
}

func validSources(sources []*pluginv1.ComponentSource) bool {
	for _, source := range sources {
		if source == nil || !filepath.IsLocal(source.Path) {
			return false
		}
		switch source.Kind {
		case pluginv1.ComponentSource_FILE, pluginv1.ComponentSource_DIRECTORY, pluginv1.ComponentSource_GLOB:
		default:
			return false
		}
	}
	return true
}

func validDependencies(dependencies []*pluginv1.ComponentDependency) bool {
	for _, dependency := range dependencies {
		if dependency == nil || !validAddress(dependency.Target) {
			return false
		}
		if dependency.Kind != pluginv1.ComponentDependency_ORDER && dependency.Kind != pluginv1.ComponentDependency_OUTPUT {
			return false
		}
	}
	return true
}

func (p *Provider) checkDiscovery(result *pluginv1.DiscoverComponentsResponse) error {
	if result == nil {
		return errUtils.ErrPluginProtocol
	}
	if len(result.Diagnostics) != 0 {
		return errUtils.ErrPluginInvalidConfig
	}
	seen := make(map[[3]string]bool)
	for _, item := range result.Components {
		if !p.validDiscoveredComponent(item) {
			return errUtils.ErrPluginProtocol
		}
		key := [3]string{item.Address.Stack, item.Address.ComponentType, item.Address.Component}
		if seen[key] {
			return errUtils.ErrDuplicatePluginExtension
		}
		seen[key] = true
	}
	return nil
}

func (p *Provider) validDiscoveredComponent(item *pluginv1.DiscoveredComponent) bool {
	if item == nil || !validAddress(item.Address) || item.Address.ComponentType != p.componentType || item.Provider != p.name || item.Engine == "" {
		return false
	}
	return filepath.IsLocal(item.Source) && validSources(item.Sources) && validDependencies(item.Dependencies)
}

func checkPreparation(result *pluginv1.PrepareComponentResponse) error {
	if result == nil {
		return errUtils.ErrPluginProtocol
	}
	if len(result.Diagnostics) != 0 {
		return errUtils.ErrPluginInvalidConfig
	}
	if result.Engine == "" || result.ModuleSource == "" || !filepath.IsAbs(result.WorkingDirectory) || !validState(result.State) || !validSources(result.Sources) || !validDependencies(result.Dependencies) {
		return errUtils.ErrPluginProtocol
	}
	return checkGeneratedFiles(result.Files)
}

func checkGeneratedFiles(files []*pluginv1.ComponentGeneratedFile) error {
	seen := make(map[string]bool)
	for _, file := range files {
		if file == nil || !filepath.IsLocal(file.Path) || seen[filepath.Clean(file.Path)] {
			return errUtils.ErrPluginProtocol
		}
		seen[filepath.Clean(file.Path)] = true
		switch file.IfExists {
		case "error", "skip", "overwrite", "overwrite_terragrunt":
		default:
			return errUtils.ErrPluginProtocol
		}
	}
	return nil
}
