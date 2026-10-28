package shell

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/process"
)

func TestCommandOptionExtraction(t *testing.T) {
	first := &cobra.Command{Use: "plan"}
	last := &cobra.Command{Use: "apply"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer

	for _, tc := range []struct {
		name    string
		opts    []ShellCommandOption
		cmd     *cobra.Command
		ctx     context.Context
		capture bool
	}{
		{name: "defaults", ctx: context.Background()},
		{name: "explicit", opts: []ShellCommandOption{WithInvokingCommand(first), WithProcessContext(ctx), WithProcessStreams(process.Streams{Stdout: &stdout, Stderr: &stderr}), WithExecMetadataOutputCapture(&stdout, &stderr)}, cmd: first, ctx: ctx, capture: true},
		{name: "last option wins", opts: []ShellCommandOption{WithInvokingCommand(first), WithInvokingCommand(last), WithProcessContext(ctx), WithProcessContext(context.TODO())}, cmd: last, ctx: context.TODO()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Same(t, tc.cmd, InvokingCommandFromOpts(tc.opts...))
			assert.Equal(t, tc.ctx, ShellCommandContext(tc.opts...))
			writers := ShellCommandOutputWriters(tc.opts...)
			capturedOut, capturedErr := ExecMetadataOutputCaptureFromOpts(tc.opts...)
			if tc.capture {
				assert.Same(t, &stdout, writers.Stdout)
				assert.Same(t, &stderr, writers.Stderr)
				assert.Same(t, &stdout, capturedOut)
				assert.Same(t, &stderr, capturedErr)
			} else {
				assert.Nil(t, writers.Stdout)
				assert.Nil(t, writers.Stderr)
				assert.Nil(t, capturedOut)
				assert.Nil(t, capturedErr)
			}
		})
	}
	cancel()
	require.ErrorIs(t, ShellCommandContext(WithProcessContext(ctx)).Err(), context.Canceled)
}

func TestEnvVarFromListUsesLastExactKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{name: "absent"},
		{name: "prefix is different key", env: []string{"TF_DATA_DIRECTORY=wrong"}},
		{name: "duplicate", env: []string{"TF_DATA_DIR=old", "OTHER=value", "TF_DATA_DIR=new"}, want: "new"},
		{name: "empty overrides previous", env: []string{"TF_DATA_DIR=old", "TF_DATA_DIR="}},
		{name: "embedded equals", env: []string{"TF_DATA_DIR=path=with=equals"}, want: "path=with=equals"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, EnvVarFromList(tc.env, "TF_DATA_DIR"))
		})
	}
}
