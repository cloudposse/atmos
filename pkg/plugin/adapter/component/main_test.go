package component

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// The test executable serves a real native protocol fixture, without shell helpers.
func TestMain(m *testing.M) {
	if os.Getenv("ATMOS_PLUGIN_MAGIC_COOKIE") != "" {
		plugin.Serve(&plugin.ServeConfig{
			HandshakeConfig: plugin.HandshakeConfig{ProtocolVersion: 1, MagicCookieKey: "ATMOS_PLUGIN_MAGIC_COOKIE", MagicCookieValue: "atmos-native-plugin-v1"},
			Plugins:         map[string]plugin.Plugin{"atmos": &fixtureProtocol{}}, GRPCServer: plugin.DefaultGRPCServer,
		})
		return
	}
	os.Exit(m.Run())
}

type fixtureProtocol struct{ plugin.NetRPCUnsupportedPlugin }

func (*fixtureProtocol) GRPCServer(_ *plugin.GRPCBroker, server *grpc.Server) error {
	fixture := &componentFixture{}
	pluginv1.RegisterInfoServer(server, fixture)
	pluginv1.RegisterComponentDiscoveryServer(server, fixture)
	pluginv1.RegisterComponentValidationServer(server, fixture)
	pluginv1.RegisterComponentPreparationServer(server, fixture)
	pluginv1.RegisterComponentStateServer(server, fixture)
	return nil
}

func (*fixtureProtocol) GRPCClient(context.Context, *plugin.GRPCBroker, *grpc.ClientConn) (any, error) {
	return nil, os.ErrInvalid
}

type componentFixture struct {
	pluginv1.UnimplementedInfoServer
	pluginv1.UnimplementedComponentDiscoveryServer
	pluginv1.UnimplementedComponentValidationServer
	pluginv1.UnimplementedComponentPreparationServer
	pluginv1.UnimplementedComponentStateServer
}

func (*componentFixture) Describe(context.Context, *pluginv1.DescribeRequest) (*pluginv1.DescribeResponse, error) {
	raw, err := os.ReadFile("description.pb")
	if err != nil {
		return nil, err
	}
	result := &pluginv1.DescribeResponse{}
	return result, proto.Unmarshal(raw, result)
}

func (*componentFixture) Configure(context.Context, *pluginv1.ConfigureRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (*componentFixture) Validate(ctx context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ValidateResponse, error) {
	switch request.GetOperation().GetCommand() {
	case "invalid":
		return &pluginv1.ValidateResponse{Diagnostics: []*pluginv1.Diagnostic{{Field: "operation.command", Message: "unsupported secret-value"}}}, nil
	case "wait":
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	case "rpc-error":
		return nil, status.Error(codes.Internal, "must-not-disclose-secret-value")
	case "unsupported":
		return nil, status.Error(codes.Unimplemented, "unavailable")
	case "detail-safe", "detail-unsafe":
		return nil, fixtureFailure(request.GetOperation().GetCommand() == "detail-safe")
	case "diagnostic-error":
		return &pluginv1.ValidateResponse{Diagnostics: []*pluginv1.Diagnostic{{Field: "mode", Message: "Bad setting", Detail: &pluginv1.ErrorDetail{Code: "bad", Summary: "Bad setting", ConfigKey: "mode"}}}}, nil
	}
	return &pluginv1.ValidateResponse{}, nil
}

func fixtureFailure(safe bool) error {
	detail := &pluginv1.ErrorDetail{
		Code: "unsupported_setting", Summary: "Unsupported Terragrunt setting", Operation: "prepare", Unit: "vpc", Stack: "live/dev",
		ConfigKey: "terraform_binary", Location: &pluginv1.SourceLocation{File: "live/dev/vpc/terragrunt.hcl", Line: 7, Column: 3}, Hints: []string{"Remove terraform_binary."}, UserSafe: safe,
	}
	if safe {
		detail.Detail = "terraform_binary is not implemented."
	} else {
		detail.Detail = "token ghp_SECRETSECRETSECRET leaked"
	}
	withDetail, err := status.New(codes.Unimplemented, detail.Summary).WithDetails(detail)
	if err != nil {
		return err
	}
	return withDetail.Err()
}

func fixtureAddress() *pluginv1.ComponentAddress {
	return &pluginv1.ComponentAddress{Stack: "live", Component: "dev/app", ComponentType: "terraform"}
}

func fixtureState(root string) *pluginv1.ComponentStateIdentity {
	return &pluginv1.ComponentStateIdentity{BackendType: "local", Workspace: "development", WorkingDirectory: root}
}

func (*componentFixture) Discover(_ context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
	if err := recordCall(request.RootDirectory, "discovery", request); err != nil {
		return nil, err
	}
	return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{{Address: fixtureAddress(), Provider: request.Provider, Engine: "terraform", Source: "live/app/terragrunt.hcl"}}}, nil
}

func (*componentFixture) Prepare(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
	if err := recordCall(request.RootDirectory, "preparation", request); err != nil {
		return nil, err
	}
	return &pluginv1.PrepareComponentResponse{Engine: "terraform", WorkingDirectory: request.RootDirectory, ModuleSource: "./module", Inputs: request.Configuration, State: fixtureState(request.RootDirectory), Files: []*pluginv1.ComponentGeneratedFile{{Path: "backend.tf", Content: []byte("$${literal}\n%{ literal }\n"), IfExists: "error"}}}, nil
}

func (*componentFixture) Resolve(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
	if err := recordCall(request.RootDirectory, "state", request); err != nil {
		return nil, err
	}
	return &pluginv1.ResolveComponentStateResponse{State: fixtureState(request.RootDirectory)}, nil
}

func recordCall(root, method string, request proto.Message) error {
	raw, err := proto.Marshal(request)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, method+".pb"), raw, 0o600)
}
