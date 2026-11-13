package component

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func fixtureActivations(t *testing.T) []*extension.Activation {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schema.json"), []byte(`{"type":"object","additionalProperties":false,"properties":{"limit":{"type":"number"}}}`), 0o600))
	executable, err := os.Executable()
	require.NoError(t, err)
	expected := &pluginv1.DescribeResponse{PluginName: "fixture/components", PluginVersion: "0.1.0", ManifestDigest: "sha256:fixture"}
	var result []*extension.Activation
	for _, capability := range []string{extension.ComponentDiscovery, extension.ComponentValidation, extension.ComponentPreparation, extension.ComponentState} {
		expected.Services = append(expected.Services, &pluginv1.ServiceVersionRange{Service: capability, Min: 1, Max: 1})
		expected.Exports = append(expected.Exports, &pluginv1.Export{Service: capability, Kind: "fixture/hcl"})
		result = append(result, &extension.Activation{Binding: extension.Binding{Capability: capability, Name: "fixture/hcl", Directory: directory, Metadata: map[string]any{"component_type": "terraform", "schema": "schema.json"}}, Package: expected.PluginName, Executable: executable, Description: expected})
	}
	raw, err := proto.Marshal(expected)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "description.pb"), raw, 0o600))
	return result
}

func fixtureProvider(t *testing.T) *Provider {
	t.Helper()
	registry, err := NewRegistry(fixtureActivations(t))
	require.NoError(t, err)
	p, found := registry.Get("fixture/hcl")
	require.True(t, found)
	return p
}

func wireValue(t *testing.T, value cty.Value) *pluginv1.ComponentValue {
	t.Helper()
	typeJSON, err := ctyjson.MarshalType(value.Type())
	require.NoError(t, err)
	payload, err := msgpack.Marshal(value, value.Type())
	require.NoError(t, err)
	return &pluginv1.ComponentValue{TypeJson: typeJSON, Msgpack: payload}
}

func TestProviderCallsPreserveValuesAndIsolateInvocations(t *testing.T) {
	p := fixtureProvider(t)
	root := t.TempDir()
	number, err := cty.ParseNumberVal("9007199254740993123456789.123456789")
	require.NoError(t, err)
	request := &pluginv1.ComponentRequest{
		RootDirectory: root, Address: fixtureAddress(),
		Invocation:            &pluginv1.Invocation{Id: "caller-id", Interactive: true, Environment: map[string]string{"EXPLICIT": "value"}},
		Operation:             &pluginv1.ComponentOperation{Command: "plan", Mode: "native"},
		Configuration:         wireValue(t, cty.ObjectVal(map[string]cty.Value{"exact": number, "unknown": cty.UnknownVal(cty.String)})),
		ProviderConfiguration: wireValue(t, cty.ObjectVal(map[string]cty.Value{"limit": number})),
	}
	request.Configuration.SensitivePaths = []*pluginv1.ComponentValuePath{{}}
	before := proto.Clone(request)
	state, err := p.ResolveState(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "development", state.State.Workspace)
	require.NoFileExists(t, filepath.Join(root, "preparation.pb"))
	validated, err := p.Validate(t.Context(), request)
	require.NoError(t, err)
	require.Empty(t, validated.Diagnostics)
	discovery, err := p.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: root, Operation: request.Operation, ProviderConfiguration: request.ProviderConfiguration})
	require.NoError(t, err)
	require.Len(t, discovery.Components, 1)
	require.Equal(t, "dev/app", discovery.Components[0].Address.Component)
	prepared, err := p.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.True(t, proto.Equal(request.Configuration, prepared.Inputs))
	require.Len(t, prepared.Files, 1)
	require.Equal(t, []byte("$${literal}\n%{ literal }\n"), prepared.Files[0].Content)
	require.NoFileExists(t, filepath.Join(root, "backend.tf"), "adapter does not materialize artifacts")
	require.True(t, proto.Equal(before, request), "adapter must not mutate caller input")
	seen := &pluginv1.ComponentRequest{}
	raw, err := os.ReadFile(filepath.Join(root, "preparation.pb"))
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(raw, seen))
	require.NotEmpty(t, seen.Invocation.Id)
	require.NotEqual(t, "caller-id", seen.Invocation.Id)
	require.False(t, seen.Invocation.Interactive)
	require.Equal(t, map[string]string{"EXPLICIT": "value"}, seen.Invocation.Environment)
	require.True(t, proto.Equal(request.ProviderConfiguration, seen.ProviderConfiguration))
}

