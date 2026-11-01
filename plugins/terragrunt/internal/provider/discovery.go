package provider

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// Discover expands local stacks in an isolated overlay without preparing units.
func (p *Provider) Discover(ctx context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
	req, err := operation(request.GetRootDirectory(), request.GetProvider(), request.GetOperation(), request.GetInvocation())
	if err != nil {
		return nil, rpcError(err)
	}
	if err := settings(request.GetProviderConfiguration()); err != nil {
		return nil, rpcError(err)
	}
	paths, err := importPaths(request)
	if err != nil {
		return nil, rpcError(err)
	}
	req.Phase, req.IncludedPaths, req.ExcludedPaths = "inventory", paths, request.ExcludePaths
	result, err := p.evaluate(ctx, req)
	if err != nil {
		return nil, rpcError(err)
	}
	addresses := map[string]*pluginv1.ComponentAddress{}
	for _, unit := range result.Inventory {
		addresses[unit.Config.Path] = &pluginv1.ComponentAddress{Stack: unit.Stack, Component: unit.Name, ComponentType: "terraform"}
	}
	response := &pluginv1.DiscoverComponentsResponse{}
	for _, unit := range result.Inventory {
		discovered, err := discoveredUnit(req, unit, addresses)
		if err != nil {
			return nil, rpcError(err)
		}
		response.Components = append(response.Components, discovered)
	}
	return response, nil
}

func discoveredUnit(req model.Request, unit model.DiscoveredUnit, addresses map[string]*pluginv1.ComponentAddress) (*pluginv1.DiscoveredComponent, error) {
	source, err := relative(req.BasePath, unit.Config.Path)
	if err != nil {
		return nil, err
	}
	module := unit.Config.Source
	if len(unit.Config.Folders) > 0 && (module == "" || filepath.IsAbs(module)) {
		path := module
		if path == "" {
			path = unit.Config.Folders[0]
		}
		local, err := relative(req.BasePath, path)
		if err != nil {
			return nil, err
		}
		module = "./" + local
	}
	if module == "" {
		return nil, ErrUnsupported
	}
	generation := ""
	if unit.GenerationRoot != "" {
		generation, err = relative(req.BasePath, unit.GenerationRoot)
		if err != nil {
			return nil, err
		}
	}
	workspace := req.Environment["TF_WORKSPACE"]
	if workspace == "" {
		workspace = "default"
	}
	config, err := encode(map[string]any{
		"source": module, "vars": map[string]any{},
		"provision":    map[string]any{"workdir": map[string]any{"enabled": true}},
		"backend_type": "local", "backend": map[string]any{"local": map[string]any{}},
		"metadata": map[string]any{"terraform_workspace": workspace},
		"settings": map[string]any{
			"terragrunt":    map[string]any{"generation_root": generation},
			"configuration": map[string]any{"watch_excludes": []string{"**/.terragrunt-cache/**", "**/.terragrunt-stack/**", "**/.terraform/**", "**/.atmos/**"}},
		},
	})
	if err != nil {
		return nil, err
	}
	watched, err := sources(req.BasePath, &unit.Config)
	if err != nil {
		return nil, err
	}
	instance := &pluginv1.DiscoveredComponent{Address: addresses[unit.Config.Path], Provider: "terragrunt", Engine: engineName(req.Tofu), Source: source, Configuration: config, Sources: watched}
	for _, path := range unit.Config.Dependencies {
		target := addresses[path]
		if target == nil {
			return nil, ErrUnsupported
		}
		kind := pluginv1.ComponentDependency_ORDER
		if slices.Contains(unit.Config.OutputDependencies, path) {
			kind = pluginv1.ComponentDependency_OUTPUT
		}
		instance.Dependencies = append(instance.Dependencies, &pluginv1.ComponentDependency{Target: target, Kind: kind})
	}
	return instance, nil
}

// importPaths adds explicit files or directory roots to the same scoped inventory.
// No HCL evaluation occurs while determining which roots were requested.
func importPaths(request *pluginv1.DiscoverComponentsRequest) ([]string, error) {
	paths := slices.Clone(request.IncludePaths)
	for _, uri := range request.Imports {
		path, found := strings.CutPrefix(uri, "terragrunt://")
		if !found || !filepath.IsLocal(path) || strings.ContainsAny(path, "\\?#*[{") {
			return nil, ErrUnsupported
		}
		info, err := os.Stat(filepath.Join(request.RootDirectory, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			path = strings.TrimSuffix(filepath.ToSlash(path), "/") + "/**"
		} else if info.Name() != "terragrunt.hcl" && info.Name() != "terragrunt.stack.hcl" {
			return nil, ErrUnsupported
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, ErrUnsupported
	}
	return paths, nil
}
