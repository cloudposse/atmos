package configuration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Prepare reevaluates only the selected instance, after the scheduler's dependencies.
func (s *Session) Prepare(ctx context.Context, info *schema.ConfigAndStacksInfo) (*schema.PreparedComponentConfiguration, error) {
	defer perf.Track(nil, "configurationadapter.Prepare")()

	if !schema.ConfigurationOwned(info.ComponentSettingsSection) {
		return nil, nil
	}
	if info.UseTerraformPlan || info.PlanFile != "" || info.StoredPlanFile != "" {
		return nil, fmt.Errorf("%w: plugin saved-plan binding is not implemented", errUtils.ErrPluginUnsupportedExtension)
	}
	instance, provider, err := s.preparationTarget(ctx, info)
	if err != nil {
		return nil, err
	}
	settings := s.providerSettings(instance.Provider)
	env := preparationEnvironment(info)
	env["ATMOS_TERRAFORM_BINARY"] = info.Command
	env["TF_WORKSPACE"] = info.TerraformWorkspace
	if err := s.checkPreparationDependencies(ctx, instance, env, info.SubCommand); err != nil {
		return nil, err
	}
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
	if result.State != nil {
		s.prepared.Store(addressKey(instance.Address), proto.Clone(result.State))
	}
	return preparedConfiguration(result)
}

// preparationTarget resolves the discovered instance selected by info and the
// provider that owns it, after the session inventory has been loaded.
func (s *Session) preparationTarget(ctx context.Context, info *schema.ConfigAndStacksInfo) (*pluginv1.DiscoveredComponent, Provider, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, nil, err
	}
	instance := s.instances[[3]string{info.Stack, "terraform", info.ComponentFromArg}]
	if instance == nil {
		return nil, nil, errUtils.ErrPluginInvalidConfig
	}
	provider, ok := s.registry[instance.Provider]
	if !ok {
		return nil, nil, errUtils.ErrPluginUnsupportedExtension
	}
	return instance, provider, nil
}

// Reevaluate the inventory without preparation effects before any dependency
// outputs are read. Identity and component environment overrides must not change
// the units that the scheduler ordered using the original discovery result.
func (s *Session) checkPreparationDependencies(ctx context.Context, instance *pluginv1.DiscoveredComponent, env map[string]string, command string) error {
	for _, original := range s.requests {
		if original.Provider != instance.Provider {
			continue
		}
		request := proto.Clone(original).(*pluginv1.DiscoverComponentsRequest)
		request.Invocation = &pluginv1.Invocation{Environment: env}
		request.Operation = &pluginv1.ComponentOperation{Command: command, Mode: "native"}
		response, err := s.registry[instance.Provider].Discover(ctx, request)
		if err != nil {
			return err
		}
		current := make(map[[3]string]*pluginv1.DiscoveredComponent)
		if err := addDiscovered(make(map[string]map[string]any), current, response); err != nil {
			return err
		}
		selected := current[addressKey(instance.Address)]
		if !sameDependencyUnit(instance, selected) || !sameDependencies(instance.Dependencies, selected.Dependencies) {
			return fmt.Errorf("%w: preparation changed discovered dependency paths", errUtils.ErrPluginInvalidConfig)
		}
		for _, dependency := range selected.Dependencies {
			key := addressKey(dependency.Target)
			if !sameDependencyUnit(s.instances[key], current[key]) {
				return fmt.Errorf("%w: preparation changed discovered dependency paths", errUtils.ErrPluginInvalidConfig)
			}
		}
		return s.checkDependencyStates(ctx, original, selected.Dependencies, env, command)
	}
	return errUtils.ErrPluginInvalidConfig
}

// checkDependencyStates keeps output reads on the dependency's own state. The
// provider evaluates a dependency's remote_state under the dependent's
// environment, so an identity or component override can select different state.
// The reference is the identity the dependency was prepared with in this
// invocation, or else its identity under the discovery environment.
func (s *Session) checkDependencyStates(ctx context.Context, discovery *pluginv1.DiscoverComponentsRequest, dependencies []*pluginv1.ComponentDependency, env map[string]string, command string) error {
	for _, dependency := range dependencies {
		if dependency.Kind != pluginv1.ComponentDependency_OUTPUT {
			continue
		}
		key := addressKey(dependency.Target)
		target := s.instances[key]
		current, err := s.dependencyState(ctx, discovery, target, env, command)
		if err != nil {
			return err
		}
		expected, err := s.referenceState(ctx, discovery, target, key, command)
		if err != nil {
			return err
		}
		if !proto.Equal(expected, current) {
			return fmt.Errorf("%w: dependency %s/%s state differs under the dependent's environment", errUtils.ErrPluginInvalidConfig, key[0], key[2])
		}
	}
	return nil
}

func (s *Session) referenceState(ctx context.Context, discovery *pluginv1.DiscoverComponentsRequest, target *pluginv1.DiscoveredComponent, key [3]string, command string) (*pluginv1.ComponentStateIdentity, error) {
	if prepared, ok := s.prepared.Load(key); ok {
		return prepared.(*pluginv1.ComponentStateIdentity), nil
	}
	return s.dependencyState(ctx, discovery, target, discovery.GetInvocation().GetEnvironment(), command)
}

func (s *Session) dependencyState(ctx context.Context, discovery *pluginv1.DiscoverComponentsRequest, target *pluginv1.DiscoveredComponent, env map[string]string, command string) (*pluginv1.ComponentStateIdentity, error) {
	result, err := s.registry[target.Provider].ResolveState(ctx, &pluginv1.ComponentRequest{
		Provider: target.Provider, Address: target.Address, Source: target.Source, RootDirectory: s.root,
		Configuration: target.Configuration, ProviderConfiguration: discovery.ProviderConfiguration,
		Invocation: &pluginv1.Invocation{Environment: env},
		Operation:  &pluginv1.ComponentOperation{Command: command, Mode: "native"},
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.State == nil {
		return nil, errUtils.ErrPluginProtocol
	}
	return result.State, nil
}

func sameDependencyUnit(before, after *pluginv1.DiscoveredComponent) bool {
	return before != nil && after != nil && before.Source == after.Source && before.Provider == after.Provider && before.Engine == after.Engine
}

func sameDependencies(before, after []*pluginv1.ComponentDependency) bool {
	type edge struct {
		target [3]string
		kind   pluginv1.ComponentDependency_Kind
	}
	counts := make(map[edge]int)
	for _, dependency := range before {
		counts[edge{addressKey(dependency.Target), dependency.Kind}]++
	}
	for _, dependency := range after {
		counts[edge{addressKey(dependency.Target), dependency.Kind}]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func preparedConfiguration(result *pluginv1.PrepareComponentResponse) (*schema.PreparedComponentConfiguration, error) {
	inputs, sensitiveKeys, err := adapter.DecodeObjectWithSensitiveKeys(result.Inputs)
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
	prepared := &schema.PreparedComponentConfiguration{Engine: result.Engine, Source: result.ModuleSource, Inputs: inputs, SensitiveInputKeys: sensitiveKeys, BackendType: result.State.BackendType, Backend: backend, Workspace: result.State.Workspace}
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