func TestValidationRejectsBeforeDiscoveryPreparationOrState(t *testing.T) {
	p := fixtureProvider(t)
	root := t.TempDir()
	request := &pluginv1.ComponentRequest{RootDirectory: root, Address: fixtureAddress(), Operation: &pluginv1.ComponentOperation{Command: "invalid"}}
	validation, err := p.Validate(t.Context(), request)
	require.NoError(t, err)
	require.Len(t, validation.Diagnostics, 1)
	require.Equal(t, "operation.command", validation.Diagnostics[0].Field)
	_, err = p.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = p.ResolveState(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = p.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: root, Operation: request.Operation})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	for _, name := range []string{"discovery.pb", "preparation.pb", "state.pb"} {
		require.NoFileExists(t, filepath.Join(root, name))
	}
}

func TestRPCFailuresAndDeadlineAreSafe(t *testing.T) {
	p := fixtureProvider(t)
	request := &pluginv1.ComponentRequest{RootDirectory: t.TempDir(), Address: fixtureAddress(), Operation: &pluginv1.ComponentOperation{Command: "rpc-error"}}
	_, err := p.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginAPI)
	require.NotContains(t, err.Error(), "secret-value")
	request.Operation.Command = "unsupported"
	_, err = p.ResolveState(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	request.Operation.Command = "wait"
	p.timeout = 50 * time.Millisecond
	_, err = p.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	request.Operation.Command = "wait-prepare"
	p.timeout = 2 * time.Second
	_, err = p.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.FileExists(t, filepath.Join(request.RootDirectory, "preparation.pb"), "preparation must have started")
}

func TestProviderWithoutValidatorStillHasIndependentStateCapability(t *testing.T) {
	items := fixtureActivations(t)
	registry, err := NewRegistry(items[3:])
	require.NoError(t, err)
	p, _ := registry.Get("fixture/hcl")
	require.True(t, p.Supports(extension.ComponentState))
	require.False(t, p.Supports(extension.ComponentValidation))
	result, err := p.ResolveState(t.Context(), &pluginv1.ComponentRequest{RootDirectory: t.TempDir(), Address: fixtureAddress()})
	require.NoError(t, err)
	require.Equal(t, "local", result.State.BackendType)
}

func TestSettingsAndInvalidRequestsFailBeforeStartup(t *testing.T) {
	p := fixtureProvider(t)
	p.executable = filepath.Join(t.TempDir(), "absent")
	for _, value := range []*pluginv1.ComponentValue{
		{},
		wireValue(t, cty.UnknownVal(cty.String)), wireValue(t, cty.NullVal(cty.String)), wireValue(t, cty.True),
		wireValue(t, cty.ObjectVal(map[string]cty.Value{"secret-value": cty.StringVal("secret-value")})),
		{TypeJson: []byte(`"string"`), Msgpack: []byte{0xc1}},
	} {
		_, err := p.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: t.TempDir(), ProviderConfiguration: value})
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		require.NotContains(t, err.Error(), "secret-value")
	}
	for _, request := range []*pluginv1.ComponentRequest{
		nil,
		{},
		{RootDirectory: t.TempDir()},
		{RootDirectory: t.TempDir(), Address: fixtureAddress(), Provider: "different"},
		{RootDirectory: t.TempDir(), Address: &pluginv1.ComponentAddress{Stack: "live", Component: "app", ComponentType: "different"}},
		{RootDirectory: t.TempDir(), Address: fixtureAddress(), ProviderConfiguration: &pluginv1.ComponentValue{}},
	} {
		_, err := p.Prepare(t.Context(), request)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		_, err = p.ResolveState(t.Context(), request)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	_, err := p.Validate(t.Context(), &pluginv1.ComponentRequest{RootDirectory: t.TempDir()})
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol, "a valid request reaches startup")
}

