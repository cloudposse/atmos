package configuration

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

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
	result, err := selected.provider.Prepare(ctx, selected.request)
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
	if nanos := result.GetRegistryResolutionNanos(); nanos > 0 && nanos <= uint64(math.MaxInt64) {
		perf.ObserveExternal("atmosbridge.ResolveRegistrySource", time.Duration(nanos))
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
	mode, options := s.providerOperation(instance.Provider)
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
		if !sameDependencyUnit(baseline.units[key], current[key]) {
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
		return &preparationEvidence{request: request, units: byAddress, self: reported}
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
	inputs, err := adapter.DecodeInputs(result.Inputs)
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
	prepared.SensitiveInputKeys = sensitiveInputKeys(result.Inputs, inputs)
	if prepared.SourceAuthorization, err = preparedSourceAuthorization(result); err != nil {
		return nil, err
	}
	execution, err := preparedExecution(result.Execution)
	if err != nil {
		return nil, err
	}
	prepared.Execution = execution
	if prepared.IAM, err = preparedIAM(result.Iam); err != nil {
		return nil, err
	}
	if err := applyParity(prepared, result); err != nil {
		return nil, err
	}
	for _, file := range result.Files {
		prepared.Files = append(prepared.Files, schema.ComponentConfigurationFile{Path: file.Path, Content: file.Content, IfExists: file.IfExists, Sensitive: file.Sensitive, UnitFileCopy: file.UnitFileCopy})
	}
	for _, source := range result.Sources {
		prepared.Sources = append(prepared.Sources, schema.ComponentConfigurationSource{Path: source.Path, Directory: source.Kind == pluginv1.ComponentSource_DIRECTORY})
	}

	return prepared, nil
}

// sensitiveInputKeys preserves declared sensitivity independently of the decoded value.
// A sensitive empty string cannot be found later by value-based secret detection.
func sensitiveInputKeys(value *pluginv1.ComponentValue, inputs map[string]any) map[string]bool {
	if value == nil || len(value.SensitivePaths) == 0 {
		return nil
	}
	keys := make(map[string]bool)
	for _, path := range value.SensitivePaths {
		if path == nil || len(path.Steps) == 0 || path.Steps[0].GetAttribute() == "" {
			for key := range inputs {
				keys[key] = true
			}
			continue
		}
		keys[path.Steps[0].GetAttribute()] = true
	}
	return keys
}

func preparedSourceAuthorization(result *pluginv1.PrepareComponentResponse) (*schema.SourceAuthorization, error) {
	encoded := result.GetModuleSourceAuthorization()
	origin := result.GetModuleSourceAuthOrigin()
	if encoded == nil && origin == "" {
		return nil, nil
	}
	if !validSensitiveAuthorization(encoded, origin) {
		return nil, errUtils.ErrPluginProtocol
	}
	authorization, err := adapter.DecodeString(encoded)
	if err != nil || authorization == "" {
		return nil, errUtils.ErrPluginProtocol
	}
	if !sameSourceOrigin(result.GetModuleSource(), origin) {
		return nil, errUtils.ErrPluginProtocol
	}
	return &schema.SourceAuthorization{Origin: origin, Header: authorization}, nil
}

func validSensitiveAuthorization(encoded *pluginv1.ComponentValue, origin string) bool {
	return encoded != nil && origin != "" && len(encoded.GetSensitivePaths()) == 1 && len(encoded.GetSensitivePaths()[0].GetSteps()) == 0
}

func sameSourceOrigin(source, origin string) bool {
	archive, archiveErr := url.Parse(source)
	registry, registryErr := url.Parse(origin)
	if archiveErr != nil || registryErr != nil || !validRegistryOrigin(registry) {
		return false
	}
	return archive.Scheme == registry.Scheme && strings.EqualFold(archive.Host, registry.Host)
}

func validRegistryOrigin(registry *url.URL) bool {
	return registry.Host != "" && registry.User == nil && registry.Path == "" && registry.RawQuery == "" && registry.Fragment == "" && (registry.Scheme == "https" || registry.Scheme == "http")
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
