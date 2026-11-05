package starlark

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestAtmosSecretStdin(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"capture", "stream", "viewport"} {
		for _, exitCode := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/exit%d", mode, exitCode), func(t *testing.T) {
				t.Parallel()
				const payload = "private-payload\n\x00café"
				runner := NewMockRunner(gomock.NewController(t))
				runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
					assert.Equal(t, "atmos-test", spec.Command)
					assert.Equal(t, []string{"secret", "set", "SPACES_SECRET_KEY", "--force", "--stdin"}, spec.Args)
					require.NotNil(t, spec.Streams.Stdin)
					input, err := io.ReadAll(spec.Streams.Stdin)
					require.NoError(t, err)
					assert.Equal(t, payload, string(input))
					_, _ = io.WriteString(spec.Streams.Stdout, "saved")
					_, _ = io.WriteString(spec.Streams.Stderr, "status")
					return process.Result{Started: true, ExitCode: exitCode}
				})
				var stdout, stderr, viewport bytes.Buffer
				viewportCalls := 0
				result, err := New(WithProcessRunner(runner), WithAtmosCommands(testAtmosCatalog()),
					WithAtmosExecutable(func() (string, error) { return "atmos-test", nil })).Execute(t.Context(), script.Spec{
					Source: fmt.Sprintf(`output = atmos.secret("set", "SPACES_SECRET_KEY", flags={"stdin": True, "force": True}, stdin=%q, output=%q).stdout`, payload, mode),
					Stdout: &stdout, Stderr: &stderr,
					Viewport: func(title string, run func(stdout, stderr io.Writer) error) error {
						viewportCalls++
						assert.NotContains(t, title, "private-payload")
						return run(&viewport, &viewport)
					},
				})
				if exitCode != 0 {
					require.ErrorIs(t, err, errUtils.ErrStarlarkProcessFailed)
					assert.NotContains(t, cockroach.FlattenDetails(err)+err.Error(), "private-payload")
				} else {
					require.NoError(t, err)
					assert.Equal(t, "saved", result.Value)
				}
				assert.NotContains(t, stdout.String()+stderr.String()+viewport.String(), "private-payload")
				if mode == "viewport" {
					assert.Equal(t, 1, viewportCalls)
					assert.Equal(t, "savedstatus", viewport.String())
				} else {
					assert.Zero(t, viewportCalls)
				}
				if mode == "stream" {
					assert.Equal(t, "saved", stdout.String())
					assert.Equal(t, "status", stderr.String())
				} else {
					assert.Empty(t, stdout.String())
					assert.Empty(t, stderr.String())
				}
			})
		}
	}
}
