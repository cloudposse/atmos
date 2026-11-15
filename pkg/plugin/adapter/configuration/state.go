package configuration

import (
	"context"
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ResolveState locates state with no evaluator commands, writes, network, or output reads.
func (s *Session) ResolveState(ctx context.Context, info *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
	defer perf.Track(nil, "configuration.Session.ResolveState")()

	if !schema.ConfigurationOwned(info.ComponentSettingsSection) {
		return nil, nil
	}
	selected, err := s.componentRequest(ctx, info)
	if err != nil {
		return nil, err
	}
	// Discovery and preparation do not authorize state resolution; the request grants no effects.
	result, err := selected.provider.ResolveState(adapter.WithLogScope(ctx, logScope(selected.request, info, &s.warnings)), selected.request)
	if err != nil {
		return nil, err
	}
	adapter.RenderLogs(logScope(selected.request, info, &s.warnings), result.GetLogs())
	if result == nil || result.State == nil {
		return nil, errUtils.ErrPluginProtocol
	}
	state := result.State
	backend, err := adapter.DecodeObject(state.Backend)
	if err != nil {
		return nil, err
	}
	if state.BackendType == "local" {
		resolveLocalBackend(backend, state.WorkingDirectory)
	}
	return &schema.ComponentConfigurationState{BackendType: state.BackendType, Backend: backend, Workspace: state.Workspace, WorkingDirectory: state.WorkingDirectory}, nil
}

// Reevaluate the inventory without preparation effects before any dependency
// outputs are read. Identity and component environment overrides must not change
// the units that the scheduler ordered using the original discovery result.
func (s *Session) checkPreparationDependencies(ctx context.Context, instance *pluginv1.DiscoveredComponent, env map[string]string, command string) error {
	baseline := s.preparationBaseline(instance)
	if baseline == nil {
		return errUtils.ErrPluginInvalidConfig
	}
	request := proto.Clone(baseline.request).(*pluginv1.DiscoverComponentsRequest)
	request.Invocation = &pluginv1.Invocation{Environment: env}
	request.Operation = proto.Clone(baseline.request.Operation).(*pluginv1.ComponentOperation)
	request.Operation.Command = command
	response, err := s.registry[instance.Provider].Discover(ctx, request)
	if err != nil {
		return err
	}
	current := make(map[[3]string]*pluginv1.DiscoveredComponent)
	if err := addDiscovered(make(map[string]map[string]any), current, response); err != nil {
		return err
	}
	selected := current[addressKey(baseline.self.Address)]
	if !sameDependencyUnit(baseline.self, selected) || !sameDependencies(baseline.self.Dependencies, selected.Dependencies) {
		return fmt.Errorf("%w: preparation changed discovered dependency paths", errUtils.ErrPluginInvalidConfig)
	}
	for _, dependency := range selected.Dependencies {
		key := addressKey(dependency.Target)
		before, after := baseline.units[key], current[key]
		// A target outside the evaluated scope, such as a dependency beyond a single imported
		// unit's root, is absent from both inventories. The unchanged edge is already compared above.
		if before == nil && after == nil {
			continue
		}
		if !sameDependencyUnit(before, after) {
			return fmt.Errorf("%w: preparation changed discovered dependency paths", errUtils.ErrPluginInvalidConfig)
		}
	}
	return s.checkDependencyStates(ctx, baseline, selected.Dependencies, env, command)
}

// preparationEvidence is the discovery request that found a unit, every unit that request found,
// and the unit as that request reported it, all under the provider's own addresses.
type preparationEvidence struct {
	request *pluginv1.DiscoverComponentsRequest
	units   map[[3]string]*pluginv1.DiscoveredComponent
	self    *pluginv1.DiscoveredComponent
	// importStack is the stack that imported the unit; it is empty for a discovered unit.
	importStack string
}

// preparationBaseline finds the evidence for a discovered unit in its provider's discovery
// request, and for an imported unit in the import that resolved it. An imported unit is
// addressed by the importing stack, so its provider address comes from the cached import.
func (s *Session) preparationBaseline(instance *pluginv1.DiscoveredComponent) *preparationEvidence {
	if discovered := s.instances[addressKey(instance.Address)]; discovered != nil && discovered.Source == instance.Source {
		for _, request := range s.requests {
			if request.Provider == instance.Provider {
				return &preparationEvidence{request: request, units: s.instances, self: discovered}
			}
		}
		return nil
	}
	return s.importBaseline(instance)
}

// importBaseline finds the cached import that resolved an imported unit.
func (s *Session) importBaseline(instance *pluginv1.DiscoveredComponent) *preparationEvidence {
	s.importMu.lock()
	defer s.importMu.unlock()
	reported := s.imported[instance.Source]
	if reported == nil {
		return nil
	}
	for key, units := range s.importResults {
		if !slices.Contains(units, reported) {
			continue
		}
		template := s.importTemplate(instance.Provider)
		if template == nil {
			return nil
		}
		request := proto.Clone(template).(*pluginv1.DiscoverComponentsRequest)
		request.IncludePaths, request.ExcludePaths, request.Imports = nil, nil, []string{key}
		byAddress := make(map[[3]string]*pluginv1.DiscoveredComponent, len(units))
		for _, unit := range units {
			byAddress[addressKey(unit.Address)] = unit
		}
		return &preparationEvidence{request: request, units: byAddress, self: reported, importStack: instance.Address.Stack}
	}
	return nil
}

// importTemplate returns the import request template of a provider, under any configured alias.
func (s *Session) importTemplate(provider string) *pluginv1.DiscoverComponentsRequest {
	for _, template := range s.importProviders {
		if template.Provider == provider {
			return template
		}
	}
	return nil
}

// checkDependencyStates keeps output reads on the dependency's own state. The
// provider evaluates a dependency's remote_state under the dependent's
// environment, so an identity or component override can select different state.
// The reference is the identity the dependency was prepared with in this
// invocation, or else its identity under the discovery environment.
func (s *Session) checkDependencyStates(ctx context.Context, baseline *preparationEvidence, dependencies []*pluginv1.ComponentDependency, env map[string]string, command string) error {
	discovery := baseline.request
	for _, dependency := range dependencies {
		if dependency.Kind != pluginv1.ComponentDependency_OUTPUT {
			continue
		}
		key := addressKey(dependency.Target)
		target := baseline.units[key]
		if target == nil {
			// The provider did not evaluate the target, so it has no state evidence to compare.
			continue
		}
		current, err := s.dependencyState(ctx, discovery, target, env, command)
		if err != nil {
			return err
		}
		expected, err := s.referenceState(ctx, discovery, target, preparedKey(baseline, key), command)
		if err != nil {
			return err
		}
		if !proto.Equal(expected, current) {
			return fmt.Errorf("%w: dependency %s/%s state differs under the dependent's environment", errUtils.ErrPluginInvalidConfig, key[0], key[2])
		}
	}
	return nil
}

// preparedKey is the address under which a dependency's prepared state is stored: the address
// the dependency was selected with. An imported unit's dependencies are its siblings in the
// importing stack, so they take that stack rather than the provider's own.
func preparedKey(baseline *preparationEvidence, key [3]string) [3]string {
	if baseline.importStack != "" {
		key[0] = baseline.importStack
	}
	return key
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
