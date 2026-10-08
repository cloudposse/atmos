package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/script"
)

func executeCLI(source string, parse Parser, events *[]string) (starlark.StringDict, error) {
	emit := starlark.NewBuiltin("emit", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		*events = append(*events, string(args[0].(starlark.String)))
		return starlark.None, nil
	})
	return starlark.ExecFileOptions(&syntax.FileOptions{}, &starlark.Thread{}, "command.star", source,
		starlark.StringDict{"cli": New(parse), "emit": emit})
}

func TestCommandUsesNativeDeclarationsAndTypedInputs(t *testing.T) {
	t.Parallel()
	var events []string
	parser := func(_ *starlark.Thread, spec script.CommandSpec) (script.CommandInput, error) {
		events = append(events, "parse")
		assert.Equal(t, "deploy", spec.Name)
		assert.Equal(t, "Deploy one component", spec.Description)
		require.Len(t, spec.Args, 2)
		assert.Equal(t, &flags.PositionalArgSpec{Name: "target", Description: "Target stack", Required: true}, spec.Args[0])
		assert.Equal(t, &flags.PositionalArgSpec{Name: "component", Required: false}, spec.Args[1])
		require.Len(t, spec.Flags, 4)
		assert.Equal(t, &flags.StringFlag{
			Name: "region", Shorthand: "r", Description: "Cloud region",
			Default: "east", EnvVars: []string{"DEPLOY_REGION"}, ValidValues: []string{"east", "west"},
		}, spec.Flags[0])
		assert.Equal(t, &flags.IntFlag{Name: "count", Default: 2, EnvVars: []string{"DEPLOY_COUNT"}}, spec.Flags[1])
		assert.Equal(t, &flags.BoolFlag{Name: "force", Default: true, Shorthand: "f", EnvVars: []string{"DEPLOY_FORCE"}}, spec.Flags[2])
		assert.Equal(t, &flags.StringSliceFlag{
			Name: "zones", Default: []string{"a"},
			EnvVars: []string{"DEPLOY_ZONES"}, ValidValues: []string{"a", "b"},
		}, spec.Flags[3])
		return script.CommandInput{
			Args:  map[string]any{"target": "dev", "component": nil},
			Flags: map[string]any{"region": "west", "count": 4, "force": false, "zones": []string{"a", "b"}},
		}, nil
	}
	globals, err := executeCLI(`
def validate(args, flags):
    emit("validate")
    if args["target"] != "dev" or flags["count"] != 4:
        fail("incorrect typed input")
    return True
def main(args, flags):
    emit("run")
    return [args["target"], args["component"], flags["region"], flags["count"], flags["force"], flags["zones"]]
result = cli.command(run=main, name="deploy", description="Deploy one component", validate=validate,
    args=[cli.arg("target", description="Target stack"), cli.arg("component", required=False)],
    flags=[cli.flag("region", default="east", shorthand="r", description="Cloud region", choices=["east", "west"], env="DEPLOY_REGION"),
           cli.flag("count", type="int", default=2, env="DEPLOY_COUNT"),
           cli.flag("force", type="bool", default=True, shorthand="f", env="DEPLOY_FORCE"),
           cli.flag("zones", type="string_list", default=("a",), choices=("a", "b"), env="DEPLOY_ZONES")])
`, parser, &events)
	require.NoError(t, err)
	assert.Equal(t, `["dev", None, "west", 4, False, ["a", "b"]]`, globals["result"].String())
	assert.Equal(t, []string{"parse", "validate", "run"}, events)
}

func TestFlagDefaultsMatchNativeTypes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind     string
		required bool
		expected flags.Flag
	}{
		{"string", false, &flags.StringFlag{}},
		{"string", true, &flags.StringFlag{}},
		{"int", false, &flags.IntFlag{}},
		{"int", true, &flags.IntFlag{}},
		{"bool", false, &flags.BoolFlag{}},
		{"string_list", false, &flags.StringSliceFlag{}},
		{"string_list", true, &flags.StringSliceFlag{}},
	} {
		t.Run(fmt.Sprintf("%s required=%t", tc.kind, tc.required), func(t *testing.T) {
			t.Parallel()
			var events []string
			parser := func(_ *starlark.Thread, spec script.CommandSpec) (script.CommandInput, error) {
				require.Len(t, spec.Flags, 1)
				actual := spec.Flags[0]
				assert.IsType(t, tc.expected, actual)
				assert.Equal(t, tc.expected.GetDefault(), actual.GetDefault())
				assert.Empty(t, actual.GetEnvVars())
				assert.Equal(t, tc.required, actual.IsRequired())
				return script.CommandInput{}, nil
			}
			required := "False"
			if tc.required {
				required = "True"
			}
			_, err := executeCLI(fmt.Sprintf("def main(args, flags):\n    return None\ncli.command(main, flags=(cli.flag(\"value\", type=%q, required=%s),))", tc.kind, required), parser, &events)
			require.NoError(t, err)
		})
	}
}

