package native

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// This fixture deliberately supplies no executor. Its component configuration
// services must work independently of the existing command service.
type componentFixture struct {
	pluginv1.UnimplementedComponentDiscoveryServer
	pluginv1.UnimplementedComponentValidationServer
	pluginv1.UnimplementedComponentPreparationServer
	pluginv1.UnimplementedComponentStateServer
}

func registerComponentFixture(server *grpc.Server) {
	fixture := &componentFixture{}
	pluginv1.RegisterComponentDiscoveryServer(server, fixture)
	pluginv1.RegisterComponentValidationServer(server, fixture)
	pluginv1.RegisterComponentPreparationServer(server, fixture)
	pluginv1.RegisterComponentStateServer(server, fixture)
}

func (*componentFixture) Discover(_ context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
	if request.GetInvocation().GetId() == "" || request.GetEffects().GetCommands() {
		return nil, status.Error(codes.InvalidArgument, "discovery requires a scoped, read-only invocation")
	}
	return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{{
		Address:  &pluginv1.ComponentAddress{Stack: "live", Component: "dev/app", ComponentType: "terraform"},
		Provider: request.Provider, Engine: "terraform", Source: "live/dev/app/terragrunt.hcl",
		Sources:      []*pluginv1.ComponentSource{{Path: "root.hcl", Kind: pluginv1.ComponentSource_FILE}, {Path: "modules/app", Kind: pluginv1.ComponentSource_DIRECTORY}},
		Dependencies: []*pluginv1.ComponentDependency{{Target: &pluginv1.ComponentAddress{Stack: "live", Component: "dev/network", ComponentType: "terraform"}, Kind: pluginv1.ComponentDependency_OUTPUT}},
	}}}, nil
}

func (*componentFixture) Validate(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ValidateResponse, error) {
	if request.GetOperation().GetMode() != "native" {
		return &pluginv1.ValidateResponse{Diagnostics: []*pluginv1.Diagnostic{{Field: "operation.mode", Message: "only native mode is supported"}}}, nil
	}
	return &pluginv1.ValidateResponse{}, nil
}

func (*componentFixture) Prepare(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
	if request.GetOperation().GetCommand() == "wait" {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	return &pluginv1.PrepareComponentResponse{
		Engine: "terraform", Inputs: request.Configuration,
		Files: []*pluginv1.ComponentGeneratedFile{{Path: "literal.tf", Content: []byte("$${literal}\n%{ literal }\n"), IfExists: "error"}},
	}, nil
}

func (*componentFixture) Resolve(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
	return &pluginv1.ResolveComponentStateResponse{State: &pluginv1.ComponentStateIdentity{BackendType: "local", Workspace: "development", WorkingDirectory: request.RootDirectory}}, nil
}

func TestComponentServicesAcrossNativeProcess(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	handle, err := Start(t.Context(), executable, WithDirectory(t.TempDir()))
	require.NoError(t, err)
	defer handle.Close()
	root := t.TempDir()
	err = handle.Invoke(t.Context(), bridge.Invocation{Namespace: "fixture/components"}, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		request := &pluginv1.ComponentRequest{Invocation: invocation, Provider: "fixture/hcl", RootDirectory: root, Operation: &pluginv1.ComponentOperation{Command: "plan", Mode: "native"}}
		// State identity is available before discovery or preparation.
		state, err := handle.State.Resolve(ctx, request)
		require.NoError(t, err)
		require.Equal(t, "development", state.State.Workspace)
		require.Equal(t, root, state.State.WorkingDirectory)
		validation, err := handle.Validation.Validate(ctx, request)
		require.NoError(t, err)
		require.Empty(t, validation.Diagnostics)
		testComponentDiscovery(t, ctx, handle, invocation)
		testComponentPreparation(t, ctx, handle, request)
		request.Operation.Mode = "unsupported"
		validation, err = handle.Validation.Validate(ctx, request)
		require.NoError(t, err)
		require.Len(t, validation.Diagnostics, 1)
		require.Equal(t, "operation.mode", validation.Diagnostics[0].Field)
		return nil
	})
	require.NoError(t, err)
}

func testComponentDiscovery(t *testing.T, ctx context.Context, handle *Handle, invocation *pluginv1.Invocation) {
	t.Helper()
	request := &pluginv1.DiscoverComponentsRequest{Invocation: invocation, Provider: "fixture/hcl", Revision: "base", Effects: &pluginv1.ComponentEffects{}}
	result, err := handle.Discovery.Discover(ctx, request)
	require.NoError(t, err)
	require.Len(t, result.Components, 1)
	unit := result.Components[0]
	require.Equal(t, "dev/app", unit.Address.Component)
	require.Equal(t, "terraform", unit.Address.ComponentType)
	require.Len(t, unit.Sources, 2)
	require.Equal(t, "root.hcl", unit.Sources[0].Path)
	require.Equal(t, pluginv1.ComponentSource_DIRECTORY, unit.Sources[1].Kind)
	require.Len(t, unit.Dependencies, 1)
	require.Equal(t, "dev/network", unit.Dependencies[0].Target.Component)
	require.Equal(t, pluginv1.ComponentDependency_OUTPUT, unit.Dependencies[0].Kind)
	request.Effects.Commands = true
	_, err = handle.Discovery.Discover(ctx, request)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func testComponentPreparation(t *testing.T, ctx context.Context, handle *Handle, request *pluginv1.ComponentRequest) {
	t.Helper()
	value, err := cty.ParseNumberVal("9007199254740993123456789.123456789")
	require.NoError(t, err)
	typeJSON, err := ctyjson.MarshalType(value.Type())
	require.NoError(t, err)
	payload, err := msgpack.Marshal(value, value.Type())
	require.NoError(t, err)
	request.Configuration = &pluginv1.ComponentValue{TypeJson: typeJSON, Msgpack: payload, SensitivePaths: []*pluginv1.ComponentValuePath{{}}}
	prepared, err := handle.Preparation.Prepare(ctx, request)
	require.NoError(t, err)
	require.True(t, proto.Equal(request.Configuration, prepared.Inputs))
	require.Len(t, prepared.Files, 1)
	require.Equal(t, []byte("$${literal}\n%{ literal }\n"), prepared.Files[0].Content)
	require.Equal(t, "error", prepared.Files[0].IfExists)
	decoded, err := msgpack.Unmarshal(prepared.Inputs.Msgpack, value.Type())
	require.NoError(t, err)
	require.True(t, value.RawEquals(decoded))
}

func TestComponentPreparationDeadlineStopsPlugin(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	handle, err := Start(t.Context(), executable, WithDirectory(t.TempDir()), WithCallTimeout(50*time.Millisecond))
	require.NoError(t, err)
	defer handle.Close()
	err = handle.Invoke(t.Context(), bridge.Invocation{Namespace: "fixture/deadline"}, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		_, err := handle.Preparation.Prepare(ctx, &pluginv1.ComponentRequest{Invocation: invocation, Operation: &pluginv1.ComponentOperation{Command: "wait"}})
		return err
	})
	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = handle.State.Resolve(t.Context(), &pluginv1.ComponentRequest{})
	require.Equal(t, codes.Canceled, status.Code(err))
}
