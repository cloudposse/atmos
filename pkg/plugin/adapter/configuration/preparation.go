package configuration

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	componentconfiguration "github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/component/terragruntexec"
	"github.com/cloudposse/atmos/pkg/downloader"
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
	if err := rejectSavedPlan(info); err != nil {
		return nil, err
	}
	// Executable mode never reaches native preparation: the two modes must not mix silently.
	if terragruntexec.Owns(info) {
		return nil, fmt.Errorf("%w: component %q in stack %q runs through the terragrunt executable, so the provider does not prepare it", errUtils.ErrExecutableModeUnsupported, info.ComponentFromArg, info.Stack)
	}
	selected, err := s.componentRequest(ctx, info)
	if err != nil {
		return nil, err
	}
	result, err := s.runProviderPrepare(ctx, info, selected)
	if err != nil {
		return nil, err
	}
	prepared, err := preparedConfiguration(result)
	if err != nil {
		return nil, err
	}
	if err = checkTerraformBinary(info, selected.instance, prepared.TerraformBinary); err != nil {
		return nil, err
	}
	warnDiagnostics(result.Diagnostics, &s.warnings)
	if err := checkSourcesInRoot(ctx, s.root, prepared.Sources); err != nil {
		return nil, err
	}
	return prepared, nil
}

// ProbeIAM reads only the Terragrunt flags before full evaluation. It must not
// run infrastructure commands or return a module configuration.
func (s *Session) ProbeIAM(ctx context.Context, info *schema.ConfigAndStacksInfo) (*schema.ComponentIAM, error) {
	defer perf.Track(nil, "configuration.Session.ProbeIAM")()

	selected, err := s.componentRequest(ctx, info)
	if err != nil {
		return nil, err
	}
	if selected.instance.Provider != "terragrunt" {
		return nil, nil
	}
	selected.request.Operation.Mode = "iam-probe"
	selected.request.Effects = &pluginv1.ComponentEffects{Commands: true, Network: true, Secrets: true, Writes: true, StateReads: true}
	result, err := selected.provider.Prepare(ctx, selected.request)
	if err != nil {
		return nil, err
	}
	for _, diagnostic := range result.Diagnostics {
		if err := adapter.DiagnosticError(diagnostic); err != nil {
			return nil, err
		}
	}
	return preparedIAM(result.Iam)
}

// rejectSavedPlan refuses a saved plan, which a plugin-provided configuration cannot bind to yet.
func rejectSavedPlan(info *schema.ConfigAndStacksInfo) error {
	if info.UseTerraformPlan || info.PlanFile != "" {
		return fmt.Errorf("%w: plugin saved-plan binding is not implemented", errUtils.ErrPluginUnsupportedExtension)
	}
	return nil
}

// runProviderPrepare asks the owning provider to reevaluate the selected unit with every effect
// granted, after checking module ownership and engine selection. Only an explicit --command
// flag may switch the engine discovered from the global configuration.
func (s *Session) runProviderPrepare(ctx context.Context, info *schema.ConfigAndStacksInfo, selected *selectedComponent) (*pluginv1.PrepareComponentResponse, error) {
	if err := checkProviderSelection(info, selected.instance); err != nil {
		return nil, err
	}
	if err := s.checkPreparationDependencies(ctx, selected.instance, selected.request.Invocation.Environment, info.SubCommand); err != nil {
		return nil, err
	}
	selected.request.Effects = &pluginv1.ComponentEffects{Commands: true, Network: true, Secrets: true, Writes: true, StateReads: true}
	result, err := selected.provider.Prepare(adapter.WithLogScope(ctx, logScope(selected.request, info, &s.warnings)), selected.request)
	if err != nil {
		return nil, err
	}
	// The log events of the same response can quote a sensitive input, for example the output of
	// sensitive(run_cmd(...)) at debug level, so the inputs are registered for masking first.
	// Reject malformed or unknown inputs before rendering any response content: until
	// they decode successfully, their sensitive values cannot be registered safely.
	if err := s.renderPreparationLogs(info, selected, result); err != nil {
		return nil, err
	}
	if err := validatePreparationResponse(info, selected.instance, result); err != nil {
		return nil, err
	}
	if result.State != nil {
		s.prepared.Store(addressKey(selected.instance.Address), proto.Clone(result.State))
	}
	return result, nil
}

func validatePreparationResponse(info *schema.ConfigAndStacksInfo, instance *pluginv1.DiscoveredComponent, result *pluginv1.PrepareComponentResponse) error {
	// An error diagnostic fails the preparation; warnings are printed by the caller.
	for _, diagnostic := range result.Diagnostics {
		if err := adapter.DiagnosticError(diagnostic); err != nil {
			return err
		}
	}
	// The provider prepares for the binary it is given, so any other engine is a protocol violation.
	if result.Engine != expectedEngine(info, instance) {
		return errUtils.ErrPluginProtocol
	}
	return nil
}

