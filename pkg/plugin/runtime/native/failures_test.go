package native

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_clients_test.go -package=native github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1 InfoClient,CommandsClient
//go:generate go run go.uber.org/mock/mockgen -destination=mock_writer_test.go -package=native io Writer

func TestMalformedDescriptionRejected(t *testing.T) {
	t.Parallel()
	expected := &pluginv1.DescribeResponse{Exports: []*pluginv1.Export{{Service: "command", Kind: "about"}}, Services: []*pluginv1.ServiceVersionRange{{Service: "command", Min: 1, Max: 1}}}
	require.ErrorIs(t, CheckDescription(nil, expected), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, CheckDescription(expected, nil), errUtils.ErrPluginProtocol)
	for _, export := range []*pluginv1.Export{nil, {}, {Service: "command"}, {Kind: "about"}} {
		invalid := proto.Clone(expected).(*pluginv1.DescribeResponse)
		invalid.Exports = []*pluginv1.Export{export}
		require.ErrorIs(t, CheckDescription(invalid, expected), errUtils.ErrPluginProtocol)
		require.ErrorIs(t, CheckDescription(expected, invalid), errUtils.ErrPluginProtocol)
	}
	for _, services := range [][]*pluginv1.ServiceVersionRange{{nil}, {{}}, {{Service: "command", Min: 0, Max: 1}}, {{Service: "command", Min: 2, Max: 1}}, {expected.Services[0], expected.Services[0]}} {
		invalid := proto.Clone(expected).(*pluginv1.DescribeResponse)
		invalid.Services = services
		require.ErrorIs(t, CheckDescription(expected, invalid), errUtils.ErrPluginProtocol)
	}
}

func TestNativeRejectsCanceledStartupAndInvalidInvocation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	handle, err := Start(ctx, "must-not-start")
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, handle)
	handle = &Handle{api: bridge.New()}
	called := false
	err = handle.Invoke(t.Context(), bridge.Invocation{}, func(context.Context, *pluginv1.Invocation) error { called = true; return nil })
	require.Error(t, err)
	assert.False(t, called)
}

func TestNativeDescriptionAndCommandRPCFailures(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	info := NewMockInfoClient(controller)
	handle := &Handle{Info: info}
	info.EXPECT().Describe(gomock.Any(), gomock.Any()).Return(nil, io.ErrUnexpectedEOF)
	require.ErrorIs(t, handle.describe(t.Context(), &pluginv1.DescribeResponse{}), errUtils.ErrPluginProtocol)
	commands := NewMockCommandsClient(controller)
	handle.Commands = commands
	commands.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil, io.ErrUnexpectedEOF)
	require.ErrorIs(t, handle.Command(t.Context(), &pluginv1.CommandRequest{}, io.Discard, io.Discard), errUtils.ErrPluginProtocol)
}

func TestCommandStreamWritesStderrAndPropagatesSinkFailures(t *testing.T) {
	t.Parallel()
	for _, short := range []bool{false, true} {
		writer := NewMockWriter(gomock.NewController(t))
		if short {
			writer.EXPECT().Write([]byte("failure")).Return(1, nil)
		} else {
			writer.EXPECT().Write([]byte("failure")).Return(0, io.ErrClosedPipe)
		}
		err := receiveCommand(func() (*pluginv1.CommandEvent, error) {
			return &pluginv1.CommandEvent{Event: &pluginv1.CommandEvent_Stderr{Stderr: []byte("failure")}}, nil
		}, io.Discard, writer)
		if short {
			require.ErrorIs(t, err, io.ErrShortWrite)
		} else {
			require.ErrorIs(t, err, io.ErrClosedPipe)
		}
	}
}

func TestStartupDeadlineDuringHandshakeReportsTimeout(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	// Expire the deadline after Start's initial check but before go-plugin starts
	// the process, reproducing a handshake interrupted by its caller's deadline.
	handle, err := Start(ctx, executable, func(*options) { <-ctx.Done() })
	require.Nil(t, handle)
	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotEmpty(t, errUtils.AllHints(err))
}

func TestStartupFailureWithoutDeadlineIsNotTimeout(t *testing.T) {
	handle, err := Start(t.Context(), filepath.Join(t.TempDir(), "missing-plugin"))
	require.Nil(t, handle)
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	require.NotErrorIs(t, err, errUtils.ErrPluginTimeout)
}
