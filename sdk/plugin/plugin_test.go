package plugin

import (
	"context"
	"errors"
	"testing"

	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudposse/atmos/sdk/api"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// brokerHarness exposes the consumer broker to exercise the bidirectional handshake.
type brokerHarness struct{ nativePlugin }

func (*brokerHarness) GRPCClient(_ context.Context, broker *goplugin.GRPCBroker, _ *grpc.ClientConn) (any, error) {
	return broker, nil
}

func TestNativePluginRegistersDeclaredServices(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "information only", true: "all services"}[enabled], func(t *testing.T) {
			t.Parallel()
			server := grpc.NewServer()
			defer server.Stop()
			var boundAPI *api.Client
			native := &nativePlugin{config: Config{Build: func(client *api.Client) Services {
				boundAPI = client
				if !enabled {
					return Services{}
				}
				return Services{
					Provider: &pluginv1.UnimplementedAuthProviderServer{}, Identity: &pluginv1.UnimplementedAuthIdentityServer{}, Commands: &pluginv1.UnimplementedCommandsServer{},
					Discovery: &pluginv1.UnimplementedComponentDiscoveryServer{}, Validation: &pluginv1.UnimplementedComponentValidationServer{},
					Preparation: &pluginv1.UnimplementedComponentPreparationServer{}, State: &pluginv1.UnimplementedComponentStateServer{},
				}
			}}}
			if err := native.GRPCServer(nil, server); err != nil {
				t.Fatal(err)
			}
			services := server.GetServiceInfo()
			for _, name := range []string{"atmos.plugin.v1.AuthProvider", "atmos.plugin.v1.AuthIdentity", "atmos.plugin.v1.Commands", "atmos.plugin.v1.ComponentDiscovery", "atmos.plugin.v1.ComponentValidation", "atmos.plugin.v1.ComponentPreparation", "atmos.plugin.v1.ComponentState"} {
				if _, exists := services[name]; exists != enabled {
					t.Fatalf("service %s registration mismatch: %v", name, services)
				}
			}
			if _, exists := services["atmos.plugin.v1.Info"]; !exists {
				t.Fatal("missing information service")
			}
			if err := boundAPI.Open(t.Context(), "https://example.com"); !errors.Is(err, api.ErrNotConfigured) {
				t.Fatalf("API bound before handshake: %v", err)
			}
			if _, err := native.GRPCClient(t.Context(), nil, nil); !errors.Is(err, errClientUnsupported) {
				t.Fatalf("unsupported client accepted: %v", err)
			}
		})
	}
}

func TestInformationRejectsMissingDescriptionAndBroker(t *testing.T) {
	t.Parallel()
	info := &information{}
	if _, err := info.Describe(t.Context(), &pluginv1.DescribeRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("missing description: %v", err)
	}
	if _, err := info.Configure(t.Context(), &pluginv1.ConfigureRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing broker: %v", err)
	}
}

func TestInformationBindsAPIOnceThroughBroker(t *testing.T) {
	t.Parallel()
	description := &pluginv1.DescribeResponse{PluginName: "example/provider", PluginVersion: "1.0.0"}
	harness := &brokerHarness{nativePlugin: nativePlugin{config: Config{Description: description, Build: func(*api.Client) Services { return Services{} }}}}
	client, server := goplugin.TestPluginGRPCConn(t, false, map[string]goplugin.Plugin{PluginName: harness})
	// Stop the local server first so client.Close cannot race it with a remote Shutdown.
	defer client.Close()
	defer server.Stop()
	raw, err := client.Dispense(PluginName)
	if err != nil {
		t.Fatal(err)
	}
	broker := raw.(*goplugin.GRPCBroker)
	info := pluginv1.NewInfoClient(client.Conn)
	actual, err := info.Describe(t.Context(), &pluginv1.DescribeRequest{})
	if err != nil || actual.GetPluginName() != description.GetPluginName() || actual.GetPluginVersion() != description.GetPluginVersion() {
		t.Fatalf("description: %v %v", actual, err)
	}
	for index := 0; index < 2; index++ {
		id := broker.NextId()
		go broker.AcceptAndServe(id, func(options []grpc.ServerOption) *grpc.Server { return grpc.NewServer(options...) })
		_, err := info.Configure(t.Context(), &pluginv1.ConfigureRequest{BrokerIds: map[string]uint32{BrokerService: id}})
		if index == 0 && err != nil {
			t.Fatal(err)
		}
		if index == 1 && status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("reconfiguration allowed: %v", err)
		}
	}
	if _, err := info.Configure(t.Context(), &pluginv1.ConfigureRequest{BrokerIds: map[string]uint32{BrokerService: 9999}}); status.Code(err) != codes.Unavailable {
		t.Fatalf("missing broker was accepted: %v", err)
	}
}
