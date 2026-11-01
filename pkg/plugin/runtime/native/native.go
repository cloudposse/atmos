// Package native runs native plugins over mutually authenticated go-plugin gRPC.
package native

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/signals"
)

const (
	maxMessage    = 16 << 20
	pluginName    = "atmos"
	brokerService = "atmos.api.v1"
)

var handshake = plugin.HandshakeConfig{ProtocolVersion: 1, MagicCookieKey: "ATMOS_PLUGIN_MAGIC_COOKIE", MagicCookieValue: "atmos-native-plugin-v1"}

// Option configures the native process without inheriting the CLI's secrets.
type (
	Option  func(*options)
	options struct {
		directory   string
		environment []string
		api         *bridge.Server
		expected    *pluginv1.DescribeResponse
		callTimeout time.Duration
	}
)

// WithDirectory sets the plugin process working directory.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithDirectory(directory string) Option { return func(o *options) { o.directory = directory } }

// WithEnvironment supplies explicitly approved process environment entries.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithEnvironment(environment []string) Option {
	return func(o *options) { o.environment = append([]string(nil), environment...) }
}

// WithAPI provides the invocation-scoped Atmos API server.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithAPI(server *bridge.Server) Option { return func(o *options) { o.api = server } }

// Handle owns a native process and its versioned service clients.
type Handle struct {
	Info        pluginv1.InfoClient
	Provider    pluginv1.AuthProviderClient
	Identity    pluginv1.AuthIdentityClient
	Commands    pluginv1.CommandsClient
	Discovery   pluginv1.ComponentDiscoveryClient
	Validation  pluginv1.ComponentValidationClient
	Preparation pluginv1.ComponentPreparationClient
	State       pluginv1.ComponentStateClient
	api         *bridge.Server
	broker      *plugin.GRPCBroker
	process     *plugin.Client
	close       sync.Once
	// command is the plugin process, kept to stop its process group.
	command *exec.Cmd
	// interrupt ends on SIGINT or SIGTERM so in-flight calls stop together with the process.
	interrupt   *interrupt
	forget      func()
	callTimeout time.Duration
}

// Start launches a plugin with gRPC-only transport and ephemeral mutual TLS.
func Start(ctx context.Context, executable string, opts ...Option) (*Handle, error) {
	defer perf.Track(nil, "native.Start")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	settings := options{api: bridge.New()}
	for _, apply := range opts {
		apply(&settings)
	}
	// SIGINT and SIGTERM cancel the plugin's context so Atmos stops it before exiting,
	// rather than dying and leaving the plugin running unsupervised.
	stopped := newInterrupt(ctx)
	command := exec.CommandContext(stopped.ctx, executable)
	configureProcess(command)
	command.Dir, command.Env = settings.directory, settings.environment
	client := newClient(command)
	// The process-level signal handler exits without unwinding, so it runs this first.
	forget := signals.RegisterExitCleanup(func() {
		client.Kill()
		killProcessGroup(command)
	})
	handle, err := dispense(client)
	if err != nil {
		err = startupError(stopped.ctx, err)
		client.Kill()
		killProcessGroup(command)
		forget()
		stopped.stop()
		return nil, err
	}
	handle.process, handle.api, handle.command = client, settings.api, command
	handle.interrupt, handle.forget, handle.callTimeout = stopped, forget, settings.callTimeout
	startup, cancel := context.WithTimeout(stopped.ctx, 15*time.Second)
	defer cancel()
	if err := handle.describe(startup, settings.expected); err != nil {
		handle.Close()
		return nil, startupError(startup, err)
	}
	if err := handle.configure(startup); err != nil {
		handle.Close()
		return nil, startupError(startup, err)
	}
	return handle, nil
}

func newClient(command *exec.Cmd) *plugin.Client {
	return plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: handshake, Plugins: map[string]plugin.Plugin{pluginName: &protocol{}},
		Cmd: command, AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		AutoMTLS: true, SkipHostEnv: true, StartTimeout: 15 * time.Second,
		Logger: hclog.NewNullLogger(), Stderr: io.Discard, SyncStderr: io.Discard, SyncStdout: io.Discard,
		GRPCDialOptions: []grpc.DialOption{grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMessage), grpc.MaxCallSendMsgSize(maxMessage))},
	})
}

