package configuration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	if err = checkSourceOverride(info, selected.instance); err != nil {
		return nil, err
	}
	selected.request.Effects = &pluginv1.ComponentEffects{Commands: true, Network: true, Secrets: true, Writes: true, StateReads: true}
	result, err := selected.provider.Prepare(ctx, selected.request)
	if err != nil {
		return nil, err
	}
	if result.Engine != selected.instance.Engine {
		return nil, engineMismatch(info, selected.instance.Engine, result.Engine)
	}
	prepared, err := preparedConfiguration(result)
	if err != nil {
		return nil, err
	}
	if err := checkSourcesInRoot(ctx, s.root, prepared.Sources); err != nil {
		return nil, err
	}
	return prepared, nil
}

// checkSourceOverride rejects a stack that points the component at another module. The module
// belongs to the unit's own `terraform { source }`; every other `source` attribute (version, ttl,
// included paths) is ordinary configuration and merges normally.
func checkSourceOverride(info *schema.ConfigAndStacksInfo, instance *pluginv1.DiscoveredComponent) error {
	discovered, err := adapter.DecodeObject(instance.Configuration)
	if err != nil {
		return err
	}
	want, got := sourceURI(discovered["source"]), sourceURI(info.ComponentSection["source"])
	if want == "" || got == "" || want == got {
		return nil
	}
	return errUtils.Build(fmt.Errorf("%w: the Terraform module of component %q in stack %q is owned by its unit, so `source.uri: %s` cannot replace `%s`", errUtils.ErrConfigurationOverrideUnsupported, info.ComponentFromArg, info.Stack, got, want)).
		WithContext("component", info.ComponentFromArg).
		WithContext("stack", info.Stack).
		WithHint("Change `terraform { source = ... }` in the unit's `terragrunt.hcl`, or define a regular component with its own `source`").
		Err()
}

// sourceURI returns the module location of a component `source`, written as a string or as a map with `uri`.
func sourceURI(source any) string {
	switch value := source.(type) {
	case string:
		return value
	case map[string]any:
		uri, _ := value["uri"].(string)
		return uri
	}
	return ""
}

// engineMismatch explains a command that selects a different engine than the one the component was
// discovered with. Stack configuration may pick another version or path of the same engine, but
// not switch between Terraform and OpenTofu, because the unit's lock file and state belong to one.
func engineMismatch(info *schema.ConfigAndStacksInfo, discovered, selected string) error {
	return errUtils.Build(fmt.Errorf("%w: command %q selects the %s engine, but component %q was discovered for %s", errUtils.ErrConfigurationOverrideUnsupported, info.Command, selected, info.ComponentFromArg, discovered)).
		WithContext("component", info.ComponentFromArg).
		WithContext("stack", info.Stack).
		WithContext("command", info.Command).
		WithHint("Set `command` to another " + discovered + " executable, or change `components.terraform.command` in `atmos.yaml` to switch every unit to " + selected).
		Err()
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
	// A unit that failed discovery reports its own diagnostic, not a generic lookup failure.
	if err := s.failureFor(info.Stack, info.ComponentFromArg); err != nil {
		return nil, err
	}
	instance := s.instanceFor(info)
	if instance == nil {
		return nil, errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanation("The component is not provided by any enabled configuration provider.").
			WithContext("component", info.ComponentFromArg).
			WithContext("stack", info.Stack).
			WithHint("Run `atmos list instances` to see the discovered and imported components").
			Err()
	}
	registry, err := s.providers(ctx)
	if err != nil {
		return nil, err
	}
	provider, ok := registry[instance.Provider]
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

// instanceFor finds the unit behind a component. A discovered unit is addressed by its derived
// stack; an imported unit lives in whichever stack imported it, so it is found by its source file
// and addressed as the stack and component the user selected.
func (s *Session) instanceFor(info *schema.ConfigAndStacksInfo) *pluginv1.DiscoveredComponent {
	if instance := s.instances[[3]string{info.Stack, "terraform", info.ComponentFromArg}]; instance != nil {
		return instance
	}
	configuration, _ := info.ComponentSettingsSection["configuration"].(map[string]any)
	source, _ := configuration["source"].(string)
	s.importMu.Lock()
	imported := s.imported[source]
	s.importMu.Unlock()
	if imported == nil {
		return nil
	}
	instance := proto.Clone(imported).(*pluginv1.DiscoveredComponent)
	instance.Address = &pluginv1.ComponentAddress{Stack: info.Stack, Component: info.ComponentFromArg, ComponentType: terraformType}
	return instance
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
	for _, request := range s.importProviders {
		if request.Provider == name {
			settings = request.ProviderConfiguration
		}
	}
	return settings
}
