package planfile

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/process"
)

const validPlan = `{"format_version":"1.2","planned_values":{},"resource_changes":[
 {"address":"terraform_data.test","type":"terraform_data","change":{
 "actions":["delete","create"],"before":{"input":"sensitive-value"},"after":{},"after_unknown":{"output":true}}}]}`

func newTestGuard(t *testing.T, expression string) (*Guard, *MockRunner) {
	t.Helper()
	var engine policy.Engine
	require.NoError(t, engine.Register(policy.CELEngine, &policy.CEL{}))
	runner := NewMockRunner(gomock.NewController(t))
	return &Guard{Engine: &engine, Show: runner, Rules: []policy.Rule{{
		ID: "test", On: policy.TerraformPlan, Engine: policy.CELEngine, Expression: expression, Effects: policy.Effects{Deny: true},
	}}}, runner
}

func expectShow(t *testing.T, runner *MockRunner, data string) {
	t.Helper()
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		require.Equal(t, []string{"show", "-json"}, spec.Args[:2])
		require.True(t, filepath.IsAbs(spec.Args[2]))
		require.Equal(t, io.Discard, spec.Streams.Stderr)
		_, err := spec.Streams.Stdout.Write([]byte(data))
		require.NoError(t, err)
		return process.Result{Started: true}
	})
}

func TestPlanGatePublish(t *testing.T) {
	for _, exitCode := range []int{0, 2} {
		t.Run(string(rune('0'+exitCode)), func(t *testing.T) {
			directory := t.TempDir()
			guard, runner := newTestGuard(t, "false")
			expectShow(t, runner, validPlan)
			invocation := Invocation{Command: "terraform", Dir: directory, Args: []string{"plan", "-out=reviewed.plan"}}
			result, err := guard.Execute(context.Background(), &invocation, func(args []string) process.Result {
				private := strings.TrimPrefix(args[1], "-out=")
				require.NotEqual(t, filepath.Join(directory, "reviewed.plan"), private)
				require.NoError(t, os.WriteFile(private, []byte("saved binary"), 0o600))
				return process.Result{Started: true, ExitCode: exitCode}
			})
			require.NoError(t, err)
			require.Equal(t, exitCode, result.ExitCode)
			data, err := os.ReadFile(filepath.Join(directory, "reviewed.plan"))
			require.NoError(t, err)
			require.Equal(t, "saved binary", string(data))
			info, err := os.Stat(filepath.Join(directory, "reviewed.plan"))
			require.NoError(t, err)
			require.True(t, info.Mode().IsRegular())
			// Windows exposes read-only attributes, not POSIX owner/group permissions.
			if runtime.GOOS != "windows" {
				require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			}
			assertClean(t, directory)
		})
	}
}

func TestPlanDenialAndStaleArtifacts(t *testing.T) {
	for name, outcome := range map[string]int{"denied": 0, "failed": 1, "canceled": 130} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			guard, runner := newTestGuard(t, `input.resource_changes.exists(r, 'delete' in r.change.actions)`)
			destination := filepath.Join(directory, "old.plan")
			require.NoError(t, os.WriteFile(destination, []byte("previous plan"), 0o600))
			if outcome == 0 {
				expectShow(t, runner, validPlan)
			}
			_, err := guard.Execute(context.Background(), &Invocation{Dir: directory, Args: []string{"plan", "-out", destination}}, func(args []string) process.Result {
				require.NoError(t, os.WriteFile(args[2], []byte("new plan"), 0o600))
				return process.Result{Started: true, ExitCode: outcome}
			})
			require.Error(t, err)
			if outcome == 0 {
				require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			}
			data, readErr := os.ReadFile(destination)
			require.NoError(t, readErr)
			require.Equal(t, "previous plan", string(data))
			assertClean(t, directory)
		})
	}
}

