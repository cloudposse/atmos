package native

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func TestCommandStreamContract(t *testing.T) {
	t.Parallel()
	output := func(value string) *pluginv1.CommandEvent {
		return &pluginv1.CommandEvent{Event: &pluginv1.CommandEvent_Stdout{Stdout: []byte(value)}}
	}
	exit := func(code uint32) *pluginv1.CommandEvent {
		return &pluginv1.CommandEvent{Event: &pluginv1.CommandEvent_Exit{Exit: &pluginv1.CommandExit{Code: code}}}
	}
	tests := []struct {
		name     string
		events   []*pluginv1.CommandEvent
		terminal error
		code     int
		invalid  bool
	}{
		{name: "success", events: []*pluginv1.CommandEvent{output("hello\n"), exit(0)}, terminal: io.EOF},
		{name: "exit preserved", events: []*pluginv1.CommandEvent{exit(42)}, terminal: io.EOF, code: 42},
		{name: "missing exit", events: []*pluginv1.CommandEvent{output("partial")}, terminal: io.EOF, invalid: true},
		{name: "duplicate exit", events: []*pluginv1.CommandEvent{exit(0), exit(0)}, terminal: io.EOF, invalid: true},
		{name: "output after exit", events: []*pluginv1.CommandEvent{exit(0), output("late")}, terminal: io.EOF, invalid: true},
		{name: "invalid code", events: []*pluginv1.CommandEvent{exit(256)}, terminal: io.EOF, invalid: true},
		{name: "unknown event", events: []*pluginv1.CommandEvent{{}}, terminal: io.EOF, invalid: true},
		{name: "empty chunk", events: []*pluginv1.CommandEvent{output("")}, terminal: io.EOF, invalid: true},
		{name: "oversize chunk", events: []*pluginv1.CommandEvent{output(string(make([]byte, (64<<10)+1)))}, terminal: io.EOF, invalid: true},
		{name: "canceled", terminal: context.Canceled, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			remaining := tt.events
			err := receiveCommand(func() (*pluginv1.CommandEvent, error) {
				if len(remaining) == 0 {
					return nil, tt.terminal
				}
				value := remaining[0]
				remaining = remaining[1:]
				return value, nil
			}, &stdout, &stderr)
			if tt.invalid {
				require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
				return
			}
			if tt.code != 0 {
				require.Error(t, err)
				assert.Equal(t, tt.code, errUtils.GetExitCode(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "hello\n", stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}
