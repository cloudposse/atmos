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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

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
