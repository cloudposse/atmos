package configuration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Prepare reevaluates only the selected instance, after the scheduler's dependencies.
func (s *Session) Prepare(ctx context.Context, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
	defer perf.Track(nil, "configurationadapter.Prepare")()

	if _, owned := info.ComponentSettingsSection["configuration"]; !owned {
		return nil, nil
	}
	if info.UseTerraformPlan || info.PlanFile != "" || info.StoredPlanFile != "" {
		return nil, fmt.Errorf("%w: plugin saved-plan binding is not implemented", errUtils.ErrPluginUnsupportedExtension)
	}
	selected, err := s.componentRequest(ctx, info)
	if err != nil {
		return nil, err
	}
	selected.request.Effects = &pluginv1.ComponentEffects{Commands: true, Network: true, Secrets: true, Writes: true, StateReads: true}
	result, err := selected.provider.Prepare(ctx, selected.request)
	if err != nil {
		return nil, err
	}
	if result.Engine != selected.instance.Engine {
		return nil, errUtils.ErrPluginProtocol
	}
	return preparedConfiguration(result)
}

// selectedComponent bundles the request, owning provider, and discovered instance for one component.
type selectedComponent struct {
	request  *pluginv1.ComponentRequest
	provider Provider
	instance *pluginv1.DiscoveredComponent
}

// componentRequest shares coordinates and invocation environment, but grants no effects.
func (s *Session) componentRequest(ctx context.Context, info *schema.ConfigAndStacksInfo) (*selectedComponent, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, err
	}
	instance := s.instances[[3]string{info.Stack, "terraform", info.ComponentFromArg}]
	if instance == nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	provider, ok := s.registry[instance.Provider]
	if !ok {
		return nil, errUtils.ErrPluginUnsupportedExtension
	}
	env := preparationEnvironment(info)
	env["ATMOS_TERRAFORM_BINARY"] = info.Command
	env["TF_WORKSPACE"] = info.TerraformWorkspace
	request := &pluginv1.ComponentRequest{
		Provider: instance.Provider, Address: instance.Address, Source: instance.Source, RootDirectory: s.root,
		Configuration: instance.Configuration, ProviderConfiguration: s.providerSettings(instance.Provider),
		Invocation: &pluginv1.Invocation{Environment: env},
		Operation:  &pluginv1.ComponentOperation{Command: info.SubCommand, Mode: "native", TerraformArguments: slices.Clone(info.AdditionalArgsAndFlags)},
	}
	return &selectedComponent{request: request, provider: provider, instance: instance}, nil
}

func preparedConfiguration(result *pluginv1.PrepareComponentResponse) (*schema.PreparedComponentConfiguration, error) {
	inputs, err := adapter.DecodeObject(result.Inputs)
	if err != nil {
		return nil, err
	}
	backend, err := adapter.DecodeObject(result.State.Backend)
	if err != nil {
		return nil, err
	}
	if result.State.BackendType == "local" {
		resolveLocalBackend(backend, result.State.WorkingDirectory)
	}
	prepared := &schema.PreparedComponentConfiguration{Engine: result.Engine, Source: result.ModuleSource, Inputs: inputs, BackendType: result.State.BackendType, Backend: backend, Workspace: result.State.Workspace, BackendGenerated: result.BackendGenerated}
	execution, err := preparedExecution(result.Execution)
	if err != nil {
		return nil, err
	}
	prepared.Execution = execution
	for _, file := range result.Files {
		prepared.Files = append(prepared.Files, schema.ComponentConfigurationFile{Path: file.Path, Content: file.Content, IfExists: file.IfExists, Sensitive: file.Sensitive})
	}
	for _, source := range result.Sources {
		prepared.Sources = append(prepared.Sources, schema.ComponentConfigurationSource{Path: source.Path, Directory: source.Kind == pluginv1.ComponentSource_DIRECTORY})
	}

	return prepared, nil
}

func resolveLocalBackend(backend map[string]any, root string) {
	for key, fallback := range map[string]string{"path": "terraform.tfstate", "workspace_dir": "terraform.tfstate.d"} {
		path, _ := backend[key].(string)
		if path == "" {
			path = fallback
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		backend[key] = path
	}
}

// The sanitized base is authoritative, including an explicitly empty slice.
// Never reintroduce ambient credentials removed by the selected identity.
func preparationEnvironment(info *schema.ConfigAndStacksInfo) map[string]string {
	base := info.SanitizedEnv
	if base == nil {
		base = os.Environ()
	}
	env := make(map[string]string)
	for _, entries := range [][]string{base, info.ComponentEnvList} {
		for _, entry := range entries {
			key, value, ok := strings.Cut(entry, "=")
			if ok {
				env[key] = value
			}
		}
	}
	return env
}

func (s *Session) providerSettings(name string) *pluginv1.ComponentValue {
	var settings *pluginv1.ComponentValue
	for _, request := range s.requests {
		if request.Provider == name {
			settings = request.ProviderConfiguration
		}
	}
	return settings
}