func TestTransportCancellationAndErrorClassification(t *testing.T) {
	t.Parallel()
	require.NoError(t, invocationError(nil))
	require.ErrorIs(t, invocationError(status.Error(codes.Canceled, "secret")), context.Canceled)
	require.ErrorIs(t, invocationError(status.Error(codes.DeadlineExceeded, "secret")), context.DeadlineExceeded)
	require.ErrorIs(t, invocationError(context.Canceled), context.Canceled)
}

func TestDiscoveryDiagnosticContract(t *testing.T) {
	for _, test := range []struct {
		name, domain, reason, file string
		safe                       bool
	}{
		{"inline", "atmos.terragrunt.discovery.v1", "MISSING_TERRAFORM_SOURCE", "live/app/terragrunt.hcl", true},
		{"evaluation", "atmos.terragrunt.discovery.v1", "EVALUATION_FAILED", "live/terragrunt.stack.hcl", true},
		{"source", "atmos.terragrunt.discovery.v1", "NONLOCAL_SOURCE", "live/app/terragrunt.hcl", true},
		{"unknown domain", "secret", "EVALUATION_FAILED", "live/app/terragrunt.hcl", false},
		{"unknown reason", "atmos.terragrunt.discovery.v1", "secret", "live/app/terragrunt.hcl", false},
		{"absolute", "atmos.terragrunt.discovery.v1", "EVALUATION_FAILED", "/secret/terragrunt.hcl", false},
		{"traversal", "atmos.terragrunt.discovery.v1", "EVALUATION_FAILED", "../secret/terragrunt.hcl", false},
		{"control", "atmos.terragrunt.discovery.v1", "EVALUATION_FAILED", "secret\nterragrunt.hcl", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			wire, err := status.New(codes.InvalidArgument, "secret RPC payload").WithDetails(&errdetails.ErrorInfo{Domain: test.domain, Reason: test.reason, Metadata: map[string]string{"file": test.file}})
			require.NoError(t, err)
			// Round-trip the status protobuf as the RPC transport does.
			data, err := proto.Marshal(wire.Proto())
			require.NoError(t, err)
			decoded := wire.Proto()
			require.NoError(t, proto.Unmarshal(data, decoded))
			result := invocationError(status.FromProto(decoded).Err())
			require.ErrorIs(t, result, errUtils.ErrPluginAPI)
			require.NotContains(t, result.Error(), "secret")
			if test.safe {
				require.Contains(t, result.Error(), test.file)
			} else {
				require.Equal(t, errUtils.ErrPluginAPI, result)
			}
		})
	}
}

