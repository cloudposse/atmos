package provider

import (
	"context"
	"slices"

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
	if len(request.IncludePaths) == 0 || len(request.Imports) != 0 {
		return nil, rpcError(ErrUnsupported)
	}
	req.Phase, req.IncludedPaths, req.ExcludedPaths = "inventory", request.IncludePaths, request.ExcludePaths
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
	if len(unit.Config.Folders) == 0 {
		return nil, ErrUnsupported
	}
	module, err := relative(req.BasePath, unit.Config.Folders[0])
	if err != nil {
		return nil, err
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
		"source": "./" + module, "vars": map[string]any{},
		"provision":    map[string]any{"workdir": map[string]any{"enabled": true}},
		"backend_type": "local", "backend": map[string]any{"local": map[string]any{}},
		"metadata": map[string]any{"terraform_workspace": workspace},
		"settings": map[string]any{"terragrunt": map[string]any{"generation_root": generation}},
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
