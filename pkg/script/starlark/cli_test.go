package starlark

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestStandaloneCommandReceivesHostInputsAndContext(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "deploy.star")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	deadline, _ := ctx.Deadline()
	calls := 0
	var stdout bytes.Buffer
	result, err := New().Execute(ctx, script.Spec{
		File: &script.File{Path: path, Args: []string{"dev", "--count=3"}}, Stdout: &stdout,
		ParseCommand: func(callCtx context.Context, spec script.CommandSpec) (script.CommandInput, error) {
			calls++
			actualDeadline, hasDeadline := callCtx.Deadline()
			assert.True(t, hasDeadline)
			assert.Equal(t, deadline, actualDeadline)
			assert.Equal(t, "deploy", spec.Name)
			require.Len(t, spec.Args, 1)
			assert.Equal(t, "target", spec.Args[0].Name)
			require.Len(t, spec.Flags, 1)
			assert.Equal(t, "count", spec.Flags[0].GetName())
			return script.CommandInput{Args: map[string]any{"target": "dev"}, Flags: map[string]any{"count": 3}}, nil
		},
		Source: `
def validate(args, flags):
    if flags["count"] < 1:
        fail("count must be positive")
def main(args, flags):
    print("deployed", args["target"])
    return {"target": args["target"], "count": flags["count"], "raw": ctx.args}
output = cli.command(main, name="deploy", args=[cli.arg("target")],
    flags=[cli.flag("count", type="int", default=1)], validate=validate)
`,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.JSONEq(t, `{"target":"dev","count":3,"raw":["dev","--count=3"]}`, result.Value)
	assert.Contains(t, stdout.String(), "deployed dev")
}

func TestCommandIsRestrictedToStandaloneMainThread(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, invoke string
		host         bool
	}{
		{"embedded step", "cli.command(main)", false},
		{"parallel function", "steps.parallel(functions=[child])", true},
		{"parallel task", `steps.parallel(tasks=[steps.task(name="child", function=child)])`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			spec := script.Spec{Source: "def main(args, flags):\n    fail(\"main must not run\")\ndef child():\n    return cli.command(main)\n" + tc.invoke}
			if tc.host {
				spec.File = &script.File{Path: filepath.Join(t.TempDir(), "main.star")}
				spec.ParseCommand = func(context.Context, script.CommandSpec) (script.CommandInput, error) {
					calls++
					return script.CommandInput{}, nil
				}
			}
			_, err := New().Execute(t.Context(), spec)
			require.ErrorContains(t, err, "only in a standalone script's main thread")
			assert.Equal(t, 0, calls)
		})
	}
}

func TestStandaloneCommandHelpAndDryRunAvoidSideEffects(t *testing.T) {
	t.Parallel()
	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "help", true: "dry run"}[dryRun], func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			calls := 0
			// No runner expectation: executing either callback would fail the test.
			runner := NewMockRunner(gomock.NewController(t))
			result, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{
				File:   &script.File{Path: filepath.Join(t.TempDir(), "main.star"), Args: []string{"--help"}},
				DryRun: dryRun, Stdout: &stdout, Stderr: &stderr,
				ParseCommand: func(context.Context, script.CommandSpec) (script.CommandInput, error) {
					calls++
					return script.CommandInput{Help: true}, nil
				},
				Source: `
def validate(args, flags):
    exec.run(["unexpected-validation-process"])
    fail("validation must not run")
def main(args, flags):
    exec.run(["unexpected-main-process"])
    fail("main must not run")
output = cli.command(main, validate=validate)
`,
			})
			require.NoError(t, err)
			if dryRun {
				assert.Equal(t, 0, calls)
				assert.False(t, result.HasOutput)
			} else {
				assert.Equal(t, 1, calls)
				assert.Equal(t, "null", result.Value)
			}
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestStandaloneHostParsingFailureDoesNotRunMain(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	_, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{
		File: &script.File{Path: filepath.Join(t.TempDir(), "main.star")},
		ParseCommand: func(context.Context, script.CommandSpec) (script.CommandInput, error) {
			return script.CommandInput{}, errUtils.ErrStarlarkInvalidArgument
		},
		Source: "def main(args, flags):\n    exec.run([\"unexpected-process\"])\ncli.command(main)",
	})
	require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
}

func TestCommandContextProvidesTypedFlagsAndArguments(t *testing.T) {
	t.Parallel()
	result, err := New().Execute(t.Context(), script.Spec{
		Flags:     map[string]any{"force": true, "count": 2, "region": "east", "zones": []string{"a", "b"}},
		Arguments: map[string]any{"target": "dev", "optional": nil},
		Source: `output = {"flags": ctx.flags, "arguments": ctx.arguments, "env": env,
    "types": [type(ctx.flags["force"]), type(ctx.flags["count"]), type(ctx.flags["region"]), type(ctx.flags["zones"])]}`,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"flags":{"force":true,"count":2,"region":"east","zones":["a","b"]},
"arguments":{"target":"dev","optional":null},"env":{},"types":["bool","int","string","list"]}`, result.Value)
}

func TestCommandContextDefaultsToEmptyDictionaries(t *testing.T) {
	t.Parallel()
	result, err := New().Execute(t.Context(), script.Spec{Source: `output = [ctx.flags, ctx.arguments]`})
	require.NoError(t, err)
	assert.JSONEq(t, `[{},{}]`, result.Value)
}

func TestCommandContextInputsAreDeeplyImmutable(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`ctx.flags["force"] = False`,
		`ctx.arguments["target"] = "prod"`,
		`ctx.flags["zones"].append("changed")`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			flags := map[string]any{"force": true, "zones": []string{"east"}}
			arguments := map[string]any{"target": "dev"}
			_, err := New().Execute(t.Context(), script.Spec{Source: source, Flags: flags, Arguments: arguments})
			require.ErrorContains(t, err, "frozen")
			assert.Equal(t, map[string]any{"force": true, "zones": []string{"east"}}, flags)
			assert.Equal(t, map[string]any{"target": "dev"}, arguments)
		})
	}
}

func TestCommandContextCopiesHostInputs(t *testing.T) {
	t.Parallel()
	flags := map[string]any{"zones": []string{"east", "west"}}
	arguments := map[string]any{"target": "dev"}
	result, err := New().Execute(t.Context(), script.Spec{
		File: &script.File{Path: filepath.Join(t.TempDir(), "main.star")}, Flags: flags, Arguments: arguments,
		ParseCommand: func(context.Context, script.CommandSpec) (script.CommandInput, error) {
			// The context snapshot was created before parsing. Mutating the host
			// inputs now must not alter what the script subsequently observes.
			flags["zones"].([]string)[0] = "changed"
			arguments["target"] = "prod"
			return script.CommandInput{}, nil
		},
		Source: "def main(args, flags):\n    return [ctx.flags, ctx.arguments]\noutput = cli.command(main)",
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"zones":["east","west"]},{"target":"dev"}]`, result.Value)
}

func TestCommandContextRejectsUnsupportedHostInputTypes(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"flags", "arguments"} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			spec := script.Spec{Source: `fail("script must not run")`}
			if input == "flags" {
				spec.Flags = map[string]any{"unsupported": struct{}{}}
			} else {
				spec.Arguments = map[string]any{"unsupported": struct{}{}}
			}
			_, err := New().Execute(t.Context(), spec)
			require.ErrorContains(t, err, "unsupported input type")
		})
	}
}