func TestDiscoveryRejectsInvalidOwnershipIdentityAndProvenance(t *testing.T) {
	t.Parallel()
	p := fixtureProvider(t)
	unit := &pluginv1.DiscoveredComponent{
		Address: fixtureAddress(), Provider: p.name, Engine: "terraform", Source: "live/app.hcl",
		Sources:      []*pluginv1.ComponentSource{{Path: "root.hcl", Kind: pluginv1.ComponentSource_FILE}},
		Dependencies: []*pluginv1.ComponentDependency{{Target: fixtureAddress(), Kind: pluginv1.ComponentDependency_ORDER}},
	}
	require.NoError(t, p.checkDiscovery(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit}}))
	for name, change := range map[string]func(*pluginv1.DiscoveredComponent){
		"no address":         func(v *pluginv1.DiscoveredComponent) { v.Address = nil },
		"empty identity":     func(v *pluginv1.DiscoveredComponent) { v.Address.Component = "" },
		"different type":     func(v *pluginv1.DiscoveredComponent) { v.Address.ComponentType = "different" },
		"different provider": func(v *pluginv1.DiscoveredComponent) { v.Provider = "different" },
		"no engine":          func(v *pluginv1.DiscoveredComponent) { v.Engine = "" },
		"escaping source":    func(v *pluginv1.DiscoveredComponent) { v.Source = "../outside.hcl" },
		"escaping watch":     func(v *pluginv1.DiscoveredComponent) { v.Sources[0].Path = "../outside.hcl" },
		"unknown watch":      func(v *pluginv1.DiscoveredComponent) { v.Sources[0].Kind = pluginv1.ComponentSource_KIND_UNSPECIFIED },
		"nil watch":          func(v *pluginv1.DiscoveredComponent) { v.Sources[0] = nil },
		"unknown dependency": func(v *pluginv1.DiscoveredComponent) {
			v.Dependencies[0].Kind = pluginv1.ComponentDependency_KIND_UNSPECIFIED
		},
		"nil dependency": func(v *pluginv1.DiscoveredComponent) { v.Dependencies[0] = nil },
		"no target":      func(v *pluginv1.DiscoveredComponent) { v.Dependencies[0].Target = nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			item := proto.Clone(unit).(*pluginv1.DiscoveredComponent)
			change(item)
			require.ErrorIs(t, p.checkDiscovery(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{item}}), errUtils.ErrPluginProtocol)
		})
	}
	require.ErrorIs(t, p.checkDiscovery(nil), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, p.checkDiscovery(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{nil}}), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, p.checkDiscovery(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit, unit}}), errUtils.ErrDuplicatePluginExtension)
	// Diagnostics never invalidate the response; the discovery session interprets them per unit.
	require.NoError(t, p.checkDiscovery(&pluginv1.DiscoverComponentsResponse{Diagnostics: []*pluginv1.Diagnostic{{Message: "failure"}}}))
}

func TestPreparationRejectsUnsafeArtifactsAndMissingStateIdentity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	valid := &pluginv1.PrepareComponentResponse{Engine: "terraform", WorkingDirectory: root, ModuleSource: "./module", State: fixtureState(root), Files: []*pluginv1.ComponentGeneratedFile{{Path: "backend.tf", IfExists: "error"}}}
	require.NoError(t, checkPreparation(valid))
	for name, change := range map[string]func(*pluginv1.PrepareComponentResponse){
		"no engine":         func(v *pluginv1.PrepareComponentResponse) { v.Engine = "" },
		"no module":         func(v *pluginv1.PrepareComponentResponse) { v.ModuleSource = "" },
		"no workdir":        func(v *pluginv1.PrepareComponentResponse) { v.WorkingDirectory = "relative" },
		"no state":          func(v *pluginv1.PrepareComponentResponse) { v.State = nil },
		"empty backend":     func(v *pluginv1.PrepareComponentResponse) { v.State.BackendType = "" },
		"empty workspace":   func(v *pluginv1.PrepareComponentResponse) { v.State.Workspace = "" },
		"relative backend":  func(v *pluginv1.PrepareComponentResponse) { v.State.WorkingDirectory = "relative" },
		"escaping artifact": func(v *pluginv1.PrepareComponentResponse) { v.Files[0].Path = "../outside.tf" },
		"absolute artifact": func(v *pluginv1.PrepareComponentResponse) { v.Files[0].Path = filepath.Join(root, "absolute.tf") },
		"unknown ownership": func(v *pluginv1.PrepareComponentResponse) { v.Files[0].IfExists = "unknown" },
		"nil artifact":      func(v *pluginv1.PrepareComponentResponse) { v.Files[0] = nil },
		"duplicate destination": func(v *pluginv1.PrepareComponentResponse) {
			v.Files = append(v.Files, &pluginv1.ComponentGeneratedFile{Path: filepath.Join("sub", "..", "backend.tf"), IfExists: "error"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			value := proto.Clone(valid).(*pluginv1.PrepareComponentResponse)
			change(value)
			require.ErrorIs(t, checkPreparation(value), errUtils.ErrPluginProtocol)
		})
	}
	require.ErrorIs(t, checkPreparation(nil), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, checkPreparation(&pluginv1.PrepareComponentResponse{Diagnostics: []*pluginv1.Diagnostic{{Message: "failure"}}}), errUtils.ErrPluginInvalidConfig)
}