// dispense completes the handshake and returns the versioned service clients.
func dispense(client *plugin.Client) (*Handle, error) {
	connection, err := client.Client()
	if err != nil {
		return nil, startError(err)
	}
	instance, err := connection.Dispense(pluginName)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginProtocol, err)
	}
	handle, ok := instance.(*Handle)
	if !ok {
		return nil, errUtils.ErrPluginProtocol
	}
	return handle, nil
}

// startError classifies a failed handshake; go-plugin reports a slow start as plain text.
func startError(err error) error {
	if strings.Contains(err.Error(), "timeout while waiting for plugin to start") {
		return startTimeoutError(err)
	}
	return errors.Join(errUtils.ErrPluginProtocol, err)
}

func startupError(startup context.Context, err error) error {
	if errors.Is(startup.Err(), context.DeadlineExceeded) {
		return startTimeoutError(err)
	}
	return err
}

func (h *Handle) configure(ctx context.Context) error {
	id := h.broker.NextId()
	go h.broker.AcceptAndServe(id, func(options []grpc.ServerOption) *grpc.Server {
		server := grpc.NewServer(append(options, grpc.MaxRecvMsgSize(maxMessage), grpc.MaxSendMsgSize(maxMessage))...)
		h.api.Register(server)
		return server
	})
	_, err := h.Info.Configure(ctx, &pluginv1.ConfigureRequest{BrokerIds: map[string]uint32{brokerService: id}})
	if err != nil {
		return errors.Join(errUtils.ErrPluginProtocol, err)
	}
	return nil
}

// Invoke grants API access for one operation and drains callbacks before returning.
// The operation is cancelled by its caller, by SIGINT or SIGTERM, or by the call timeout;
// in every case the plugin process is stopped rather than left running.
//
//nolint:gocritic // Copy the invocation scope so request context changes cannot alias another call.
func (h *Handle) Invoke(ctx context.Context, scope bridge.Invocation, run func(context.Context, *pluginv1.Invocation) error) error {
	defer perf.Track(nil, "native.Invoke")()

	callCtx, cancel := h.callContext(ctx)
	defer cancel()
	scope.Context = callCtx
	invocation := &pluginv1.Invocation{Id: uuid.NewString(), Attempt: 1, Interactive: scope.Interactive, Deadline: invocationDeadline(callCtx)}
	done, err := h.api.Begin(invocation.Id, scope)
	if err != nil {
		return err
	}
	defer done()
	return h.finishInvocation(callCtx, run(callCtx, invocation))
}

// Close terminates the process, its process group, the broker, and active callback streams.
func (h *Handle) Close() {
	defer perf.Track(nil, "native.Handle.Close")()

	h.close.Do(func() {
		if h.process != nil {
			h.process.Kill()
		}
		killProcessGroup(h.command)
		if h.forget != nil {
			h.forget()
		}
		if h.interrupt != nil {
			h.interrupt.stop()
		}
	})
}

type protocol struct{ plugin.NetRPCUnsupportedPlugin }

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (*protocol) GRPCServer(*plugin.GRPCBroker, *grpc.Server) error {
	return errUtils.ErrPluginProtocol
}

func (*protocol) GRPCClient(_ context.Context, broker *plugin.GRPCBroker, connection *grpc.ClientConn) (any, error) {
	defer perf.Track(nil, "native.protocol.GRPCClient")()

	return &Handle{
		Info: pluginv1.NewInfoClient(connection), Provider: pluginv1.NewAuthProviderClient(connection),
		Identity: pluginv1.NewAuthIdentityClient(connection), Commands: pluginv1.NewCommandsClient(connection),
		Discovery: pluginv1.NewComponentDiscoveryClient(connection), Validation: pluginv1.NewComponentValidationClient(connection),
		Preparation: pluginv1.NewComponentPreparationClient(connection), State: pluginv1.NewComponentStateClient(connection), broker: broker,
	}, nil
}
