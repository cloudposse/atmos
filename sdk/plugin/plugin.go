// Package plugin serves native Atmos plugins over the versioned gRPC protocol.
package plugin

import (
	"context"
	"errors"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/cloudposse/atmos/sdk/api"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

const (
	ProtocolVersion = 1
	PluginName      = "atmos"
	BrokerService   = "atmos.api.v1"
	MaxMessage      = 16 << 20
)

var (
	Handshake            = plugin.HandshakeConfig{ProtocolVersion: ProtocolVersion, MagicCookieKey: "ATMOS_PLUGIN_MAGIC_COOKIE", MagicCookieValue: "atmos-native-plugin-v1"}
	errClientUnsupported = errors.New("plugin author SDK does not create Atmos clients")
)

// Services declares the implementations exported by the plugin.
type Services struct {
	Provider pluginv1.AuthProviderServer
	Identity pluginv1.AuthIdentityServer
	Commands pluginv1.CommandsServer
}

// Config supplies static identity and builds handlers sharing one Atmos API connection.
type Config struct {
	Description *pluginv1.DescribeResponse
	Build       func(*api.Client) Services
}

// Main serves a native plugin until Atmos terminates it. Stdout is reserved for
// the go-plugin handshake; handlers return data through the protocol.
func Main(config Config) {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         map[string]plugin.Plugin{PluginName: &nativePlugin{config: config}},
		GRPCServer: func(options []grpc.ServerOption) *grpc.Server {
			return grpc.NewServer(append(options, grpc.MaxRecvMsgSize(MaxMessage), grpc.MaxSendMsgSize(MaxMessage))...)
		},
	})
}

type nativePlugin struct {
	plugin.NetRPCUnsupportedPlugin
	config Config
}

func (p *nativePlugin) GRPCServer(broker *plugin.GRPCBroker, server *grpc.Server) error {
	client := &api.Client{}
	info := &information{description: p.config.Description, broker: broker, api: client}
	pluginv1.RegisterInfoServer(server, info)
	services := p.config.Build(client)
	if services.Provider != nil {
		pluginv1.RegisterAuthProviderServer(server, services.Provider)
	}
	if services.Identity != nil {
		pluginv1.RegisterAuthIdentityServer(server, services.Identity)
	}
	if services.Commands != nil {
		pluginv1.RegisterCommandsServer(server, services.Commands)
	}
	return nil
}

func (*nativePlugin) GRPCClient(context.Context, *plugin.GRPCBroker, *grpc.ClientConn) (any, error) {
	return nil, errClientUnsupported
}

type information struct {
	pluginv1.UnimplementedInfoServer
	description *pluginv1.DescribeResponse
	broker      *plugin.GRPCBroker
	api         *api.Client
}

func (i *information) Describe(context.Context, *pluginv1.DescribeRequest) (*pluginv1.DescribeResponse, error) {
	if i.description == nil {
		return nil, status.Error(codes.FailedPrecondition, "plugin description missing")
	}
	return i.description, nil
}

func (i *information) Configure(_ context.Context, request *pluginv1.ConfigureRequest) (*emptypb.Empty, error) {
	id, exists := request.GetBrokerIds()[BrokerService]
	if !exists {
		return nil, status.Error(codes.InvalidArgument, "Atmos API broker missing")
	}
	conn, err := i.broker.DialWithOptions(id, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxMessage), grpc.MaxCallSendMsgSize(MaxMessage)))
	if err != nil {
		return nil, status.Error(codes.Unavailable, "Atmos API broker unavailable")
	}
	if err := i.api.Bind(conn); err != nil {
		_ = conn.Close()
		return nil, status.Error(codes.FailedPrecondition, "plugin already configured")
	}
	return &emptypb.Empty{}, nil
}