func checkProviderSelection(info *schema.ConfigAndStacksInfo, instance *pluginv1.DiscoveredComponent) error {
	if err := checkSourceOverride(info, instance); err != nil {
		return err
	}
	if engine := expectedEngine(info, instance); engine != instance.Engine && info.ComponentCommand == "" {
		return engineMismatch(info, instance.Engine, engine)
	}
	return nil
}

func (s *Session) renderPreparationLogs(info *schema.ConfigAndStacksInfo, selected *selectedComponent, result *pluginv1.PrepareComponentResponse) error {
	if _, err := adapter.DecodeInputs(result.GetInputs()); err != nil {
		return err
	}
	adapter.RenderLogs(logScope(selected.request, info, &s.warnings), result.GetLogs())
	return nil
}

// logScope names the component and stack a plugin's log events belong to.
func logScope(request *pluginv1.ComponentRequest, info *schema.ConfigAndStacksInfo, warnings *adapter.WarningDeduper) adapter.LogScope {
	return adapter.LogScope{Provider: request.GetProvider(), Component: info.ComponentFromArg, Stack: info.Stack, Warnings: warnings}
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
	// Either URI can carry credentials, so the error shows only their redacted forms.
	return errUtils.Build(fmt.Errorf("%w: the Terraform module of component %q in stack %q is owned by its unit, so `source.uri: %s` cannot replace `%s`", errUtils.ErrConfigurationOverrideUnsupported, info.ComponentFromArg, info.Stack, downloader.RedactSource(got), downloader.RedactSource(want))).
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

// expectedEngine is the engine of the command this invocation runs. Discovery records the engine of
// the globally configured command, but --command may explicitly select another one,
// and the provider prepares for the binary it is given. Any other engine is a protocol violation.
func expectedEngine(info *schema.ConfigAndStacksInfo, instance *pluginv1.DiscoveredComponent) string {
	if info.Command == "" {
		return instance.Engine
	}
	// Match execution's executable-name detection (pkg/component/terraform/detection.go).
	if strings.Contains(strings.ToLower(filepath.Base(info.Command)), "tofu") {
		return "opentofu"
	}
	return "terraform"
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
	instance, err := s.instanceFor(info)
	if err != nil {
		return nil, err
	}
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
	mode, options, err := s.providerOperation(instance.Provider, env)
	if err != nil {
		return nil, err
	}
	request := &pluginv1.ComponentRequest{
		Provider: instance.Provider, Address: instance.Address, Source: instance.Source, RootDirectory: s.root,
		Configuration: instance.Configuration, ProviderConfiguration: s.providerSettings(instance.Provider),
		Invocation: &pluginv1.Invocation{Environment: env, LogLevel: adapter.HostLogLevel()},
		Operation:  &pluginv1.ComponentOperation{Command: info.SubCommand, Mode: mode, Options: options, TerraformArguments: slices.Clone(info.AdditionalArgsAndFlags)},
	}
	return &selectedComponent{request: request, provider: provider, instance: instance}, nil
}

// instanceFor finds the unit behind a component. A discovered unit is addressed by its derived
// stack; an imported unit lives in whichever stack imported it, so it is found by its source file
// and addressed as the stack and component the user selected. When a discovered unit and an
// imported unit claim the same stack and component, neither is chosen: the conflict is reported.
func (s *Session) instanceFor(info *schema.ConfigAndStacksInfo) (*pluginv1.DiscoveredComponent, error) {
	configuration, _ := info.ComponentSettingsSection["configuration"].(map[string]any)
	source, _ := configuration["source"].(string)
	s.importMu.lock()
	imported := s.imported[source]
	s.importMu.unlock()
	if discovered := s.instances[[3]string{info.Stack, "terraform", info.ComponentFromArg}]; discovered != nil {
		if imported != nil && imported.Source != discovered.Source {
			return nil, componentconfiguration.ImportConflict(info.ComponentFromArg, discovered.Source, imported.Source, "stack "+info.Stack+": one discovered, one imported")
		}
		return discovered, nil
	}
	if imported == nil {
		return nil, nil
	}
	instance := proto.Clone(imported).(*pluginv1.DiscoveredComponent)
	instance.Address = &pluginv1.ComponentAddress{Stack: info.Stack, Component: info.ComponentFromArg, ComponentType: terraformType}
	return instance, nil
}
