package native

import (
	"context"
	"errors"
	"io"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

const (
	maxOutputChunk = 64 << 10
	maxExitCode    = 255
)

// Command runs an export and validates its terminal event before accepting its exit status.
// Writers must come from Atmos's masked I/O layer when attached to a terminal.
func (h *Handle) Command(ctx context.Context, request *pluginv1.CommandRequest, stdout, stderr io.Writer) error {
	defer perf.Track(nil, "native.Command")()
	stream, err := h.Commands.Run(ctx, request)
	if err != nil {
		return errors.Join(errUtils.ErrPluginProtocol, err)
	}
	return receiveCommand(stream.Recv, stdout, stderr)
}

func receiveCommand(receive func() (*pluginv1.CommandEvent, error), stdout, stderr io.Writer) error {
	for {
		event, err := receive()
		if err != nil {
			return errors.Join(errUtils.ErrPluginProtocol, err)
		}
		switch value := event.GetEvent().(type) {
		case *pluginv1.CommandEvent_Stdout:
			err = writeChunk(stdout, value.Stdout)
		case *pluginv1.CommandEvent_Stderr:
			err = writeChunk(stderr, value.Stderr)
		case *pluginv1.CommandEvent_Exit:
			return commandExit(receive, value.Exit)
		default:
			return errUtils.ErrPluginProtocol
		}
		if err != nil {
			return err
		}
	}
}

func writeChunk(writer io.Writer, value []byte) error {
	if len(value) == 0 || len(value) > maxOutputChunk {
		return errUtils.ErrPluginProtocol
	}
	count, err := writer.Write(value)
	if err != nil {
		return errors.Join(errUtils.ErrPluginAPI, err)
	}
	if count != len(value) {
		return errors.Join(errUtils.ErrPluginAPI, io.ErrShortWrite)
	}
	return nil
}

func commandExit(receive func() (*pluginv1.CommandEvent, error), result *pluginv1.CommandExit) error {
	if result == nil || result.Code > maxExitCode {
		return errUtils.ErrPluginProtocol
	}
	if _, err := receive(); !errors.Is(err, io.EOF) {
		return errUtils.ErrPluginProtocol
	}
	if result.Code != 0 {
		return errUtils.ExitCodeError{Code: int(result.Code), Silent: true}
	}
	return nil
}