func TestSavedApplyUsesEvaluatedSnapshot(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "input.plan")
	require.NoError(t, os.WriteFile(source, []byte("approved bytes"), 0o600))
	guard, runner := newTestGuard(t, "false")
	var evaluated string
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		evaluated = spec.Args[2]
		data, err := os.ReadFile(evaluated)
		require.NoError(t, err)
		require.Equal(t, "approved bytes", string(data))
		// Replacing the original after extraction must not change what apply reads.
		require.NoError(t, os.WriteFile(source, []byte("different bytes"), 0o600))
		_, err = spec.Streams.Stdout.Write([]byte(validPlan))
		require.NoError(t, err)
		return process.Result{Started: true}
	})
	called := false
	_, err := guard.Execute(context.Background(), &Invocation{Dir: directory, Args: []string{"apply", "-no-color", source}}, func(args []string) process.Result {
		called = true
		require.Equal(t, evaluated, args[2])
		data, err := os.ReadFile(args[2])
		require.NoError(t, err)
		require.Equal(t, "approved bytes", string(data))
		return process.Result{Started: true}
	})
	require.NoError(t, err)
	require.True(t, called)
	assertClean(t, directory)
}

func TestSavedApplyBlocked(t *testing.T) {
	for name, test := range map[string]struct{ expression, data string }{
		"deny":             {"true", validPlan},
		"invalid JSON":     {"false", "not JSON"},
		"state JSON":       {"false", `{"format_version":"1.0","values":{}}`},
		"expression error": {`int(input.resource_changes[0].change.before.input) == 0`, validPlan},
		"missing value":    {`input.missing == 0`, validPlan},
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			source := filepath.Join(directory, "input.plan")
			require.NoError(t, os.WriteFile(source, []byte("binary"), 0o600))
			guard, runner := newTestGuard(t, test.expression)
			expectShow(t, runner, test.data)
			_, err := guard.Execute(context.Background(), &Invocation{Dir: directory, Args: []string{"apply", source}}, func([]string) process.Result {
				t.Fatal("apply must not run")
				return process.Result{}
			})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "sensitive-value")
			assertClean(t, directory)
		})
	}
}

func TestPrivateArtifactMutationBlocked(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "input.plan")
	require.NoError(t, os.WriteFile(source, []byte("original"), 0o600))
	guard, runner := newTestGuard(t, "false")
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		require.NoError(t, os.WriteFile(spec.Args[2], []byte("mutated"), 0o600))
		_, err := spec.Streams.Stdout.Write([]byte(validPlan))
		require.NoError(t, err)
		return process.Result{Started: true}
	})
	_, err := guard.Execute(context.Background(), &Invocation{Dir: directory, Args: []string{"apply", source}}, func([]string) process.Result {
		t.Fatal("mutated plan must not be applied")
		return process.Result{}
	})
	require.ErrorContains(t, err, "changed during evaluation")
	assertClean(t, directory)
}

func TestGuardFailures(t *testing.T) {
	guard, runner := newTestGuard(t, "false")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	noop := func([]string) process.Result { t.Fatal("must not execute"); return process.Result{} }
	_, err := guard.Execute(ctx, &Invocation{}, noop)
	require.ErrorIs(t, err, context.Canceled)
	_, err = (&Guard{}).Execute(context.Background(), &Invocation{}, noop)
	require.Error(t, err)
	_, err = guard.Execute(context.Background(), &Invocation{Args: []string{"destroy"}}, noop)
	require.Error(t, err)
	directory := t.TempDir()
	_, err = guard.Execute(context.Background(), &Invocation{Dir: filepath.Join(directory, "missing"), Args: []string{"apply", "x"}}, noop)
	require.Error(t, err)
	_, err = guard.Execute(context.Background(), &Invocation{Dir: directory, Args: []string{"apply", "missing"}}, noop)
	require.Error(t, err)
	source := filepath.Join(directory, "plan")
	require.NoError(t, os.WriteFile(source, []byte("binary"), 0o600))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Return(process.Result{Started: true, Err: errors.New("secret failure"), ExitCode: 1})
	_, err = guard.Execute(context.Background(), &Invocation{Dir: directory, Args: []string{"apply", source}}, noop)
	require.ErrorContains(t, err, "show -json failed")
	require.NotContains(t, err.Error(), "secret failure")
	assertClean(t, directory)
}

func assertClean(t *testing.T, directory string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(directory, ".atmos-*"))
	require.NoError(t, err)
	require.Empty(t, files)
}
