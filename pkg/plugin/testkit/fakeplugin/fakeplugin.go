// Package fakeplugin lets a Go test binary act as a native Atmos plugin without shell commands
// or platform-specific executables. Call MaybeServe first in TestMain, then launch the test
// binary (os.Executable) as the plugin with its working directory set to a directory that
// WriteMode prepared.
package fakeplugin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// Modes select what the fake plugin's command does.
const (
	// ModeEcho writes "ok" and exits successfully.
	ModeEcho = "echo"
	// ModeHang records its pid and never returns, ignoring cancellation.
	ModeHang = "hang"
	// ModeSpawn is ModeHang plus a descendant process whose pid is recorded as well.
	ModeSpawn = "spawn"

	modeFile        = "fake-plugin-mode"
	pidFile         = "fake-plugin-pids"
	sleepEnv        = "_ATMOS_TEST_FAKE_PLUGIN_SLEEP"
	cookieEnv       = "ATMOS_PLUGIN_MAGIC_COOKIE"
	privateFileMode = 0o600
	sleepLimit      = 10 * time.Minute
)

// WriteMode selects the fake plugin's behavior for processes started in directory.
func WriteMode(directory, mode string) error {
	defer perf.Track(nil, "fakeplugin.WriteMode")()

	return os.WriteFile(filepath.Join(directory, modeFile), []byte(mode), privateFileMode)
}

// PIDs reads the process ids the plugin recorded; the plugin is first and descendants follow.
func PIDs(directory string) ([]int, error) {
	defer perf.Track(nil, "fakeplugin.PIDs")()

	raw, err := os.ReadFile(filepath.Join(directory, pidFile))
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, field := range strings.Fields(string(raw)) {
		pid, err := strconv.Atoi(field)
		if err != nil {
			return nil, err
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// MaybeServe runs the process as a fake plugin (or a long-sleeping helper) when launched as
// one and reports whether it did. Optional registration functions add fixture services.
// A true result means TestMain must return without running tests.
func MaybeServe(register ...func(*grpc.Server)) bool {
	defer perf.Track(nil, "fakeplugin.MaybeServe")()

	if os.Getenv(sleepEnv) == "1" { //nolint:forbidigo // Read the test subprocess handshake before application configuration.
		time.Sleep(sleepLimit)
		return true
	}
	if os.Getenv(cookieEnv) == "" { //nolint:forbidigo // HashiCorp go-plugin owns this process handshake variable.
		return false
	}
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig{ProtocolVersion: 1, MagicCookieKey: cookieEnv, MagicCookieValue: "atmos-native-plugin-v1"},
		Plugins:         map[string]plugin.Plugin{"atmos": &server{register: register}},
		GRPCServer:      plugin.DefaultGRPCServer,
	})
	return true
}

type server struct {
	plugin.NetRPCUnsupportedPlugin
	register []func(*grpc.Server)
}

func (s *server) GRPCServer(_ *plugin.GRPCBroker, grpcServer *grpc.Server) error {
	defer perf.Track(nil, "fakeplugin.GRPCServer")()

	pluginv1.RegisterInfoServer(grpcServer, &info{})
	pluginv1.RegisterCommandsServer(grpcServer, &commands{})
	for _, register := range s.register {
		register(grpcServer)
	}
	return nil
}

func (*server) GRPCClient(context.Context, *plugin.GRPCBroker, *grpc.ClientConn) (any, error) {
	defer perf.Track(nil, "fakeplugin.GRPCClient")()

	return nil, os.ErrInvalid
}

type info struct {
	pluginv1.UnimplementedInfoServer
}

func (*info) Describe(context.Context, *pluginv1.DescribeRequest) (*pluginv1.DescribeResponse, error) {
	defer perf.Track(nil, "fakeplugin.Describe")()

	return &pluginv1.DescribeResponse{}, nil
}

func (*info) Configure(context.Context, *pluginv1.ConfigureRequest) (*emptypb.Empty, error) {
	defer perf.Track(nil, "fakeplugin.Configure")()

	return &emptypb.Empty{}, nil
}

type commands struct {
	pluginv1.UnimplementedCommandsServer
}

func (*commands) Run(_ *pluginv1.CommandRequest, stream grpc.ServerStreamingServer[pluginv1.CommandEvent]) error {
	defer perf.Track(nil, "fakeplugin.Run")()

	raw, _ := os.ReadFile(modeFile)
	mode := strings.TrimSpace(string(raw))
	if mode == ModeEcho || mode == "" {
		if err := stream.Send(&pluginv1.CommandEvent{Event: &pluginv1.CommandEvent_Stdout{Stdout: []byte("ok")}}); err != nil {
			return err
		}
		return stream.Send(&pluginv1.CommandEvent{Event: &pluginv1.CommandEvent_Exit{Exit: &pluginv1.CommandExit{}}})
	}
	pids := strconv.Itoa(os.Getpid())
	if mode == ModeSpawn {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		child := exec.Command(executable)
		child.Env = []string{sleepEnv + "=1"}
		if err := child.Start(); err != nil {
			return err
		}
		pids += "\n" + strconv.Itoa(child.Process.Pid)
	}
	if err := os.WriteFile(pidFile, []byte(pids), privateFileMode); err != nil {
		return err
	}
	time.Sleep(sleepLimit)
	return nil
}
