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
	"github.com/cloudposse/atmos/pkg/downloader"
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
	if err := rejectSavedPlan(info); err != nil {
		return nil, err
	}
	selected, err := s.componentRequest(ctx, info)
	if err != nil {
		return nil, err
	}
	result, err := runProviderPrepare(ctx, info, selected)
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
	warnDiagnostics(result.Diagnostics)
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
	if info.UseTerraformPlan || info.PlanFile != "" || info.StoredPlanFile != "" {
		return fmt.Errorf("%w: plugin saved-plan binding is not implemented", errUtils.ErrPluginUnsupportedExtension)
	}
	return nil
}

// runProviderPrepare asks the owning provider to reevaluate the selected unit with every effect
// granted, after checking module ownership and engine selection. Only an explicit --command
// flag may switch the engine discovered from the global configuration.
func runProviderPrepare(ctx context.Context, info *schema.ConfigAndStacksInfo, selected *selectedComponent) (*pluginv1.PrepareComponentResponse, error) {
	if err := checkSourceOverride(info, selected.instance); err != nil {
		return nil, err
	}
	if engine := expectedEngine(info, selected.instance); engine != selected.instance.Engine && info.ComponentCommand == "" {
		return nil, engineMismatch(info, selected.instance.Engine, engine)
	}
	selected.request.Effects = &pluginv1.ComponentEffects{Commands: true, Network: true, Secrets: true, Writes: true, StateReads: true}
	result, err := selected.provider.Prepare(ctx, selected.request)
	if err != nil {
		return nil, err
	}
	if nanos := result.GetRegistryResolutionNanos(); nanos > 0 && nanos <= uint64(math.MaxInt64) {
		perf.ObserveExternal("atmosbridge.ResolveRegistrySource", time.Duration(nanos))
	}
	// An error diagnostic fails the preparation; warnings are printed by the caller.
	for _, diagnostic := range result.Diagnostics {
		if err = adapter.DiagnosticError(diagnostic); err != nil {
			return nil, err
		}
	}
	// The provider prepares for the binary it is given, so any other engine is a protocol violation.
	if result.Engine != expectedEngine(info, selected.instance) {
		return nil, errUtils.ErrPluginProtocol
	}
	return result, nil
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