func TestRegistryGroupsCapabilitiesWithoutReplacingEngines(t *testing.T) {
	t.Parallel()
	activations := fixtureActivations(t)
	registry, err := NewRegistry(activations)
	require.NoError(t, err)
	require.Equal(t, []string{"fixture/hcl"}, registry.Names())
	provider, ok := registry.Get("fixture/hcl")
	require.True(t, ok)
	require.Equal(t, "terraform", provider.ComponentType())
	require.True(t, provider.Supports(extension.ComponentState))
	require.False(t, provider.Supports("component.execution"))
	_, ok = registry.Get("terraform")
	require.False(t, ok, "provider names do not occupy the engine namespace")
	activations[0].Description.PluginName = "changed"
	activations[0].Metadata["component_type"] = "changed"
	require.Equal(t, "fixture/components", provider.expected.PluginName)
	require.Equal(t, "terraform", provider.ComponentType())
	names := registry.Names()
	names[0] = "changed"
	require.Equal(t, []string{"fixture/hcl"}, registry.Names())
	_, ok = (*Registry)(nil).Get("anything")
	require.False(t, ok)
	require.Nil(t, (*Registry)(nil).Names())
}

func TestRegistryRejectsPartialAndConflictingBatches(t *testing.T) {
	t.Parallel()
	for name, modify := range map[string]func(*extension.Activation){
		"owner":              func(a *extension.Activation) { a.Package = "other/package" },
		"family":             func(a *extension.Activation) { a.Metadata["component_type"] = "other" },
		"executable":         func(a *extension.Activation) { a.Executable = "other" },
		"timeout":            func(a *extension.Activation) { a.Timeout = time.Second },
		"duplicate":          func(a *extension.Activation) { a.Capability = extension.ComponentDiscovery },
		"bad schema":         func(a *extension.Activation) { a.Metadata["schema"] = "missing.json" },
		"bad type":           func(a *extension.Activation) { a.Metadata["component_type"] = "" },
		"unknown key":        func(a *extension.Activation) { a.Metadata["typo"] = true },
		"unknown capability": func(a *extension.Activation) { a.Capability = "unknown" },
		"missing handshake":  func(a *extension.Activation) { a.Description = nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			items := fixtureActivations(t)
			modify(items[1])
			registry, err := NewRegistry(items)
			require.Error(t, err)
			require.Nil(t, registry, "no partial batch may become visible")
		})
	}
	_, err := NewRegistry([]*extension.Activation{nil})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	empty, err := NewRegistry(nil)
	require.NoError(t, err)
	require.Empty(t, empty.Names())
}

func TestUnsupportedAndInvalidCallsDoNotStartExecutable(t *testing.T) {
	t.Parallel()
	items := fixtureActivations(t)[:1]
	items[0].Executable = filepath.Join(t.TempDir(), "does-not-exist")
	registry, err := NewRegistry(items)
	require.NoError(t, err)
	p, _ := registry.Get("fixture/hcl")
	request := &pluginv1.ComponentRequest{RootDirectory: t.TempDir(), Address: fixtureAddress()}
	_, err = p.ResolveState(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	_, err = p.Validate(t.Context(), nil)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = p.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: "relative"})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = p.Discover(ctx, &pluginv1.DiscoverComponentsRequest{RootDirectory: request.RootDirectory})
	require.ErrorIs(t, err, context.Canceled)
}
