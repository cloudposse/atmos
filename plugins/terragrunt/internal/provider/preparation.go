package provider

import (
	"context"
	"maps"
	"path/filepath"
	"slices"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func (p *Provider) componentRequest(ctx context.Context, request *pluginv1.ComponentRequest) (model.Request, error) {
	if _, err := p.Validate(ctx, request); err != nil {
		return model.Request{}, err
	}
	req, err := operation(request.RootDirectory, request.Provider, request.Operation, request.Invocation)
	if err != nil {
		return req, err
	}
	if !filepath.IsLocal(request.Source) {
		return req, ErrUnsupported
	}
	req.Path = filepath.Join(request.RootDirectory, filepath.FromSlash(request.Source))
	var config struct {
		Settings struct {
			Terragrunt struct {
				GenerationRoot string `json:"generation_root"`
			} `json:"terragrunt"`
		} `json:"settings"`
	}
	if err := decode(request.Configuration, &config); err != nil {
		return req, err
	}
	if generation := config.Settings.Terragrunt.GenerationRoot; generation != "" {
		if !filepath.IsLocal(generation) {
			return req, ErrUnsupported
		}
		req.GenerationRoot = filepath.Join(req.BasePath, filepath.FromSlash(generation))
	}
	return req, nil
}

// Prepare resolves current inputs after dependencies and returns literal files.
// Terraform execution remains owned by Atmos.
func (p *Provider) Prepare(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
	response, err := p.prepare(ctx, request)
	return response, scoped(err, "prepare", request.GetRootDirectory(), request.GetAddress())
}

func (p *Provider) prepare(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
	req, err := p.componentRequest(ctx, request)
	if err != nil {
		return nil, rpcError(err)
	}
	effects := request.GetEffects()
	if !effects.GetCommands() || !effects.GetWrites() || !effects.GetStateReads() || !effects.GetNetwork() || !effects.GetSecrets() {
		return nil, rpcError(ErrUnsupported)
	}
	if req.GenerationRoot != "" {
		generation := req
		generation.Path, generation.Phase, generation.Command = req.GenerationRoot, "discover", "generate"
		if _, err := p.evaluate(ctx, generation); err != nil {
			return nil, rpcError(err)
		}
	}
	result, err := p.evaluate(ctx, req)
	if err != nil {
		return nil, rpcError(err)
	}
	values := result.Inputs
	if values == nil {
		values = map[string]any{}
	}
	inputs, err := encode(values)
	if err != nil {
		return nil, rpcError(err)
	}
	state, err := stateIdentity(req, result)
	if err != nil {
		return nil, rpcError(err)
	}
	watched, err := sources(req.BasePath, result)
	if err != nil {
		return nil, rpcError(err)
	}
	response := &pluginv1.PrepareComponentResponse{Engine: engineName(req.Tofu), ModuleSource: result.Source, WorkingDirectory: result.WorkingDir, Inputs: inputs, State: state, Sources: watched, BackendGenerated: result.BackendGenerated, Execution: executionResponse(result.Execution)}
	for _, name := range slices.Sorted(maps.Keys(result.Generated)) {
		policy := result.GeneratedPolicies[name]
		if policy == "" {
			policy = "overwrite"
		}
		response.Files = append(response.Files, &pluginv1.ComponentGeneratedFile{Path: name, Content: []byte(result.Generated[name]), IfExists: policy})
	}
	return response, nil
}

func stateIdentity(req model.Request, result *model.Resolution) (*pluginv1.ComponentStateIdentity, error) {
	backend := result.Backend
	if backend == nil {
		backend = map[string]any{}
	}
	backendType := result.BackendType
	if backendType == "" {
		backendType = "local"
	}
	encoded, err := encode(backend)
	if err != nil {
		return nil, err
	}
	workspace := req.Environment["TF_WORKSPACE"]
	if workspace == "" {
		workspace = "default"
	}
	return &pluginv1.ComponentStateIdentity{BackendType: backendType, Backend: encoded, Workspace: workspace, WorkingDirectory: result.WorkingDir}, nil
}

// Resolve reads backend identity in an overlay, independently of dependency outputs.
func (p *Provider) Resolve(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
	response, err := p.resolveState(ctx, request)
	return response, scoped(err, "resolve state", request.GetRootDirectory(), request.GetAddress())
}

func (p *Provider) resolveState(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
	req, err := p.componentRequest(ctx, request)
	if err != nil {
		return nil, rpcError(err)
	}
	req.Phase = "backend"
	result, err := p.evaluate(ctx, req)
	if err != nil {
		return nil, rpcError(err)
	}
	state, err := stateIdentity(req, result)
	if err != nil {
		return nil, rpcError(err)
	}
	watched, err := sources(req.BasePath, result)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pluginv1.ResolveComponentStateResponse{State: state, Sources: watched}, nil
}

// executionResponse hands evaluated hooks and extra arguments to the host unchanged and in
// declaration order. The plugin never runs them.
func executionResponse(execution *model.Execution) *pluginv1.ComponentExecutionConfiguration {
	if execution == nil {
		return nil
	}
	result := &pluginv1.ComponentExecutionConfiguration{
		BeforeHooks: hooksResponse(execution.BeforeHooks),
		AfterHooks:  hooksResponse(execution.AfterHooks),
		ErrorHooks:  hooksResponse(execution.ErrorHooks),
	}
	for i := range execution.ExtraArguments {
		extra := &execution.ExtraArguments[i]
		result.ExtraArguments = append(result.ExtraArguments, &pluginv1.ComponentExtraArguments{
			Commands:         slices.Clone(extra.Commands),
			Arguments:        slices.Clone(extra.Arguments),
			RequiredVarFiles: slices.Clone(extra.RequiredVarFiles),
			OptionalVarFiles: slices.Clone(extra.OptionalVarFiles),
			Environment:      maps.Clone(extra.Environment),
		})
	}
	return result
}

func hooksResponse(hooks []model.Hook) []*pluginv1.ComponentHook {
	var result []*pluginv1.ComponentHook
	for i := range hooks {
		hook := &hooks[i]
		result = append(result, &pluginv1.ComponentHook{
			Name:             hook.Name,
			Commands:         slices.Clone(hook.Commands),
			Execute:          slices.Clone(hook.Execute),
			WorkingDirectory: hook.WorkingDirectory,
			RunOnError:       hook.RunOnError,
			SuppressStdout:   hook.SuppressStdout,
			OnErrors:         slices.Clone(hook.OnErrors),
		})
	}
	return result
}