func TestInvalidDeclarationsNeverReachHost(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, expression, message string }{
		{"missing argument name", `cli.arg()`, "missing argument"},
		{"invalid argument name", `cli.arg("--target")`, "invalid name"},
		{"argument required type", `cli.arg("target", required="yes")`, "bool"},
		{"missing flag name", `cli.flag()`, "missing argument"},
		{"invalid flag name", `cli.flag("two words")`, "invalid or reserved"},
		{"reserved help", `cli.flag("help")`, "invalid or reserved"},
		{"long shorthand", `cli.flag("target", shorthand="xx")`, "shorthand"},
		{"numeric shorthand", `cli.flag("target", shorthand="1")`, "shorthand"},
		{"reserved shorthand", `cli.flag("target", shorthand="h")`, "shorthand"},
		{"unsupported kind", `cli.flag("target", type="float")`, "unsupported type"},
		{"wrong string default", `cli.flag("target", default=1)`, "must be a string"},
		{"wrong integer default", `cli.flag("target", type="int", default="1")`, "must fit an integer"},
		{"overflow integer", `cli.flag("target", type="int", default=1 << 200)`, "must fit an integer"},
		{"wrong boolean default", `cli.flag("target", type="bool", default="false")`, "must be a bool"},
		{"required default", `cli.flag("target", default="east", required=True)`, "cannot have a default"},
		{"required boolean", `cli.flag("target", type="bool", required=True)`, "cannot be required"},
		{"string list default", `cli.flag("target", type="string_list", default="a")`, "list or tuple"},
		{"nonstring list default", `cli.flag("target", type="string_list", default=[1])`, "expected strings"},
		{"string default outside choices", `cli.flag("stage", default="qa", choices=["dev", "prod"])`, `default "qa" for "stage" is not one of the choices`},
		{"list default outside choices", `cli.flag("stage", type="string_list", default=["dev", "qa"], choices=["dev", "prod"])`, `default "qa" for "stage" is not one of the choices`},
		{"invalid choices type", `cli.flag("target", choices="a")`, "list or tuple"},
		{"invalid choices element", `cli.flag("target", choices=[1])`, "expected strings"},
		{"integer choices", `cli.flag("target", type="int", choices=["1"])`, "choices require"},
		{"unknown flag option", `cli.flag("target", unknown=True)`, "unexpected keyword"},
		{"noncallable main", `cli.command(run=1)`, "callable"},
		{"invalid command name", `cli.command(main, name="two words")`, "invalid name"},
		{"noncallable validation", `cli.command(main, validate=1)`, "validate must be callable"},
		{"nonsequence arguments", `cli.command(main, args=1)`, "list or tuple"},
		{"wrong argument declaration", `cli.command(main, args=[cli.flag("target")])`, "cli.arg declarations"},
		{"duplicate arguments", `cli.command(main, args=[cli.arg("target"), cli.arg("target")])`, "must be unique"},
		{"optional before required", `cli.command(main, args=[cli.arg("first", required=False), cli.arg("second")])`, "required arguments must precede"},
		{"nonsequence flags", `cli.command(main, flags=1)`, "list or tuple"},
		{"wrong flag declaration", `cli.command(main, flags=[cli.arg("target")])`, "cli.flag declarations"},
		{"duplicate flags", `cli.command(main, flags=[cli.flag("target"), cli.flag("target")])`, "must be unique"},
		{"duplicate shorthand", `cli.command(main, flags=[cli.flag("first", shorthand="f"), cli.flag("second", shorthand="f")])`, "must be unique"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var events []string
			_, err := executeCLI("def main(args, flags):\n    emit(\"run\")\n"+tc.expression,
				func(_ *starlark.Thread, _ script.CommandSpec) (script.CommandInput, error) {
					events = append(events, "parse")
					return script.CommandInput{}, nil
				}, &events)
			require.ErrorContains(t, err, tc.message)
			assert.Empty(t, events)
		})
	}
}

func TestCommandCallbacksAndHostFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, validation, main, message string
		input                           script.CommandInput
		parseErr                        error
		events                          []string
		// ended means the script stops at cli.command, after the host printed help.
		ended bool
	}{
		{name: "validation None", validation: "return None", main: `return "ok"`, events: []string{"parse", "validate", "run"}},
		{name: "validation True", validation: "return True", main: `return "ok"`, events: []string{"parse", "validate", "run"}},
		{name: "validation False", validation: "return False", main: `return "ok"`, message: "validation", events: []string{"parse", "validate"}},
		{name: "validation wrong return type", validation: "return 1", main: `return "ok"`, message: "must return None or a bool", events: []string{"parse", "validate"}},
		{name: "validation fails", validation: `fail("bad input")`, main: `return "ok"`, message: "bad input", events: []string{"parse", "validate"}},
		{name: "main fails", validation: "return None", main: `fail("main failed")`, message: "main failed", events: []string{"parse", "validate", "run"}},
		{name: "parse fails", parseErr: errUtils.ErrStarlarkInvalidArgument, validation: "return None", main: `return "ok"`, message: "invalid starlark argument", events: []string{"parse"}},
		{name: "help bypasses all callbacks and ends the script", input: script.CommandInput{Help: true, Flags: map[string]any{"unused": struct{}{}}}, validation: `fail("validation ran")`, main: `fail("main ran")`, events: []string{"parse"}, ended: true},
		{name: "unsupported host argument", input: script.CommandInput{Args: map[string]any{"bad": struct{}{}}}, validation: "return None", main: `return "ok"`, message: "unsupported input type", events: []string{"parse"}},
		{name: "unsupported host flag", input: script.CommandInput{Flags: map[string]any{"bad": float64(1)}}, validation: "return None", main: `return "ok"`, message: "unsupported input type", events: []string{"parse"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var events []string
			source := fmt.Sprintf("def validate(args, flags):\n    emit(\"validate\")\n    %s\ndef main(args, flags):\n    emit(\"run\")\n    %s\nresult = cli.command(main, validate=validate)", tc.validation, tc.main)
			globals, err := executeCLI(source, func(_ *starlark.Thread, _ script.CommandSpec) (script.CommandInput, error) {
				events = append(events, "parse")
				return tc.input, tc.parseErr
			}, &events)
			switch {
			case tc.message != "":
				require.ErrorContains(t, err, tc.message)
			case tc.ended:
				require.ErrorIs(t, err, errUtils.ErrScriptHelpShown)
				assert.NotContains(t, globals, "result", "statements after cli.command must not run")
			default:
				require.NoError(t, err)
				assert.Equal(t, starlark.String("ok"), globals["result"])
			}
			assert.Equal(t, tc.events, events)
		})
	}
}

func TestParsedCommandInputsAreDeeplyImmutable(t *testing.T) {
	t.Parallel()
	for _, mutation := range []string{`args["target"] = "changed"`, `flags["force"] = True`, `flags["zones"].append("changed")`} {
		t.Run(mutation, func(t *testing.T) {
			t.Parallel()
			var events []string
			input := script.CommandInput{Args: map[string]any{"target": "dev"}, Flags: map[string]any{"force": false, "zones": []string{"east"}}}
			_, err := executeCLI("def main(args, flags):\n    "+mutation+"\ncli.command(main)",
				func(_ *starlark.Thread, _ script.CommandSpec) (script.CommandInput, error) { return input, nil }, &events)
			require.ErrorContains(t, err, "frozen")
			assert.Equal(t, map[string]any{"target": "dev"}, input.Args)
			assert.Equal(t, map[string]any{"force": false, "zones": []string{"east"}}, input.Flags)
		})
	}
}

func TestCommandRunsAtMostOnce(t *testing.T) {
	t.Parallel()
	var events []string
	_, err := executeCLI("def main(args, flags):\n    emit(\"run\")\ncli.command(main)\ncli.command(main)",
		func(_ *starlark.Thread, _ script.CommandSpec) (script.CommandInput, error) {
			events = append(events, "parse")
			return script.CommandInput{}, nil
		}, &events)
	require.ErrorContains(t, err, "only once")
	assert.Equal(t, []string{"parse", "run"}, events)
}

