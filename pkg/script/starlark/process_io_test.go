package starlark

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestExecStdinReplayedOnRetry(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	attempt := 0
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		input, err := io.ReadAll(spec.Streams.Stdin)
		require.NoError(t, err)
		assert.Equal(t, "secret\n\x00café", string(input))
		assert.Empty(t, spec.Args)
		attempt++
		if attempt == 1 {
			return process.Result{Started: true, ExitCode: 1}
		}
		return process.Result{Started: true}
	})
	_, err := runSource(t, `exec.run(["tool"], stdin="secret\n\x00café", retry={"max_attempts":2, "initial_delay":"0s"})`, WithProcessRunner(runner))
	require.NoError(t, err)
}

func TestExecStdinAbsentEmptyAndFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, arg string
		absent    bool
	}{
		{"omitted", "", true}, {"none", ", stdin=None", true}, {"empty", `, stdin=""`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				if tc.absent {
					assert.Nil(t, spec.Streams.Stdin)
				} else {
					input, err := io.ReadAll(spec.Streams.Stdin)
					require.NoError(t, err)
					assert.Empty(t, input)
				}
				return process.Result{Started: true}
			})
			_, err := runSource(t, `exec.run(["tool"]`+tc.arg+`)`, WithProcessRunner(runner))
			require.NoError(t, err)
		})
	}
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Return(process.Result{Started: true, ExitCode: 1})
	var stdout, stderr bytes.Buffer
	_, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{Source: `exec.run(["tool"], stdin="private-payload")`, Stdout: &stdout, Stderr: &stderr})
	require.ErrorIs(t, err, errUtils.ErrStarlarkProcessFailed)
	assert.NotContains(t, err.Error()+stdout.String()+stderr.String(), "private-payload")
	_, err = runSource(t, `exec.run(["tool"], stdin=42)`, WithProcessRunner(NewMockRunner(gomock.NewController(t))))
	require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
}

func TestOSGetenvEffectiveEnvironment(t *testing.T) {
	t.Setenv("ATMOS_TEST_AMBIENT_ONLY", "must-not-leak")
	result, err := New().Execute(t.Context(), script.Spec{
		Env:        map[string]string{"EXPLICIT": "script-only"},
		ProcessEnv: []string{"TOKEN=old", "EMPTY=", "TOKEN=effective", "WITH_EQUALS=a=b"},
		Source:     `output = [os.getenv("TOKEN"), os.getenv("EMPTY", "fallback"), os.getenv("MISSING"), os.getenv("MISSING", "fallback"), os.getenv("WITH_EQUALS"), os.getenv("ATMOS_TEST_AMBIENT_ONLY"), env, dir(os), dir(exec)]`,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `["effective","",null,"fallback","a=b",null,{"EXPLICIT":"script-only"},["arch","chdir","environ","getcwd","getenv","platform","setenv"],["run","which"]]`, result.Value)
	for _, source := range []string{`os.getenv(42)`, `os.getenv("")`, `os.getenv("A=B")`} {
		_, err := runSource(t, source)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "os.getenv")
	}
}

func TestExecStdinKeepsRegisteredSecretsMasked(t *testing.T) {
	const secret = "script-input-secret-e89342"
	iolib.ApplyMaskingConfig(&iolib.Config{DisableMasking: false})
	iolib.RegisterSecret(secret)
	require.NotContains(t, iolib.MaskString(secret), secret)
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		data, err := io.ReadAll(spec.Streams.Stdin)
		require.NoError(t, err)
		assert.Equal(t, secret, string(data))
		_, _ = spec.Streams.Stdout.Write(data)
		_, _ = spec.Streams.Stderr.Write(data)
		return process.Result{Started: true}
	})
	var stdout, stderr bytes.Buffer
	_, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{
		Source: `exec.run(["tool"], stdin="` + secret + `")`, Stdout: iolib.MaskWriter(&stdout), Stderr: iolib.MaskWriter(&stderr),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, stdout.String())
	assert.NotEmpty(t, stderr.String())
	assert.NotContains(t, stdout.String()+stderr.String(), secret)
}
