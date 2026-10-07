package starlark

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

// A host usage error reaches the user as the host built it: no Starlark prefix, no traceback.
func TestHostUsageErrorIsNotPresentedAsAScriptFailure(t *testing.T) {
	t.Parallel()
	usage := errUtils.Build(fmt.Errorf("%w: missing required argument", errUtils.ErrScriptUsage)).
		WithHint("Run tool --help for usage.").
		WithExitCode(2).
		Err()
	_, err := New().Execute(t.Context(), script.Spec{
		Name: "tool.star", File: &script.File{Path: filepath.Join(t.TempDir(), "tool.star")},
		ParseCommand: func(context.Context, script.CommandSpec) (script.CommandInput, error) {
			return script.CommandInput{}, usage
		},
		Source: "def main(args, flags):\n    pass\ncli.command(main)",
	})
	require.ErrorIs(t, err, errUtils.ErrScriptUsage)
	assert.NotErrorIs(t, err, errUtils.ErrStarlark)
	assert.Equal(t, 2, errUtils.GetExitCode(err))
	assert.Equal(t, "Run tool --help for usage.", cockroach.FlattenHints(err))
	assert.Empty(t, cockroach.FlattenDetails(err), "no traceback")
}

// Failures raised by the script itself keep the Starlark presentation, even in a standalone run.
func TestScriptFailuresStillUseTheStarlarkPresentation(t *testing.T) {
	t.Parallel()
	for name, source := range map[string]string{
		"fail":          "def main(args, flags):\n    fail(\"boom\")\ncli.command(main)",
		"runtime":       "def main(args, flags):\n    return 1 // 0\ncli.command(main)",
		"validate-fail": "def main(args, flags):\n    pass\ndef check(a, f):\n    fail(\"bad input\")\ncli.command(main, validate = check)",
		"toplevel":      "fail(\"before cli.command\")",
		"not-usage":     "x = 1 +",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := New().Execute(t.Context(), script.Spec{
				Name: "tool.star", File: &script.File{Path: filepath.Join(t.TempDir(), "tool.star")},
				ParseCommand: func(context.Context, script.CommandSpec) (script.CommandInput, error) {
					return script.CommandInput{}, nil
				},
				Source: source,
			})
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			assert.NotErrorIs(t, err, errUtils.ErrScriptUsage)
		})
	}
}

// A validate callback that returns False rejects the user's input, not the script: it is reported
// through the host's usage presentation, without a Starlark prefix or a traceback.
func TestRejectedValidationIsPresentedAsAUsageError(t *testing.T) {
	t.Parallel()
	_, err := New().Execute(t.Context(), script.Spec{
		Name: "tool.star", File: &script.File{Path: filepath.Join(t.TempDir(), "tool.star")},
		ParseCommand: func(context.Context, script.CommandSpec) (script.CommandInput, error) {
			return script.CommandInput{Usage: func(cause error) error {
				return errUtils.Build(cause).WithHint("Run tool --help for usage.").WithExitCode(2).Err()
			}}, nil
		},
		Source: "def main(args, flags):\n    pass\ncli.command(main, validate = lambda a, f: False)",
	})
	require.ErrorIs(t, err, errUtils.ErrScriptUsage)
	assert.NotErrorIs(t, err, errUtils.ErrStarlark)
	assert.ErrorContains(t, err, "input validation failed")
	assert.Equal(t, 2, errUtils.GetExitCode(err))
	assert.Equal(t, "Run tool --help for usage.", cockroach.FlattenHints(err))
	assert.Empty(t, cockroach.FlattenDetails(err), "no traceback")
}

func TestOutputHintNamesWhatIsAcceptedAndOmitsNone(t *testing.T) {
	t.Parallel()
	const source = "output = {\"a\": lambda: 1}"
	for name, spec := range map[string]script.Spec{
		"step": {Name: "deploy"},
		"standalone": {Name: "tool.star", ParseCommand: func(context.Context, script.CommandSpec) (script.CommandInput, error) {
			return script.CommandInput{}, nil
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			spec.Source = source
			_, err := New().Execute(t.Context(), spec)
			require.ErrorIs(t, err, errUtils.ErrStarlarkOutputEncode)
			hint := cockroach.FlattenHints(err)
			assert.Contains(t, hint, "Assign a string, number, bool, list, or dict to `output`")
			assert.Contains(t, hint, "leave it unset (or None)")
			assert.Contains(t, hint, spec.Name)
			assert.NotContains(t, hint, "dict, or None", "None is not an encodable value")
		})
	}
}

// A callback that returns None, and a script that never assigns output, both produce no output.
func TestCommandReturningNoneProducesNoOutput(t *testing.T) {
	t.Parallel()
	for name, source := range map[string]string{
		"callback returns None": "def main(args, flags):\n    pass\noutput = cli.command(main)",
		"explicit None":         "output = None",
		"unset":                 "x = 1",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, err := New().Execute(t.Context(), script.Spec{
				Name: "tool.star", File: &script.File{Path: filepath.Join(t.TempDir(), "tool.star")},
				ParseCommand: func(context.Context, script.CommandSpec) (script.CommandInput, error) {
					return script.CommandInput{}, nil
				},
				Source: source,
			})
			require.NoError(t, err)
			assert.False(t, result.HasOutput)
		})
	}
	t.Run("a returned value is still printed", func(t *testing.T) {
		t.Parallel()
		result, err := New().Execute(t.Context(), script.Spec{
			Name: "tool.star", File: &script.File{Path: filepath.Join(t.TempDir(), "tool.star")},
			ParseCommand: func(context.Context, script.CommandSpec) (script.CommandInput, error) {
				return script.CommandInput{}, nil
			},
			Source: "output = cli.command(lambda args, flags: [1, None])",
		})
		require.NoError(t, err)
		assert.True(t, result.HasOutput)
		assert.JSONEq(t, `[1,null]`, result.Value)
	})
}