func TestDeclarationsHaveReadableTypesAndRejectDictionaryKeys(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"arg", "flag"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			var events []string
			globals, err := executeCLI(fmt.Sprintf("value = cli.%s(\"target\")\nlabel = str(value)\nkind = type(value)\ntruth = bool(value)", kind), nil, &events)
			require.NoError(t, err)
			assert.Equal(t, starlark.String("cli."+kind+"(target)"), globals["label"])
			assert.Equal(t, starlark.String("cli."+kind), globals["kind"])
			assert.Equal(t, starlark.True, globals["truth"])
			_, err = executeCLI(fmt.Sprintf("key = {cli.%s(\"target\"): 1}", kind), nil, &events)
			require.ErrorContains(t, err, "unhashable type: cli."+kind)
		})
	}
}

func TestCommandWithoutHostParserFailsBeforeMain(t *testing.T) {
	t.Parallel()
	var events []string
	_, err := executeCLI("def main(args, flags):\n    emit(\"run\")\ncli.command(main)", nil, &events)
	require.ErrorContains(t, err, "requires a standalone command parser")
	assert.Empty(t, events)
}

// Parsed values are keyed case-insensitively, so names that differ only by case would silently
// lose a value; declaration time is the only place to say so clearly.
func TestCommandRejectsCaseCollidingNames(t *testing.T) {
	t.Parallel()
	for name, declaration := range map[string]string{
		"flags":                    `flags=[cli.flag("Stage"), cli.flag("stage")]`,
		"flags in the other order": `flags=[cli.flag("stage"), cli.flag("STAGE")]`,
		"arguments":                `args=[cli.arg("Service"), cli.arg("service", required=False)]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			parsed := false
			_, err := executeCLI(`cli.command(run=lambda a, f: None, `+declaration+`)`, func(*starlark.Thread, script.CommandSpec) (script.CommandInput, error) {
				parsed = true
				return script.CommandInput{}, nil
			}, new([]string))
			require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
			assert.ErrorContains(t, err, "ignoring case")
			assert.False(t, parsed, "the host parser never sees an ambiguous declaration")
		})
	}
	t.Run("help is reserved regardless of case", func(t *testing.T) {
		t.Parallel()
		_, err := executeCLI(`cli.flag("Help")`, nil, new([]string))
		require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
	})
	t.Run("distinct names are accepted", func(t *testing.T) {
		t.Parallel()
		_, err := executeCLI(`cli.command(run=lambda a, f: None, flags=[cli.flag("stage"), cli.flag("region")], args=[cli.arg("service")])`,
			func(*starlark.Thread, script.CommandSpec) (script.CommandInput, error) {
				return script.CommandInput{}, nil
			}, new([]string))
		require.NoError(t, err)
	})
}

func TestRejectedValidationIsAUsageFailure(t *testing.T) {
	t.Parallel()
	const source = "def validate(args, flags):\n    return False\ndef main(args, flags):\n    emit(\"run\")\ncli.command(main, validate=validate)"

	t.Run("uses the host's usage presentation", func(t *testing.T) {
		t.Parallel()
		var events []string
		presented := errors.New("presented by the host")
		var received error
		_, err := executeCLI(source, func(_ *starlark.Thread, _ script.CommandSpec) (script.CommandInput, error) {
			return script.CommandInput{Usage: func(cause error) error {
				received = cause
				return presented
			}}, nil
		}, &events)
		var usage *script.UsageFailure
		require.ErrorAs(t, err, &usage)
		assert.Same(t, presented, usage.Err, "the host's error is reported unchanged")
		require.ErrorIs(t, received, errUtils.ErrScriptUsage)
		assert.ErrorContains(t, received, "input validation failed")
		assert.Empty(t, events, "main does not run")
	})

	t.Run("without a host presentation it is still a usage error", func(t *testing.T) {
		t.Parallel()
		var events []string
		_, err := executeCLI(source, func(_ *starlark.Thread, _ script.CommandSpec) (script.CommandInput, error) {
			return script.CommandInput{}, nil
		}, &events)
		require.ErrorIs(t, err, errUtils.ErrScriptUsage)
		assert.ErrorContains(t, err, "input validation failed")
	})

	t.Run("fail inside validate stays a script failure", func(t *testing.T) {
		t.Parallel()
		var events []string
		_, err := executeCLI("def validate(args, flags):\n    fail(\"bad\")\ncli.command(lambda a, f: None, validate=validate)",
			func(_ *starlark.Thread, _ script.CommandSpec) (script.CommandInput, error) {
				return script.CommandInput{Usage: func(error) error { return errors.New("must not be used") }}, nil
			}, &events)
		require.Error(t, err)
		assert.NotErrorIs(t, err, errUtils.ErrScriptUsage)
		assert.ErrorContains(t, err, "bad")
	})
}
