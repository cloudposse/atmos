package rpc

import (
	"context"
	"io"

	"google.golang.org/grpc"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// maxChunk is the largest output chunk the protocol allows.
const maxChunk = 64 << 10

// maxExit is the largest exit code the protocol allows.
const maxExit = 255

// Runner executes a command and returns its exit code.
type Runner interface {
	Run(ctx context.Context, args []string, settings map[string]any, stdout, stderr io.Writer) int
}

// Commands is the "ssh" command server.
type Commands struct {
	pluginv1.UnimplementedCommandsServer
	runner Runner
}

// NewCommands returns the command server.
func NewCommands(runner Runner) *Commands { return &Commands{runner: runner} }

// Run executes the command, streams its output, and ends with exactly one exit event.
func (c *Commands) Run(request *pluginv1.CommandRequest, stream grpc.ServerStreamingServer[pluginv1.CommandEvent]) error {
	sink := &sink{stream: stream}
	code := c.runner.Run(stream.Context(), request.GetArguments(), request.GetSettings().AsMap(),
		&chunkWriter{sink: sink, stderr: false}, &chunkWriter{sink: sink, stderr: true})
	if sink.err != nil {
		return sink.err
	}
	if code < 0 || code > maxExit {
		code = maxExit
	}
	return stream.Send(&pluginv1.CommandEvent{Event: &pluginv1.CommandEvent_Exit{Exit: &pluginv1.CommandExit{Code: uint32(code)}}})
}

// sink sends output events and remembers the first failure.
type sink struct {
	stream grpc.ServerStreamingServer[pluginv1.CommandEvent]
	err    error
}

// chunkWriter splits writes into protocol-sized, non-empty events.
type chunkWriter struct {
	sink   *sink
	stderr bool
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		n := min(len(p), maxChunk)
		chunk := append([]byte(nil), p[:n]...)
		event := &pluginv1.CommandEvent{Event: &pluginv1.CommandEvent_Stdout{Stdout: chunk}}
		if w.stderr {
			event = &pluginv1.CommandEvent{Event: &pluginv1.CommandEvent_Stderr{Stderr: chunk}}
		}
		if w.sink.err == nil {
			w.sink.err = w.sink.stream.Send(event)
		}
		if w.sink.err != nil {
			return written, w.sink.err
		}
		written += n
		p = p[n:]
	}
	return written, nil
}
