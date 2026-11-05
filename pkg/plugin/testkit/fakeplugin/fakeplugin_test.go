package fakeplugin

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	goplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func TestFixtureEchoProtocol(t *testing.T) {
	t.Chdir(t.TempDir())
	client, server := goplugin.TestPluginGRPCConn(t, false, map[string]goplugin.Plugin{"atmos": &server{}})
	defer client.Close()
	defer server.Stop()

	info := pluginv1.NewInfoClient(client.Conn)
	description, err := info.Describe(t.Context(), &pluginv1.DescribeRequest{})
	require.NoError(t, err)
	require.Empty(t, description.GetExports())
	_, err = info.Configure(t.Context(), &pluginv1.ConfigureRequest{})
	require.NoError(t, err)

	for _, mode := range []string{"", ModeEcho} {
		t.Run("mode="+mode, func(t *testing.T) {
			require.NoError(t, WriteMode(".", mode))
			stream, err := pluginv1.NewCommandsClient(client.Conn).Run(t.Context(), &pluginv1.CommandRequest{})
			require.NoError(t, err)
			output, err := stream.Recv()
			require.NoError(t, err)
			require.Equal(t, []byte("ok"), output.GetStdout())
			exit, err := stream.Recv()
			require.NoError(t, err)
			require.NotNil(t, exit.GetExit())
			require.Zero(t, exit.GetExit().GetCode())
			_, err = stream.Recv()
			require.ErrorIs(t, err, io.EOF)
		})
	}
}

func TestFixtureAddsServicesWithoutLosingBuiltins(t *testing.T) {
	t.Parallel()
	grpcServer := grpc.NewServer()
	defer grpcServer.Stop()
	fixture := &server{register: []func(*grpc.Server){func(server *grpc.Server) {
		pluginv1.RegisterAuthProviderServer(server, &pluginv1.UnimplementedAuthProviderServer{})
	}}}
	require.NoError(t, fixture.GRPCServer(nil, grpcServer))
	services := grpcServer.GetServiceInfo()
	for _, name := range []string{"atmos.plugin.v1.Info", "atmos.plugin.v1.Commands", "atmos.plugin.v1.AuthProvider"} {
		require.Contains(t, services, name)
	}
	_, err := fixture.GRPCClient(t.Context(), nil, nil)
	require.ErrorIs(t, err, os.ErrInvalid)
}

func TestFixturePIDFileRejectsMalformedRecords(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	_, err := PIDs(directory)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, os.WriteFile(filepath.Join(directory, pidFile), []byte("123\ninvalid\n456"), privateFileMode))
	pids, err := PIDs(directory)
	require.Error(t, err)
	require.Nil(t, pids, "partial process lists must not hide malformed records")
	require.NoError(t, os.WriteFile(filepath.Join(directory, pidFile), []byte("123\n456\n"), privateFileMode))
	pids, err = PIDs(directory)
	require.NoError(t, err)
	require.Equal(t, []int{123, 456}, pids)
	require.ErrorIs(t, WriteMode(filepath.Join(directory, "missing"), ModeEcho), os.ErrNotExist)
}
