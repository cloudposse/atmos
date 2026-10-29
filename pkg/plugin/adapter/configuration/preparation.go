package configuration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	settings := s.providerSettings(instance.Provider)
	env := preparationEnvironment(info)
	env["ATMOS_TERRAFORM_BINARY"] = info.Command
	env["TF_WORKSPACE"] = info.TerraformWorkspace
	result, err := provider.Prepare(ctx, &pluginv1.ComponentRequest{
		Provider: instance.Provider, Address: instance.Address, Source: instance.Source, RootDirectory: s.root,
		Configuration: instance.Configuration, ProviderConfiguration: settings,
		Invocation: &pluginv1.Invocation{Environment: env},
		Operation:  &pluginv1.ComponentOperation{Command: info.SubCommand, Mode: "native"},
		Effects:    &pluginv1.ComponentEffects{Commands: true, Network: true, Secrets: true, Writes: true, StateReads: true},
	})
	if err != nil {
		return nil, err
	}
	if result.Engine != instance.Engine {
		return nil, errUtils.ErrPluginProtocol
	}
	return preparedConfiguration(result)
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
	prepared := &schema.PreparedComponentConfiguration{Engine: result.Engine, Source: result.ModuleSource, Inputs: inputs, BackendType: result.State.BackendType, Backend: backend, Workspace: result.State.Workspace}
	for _, file := range result.Files {
		if file.IfExists == "overwrite_terragrunt" {
			return nil, errUtils.ErrPluginUnsupportedExtension
		}
		prepared.Files = append(prepared.Files, schema.ComponentConfigurationFile{Path: file.Path, Content: file.Content, IfExists: file.IfExists, Sensitive: file.Sensitive})
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
