package exec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_process_runner_test.go -package=exec github.com/cloudposse/atmos/pkg/process Runner

// useProcessRunner substitutes the subprocess runner for one test.
func useProcessRunner(t *testing.T, runner process.Runner) {
	t.Helper()
	previous := shellProcessRunner
	shellProcessRunner = runner
	t.Cleanup(func() { shellProcessRunner = previous })
}

// The interrupt policy chosen by the caller must reach the subprocess runner unchanged.
func TestExecuteShellCommandPassesTheInterruptPolicy(t *testing.T) {
	tests := []struct {
		name string
		opts []ShellCommandOption
		want process.InterruptPolicy
	}{
		{name: "default terminates on interrupt", want: process.InterruptKill},
		{
			name: "terraform stops gracefully",
			opts: []ShellCommandOption{component.WithGracefulInterrupt()},
			want: process.InterruptGraceful,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewMockRunner(gomock.NewController(t))
			var spec process.TaskSpec
			record := func(_ context.Context, got process.TaskSpec) process.Result {
				spec = got
				return process.Result{Command: got.Command}
			}
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(record)
			useProcessRunner(t, runner)

			err := ExecuteShellCommand(schema.AtmosConfiguration{}, "terraform", []string{"apply"}, t.TempDir(),
				nil, false, "", tt.opts...)
			require.NoError(t, err)
			assert.Equal(t, tt.want, spec.Interrupt)
			assert.Equal(t, "terraform", spec.Command)
			assert.Equal(t, []string{"apply"}, spec.Args)
		})
	}
}
