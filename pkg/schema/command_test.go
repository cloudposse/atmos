package schema

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type testConfigWithCommandEnv struct {
	Env []CommandEnv `mapstructure:"env"`
}

func TestCommandEnvDecodeHook_MapValues(t *testing.T) {
	input := map[string]any{
		"env": map[string]any{
			"PATH":         "{{ env \"PWD\" }}/bin:{{ env \"PATH\" }}",
			"GOBIN":        "{{ env \"PWD\" }}/bin",
			"FROM_COMMAND": map[string]any{"valueCommand": "printf value"},
		},
	}

	var result testConfigWithCommandEnv
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       CommandEnvDecodeHook(),
	})
	require.NoError(t, err)

	err = decoder.Decode(input)
	require.NoError(t, err)

	require.Len(t, result.Env, 3)
	assert.Equal(t, CommandEnv{Key: "FROM_COMMAND", ValueCommand: "printf value"}, result.Env[0])
	assert.Equal(t, CommandEnv{Key: "GOBIN", Value: "{{ env \"PWD\" }}/bin"}, result.Env[1])
	assert.Equal(t, CommandEnv{Key: "PATH", Value: "{{ env \"PWD\" }}/bin:{{ env \"PATH\" }}"}, result.Env[2])
}

func TestDecodeCommandEnvMapValueDecodeErrorUsesSentinel(t *testing.T) {
	_, err := decodeCommandEnvMapValue("BROKEN", map[string]any{"value": []string{"not", "a", "string"}})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCommandEnvDecodeFailed))
}

// TestCommandEnvDecodeError_Is verifies the custom Is method on commandEnvDecodeError:
// it matches any target whose Error() string equals the sentinel message (which is how
// ErrCommandEnvDecodeFailed compares equal to itself and to fmt.Errorf-wrapped copies
// carrying the same message), and rejects both nil and non-matching targets.
func TestCommandEnvDecodeError_Is(t *testing.T) {
	sentinel := commandEnvDecodeError{}

	assert.True(t, sentinel.Is(ErrCommandEnvDecodeFailed))
	assert.True(t, sentinel.Is(errors.New(commandEnvDecodeFailedMessage)))
	assert.False(t, sentinel.Is(nil))
	assert.False(t, sentinel.Is(errors.New("some other error")))
	assert.True(t, errors.Is(ErrCommandEnvDecodeFailed, ErrCommandEnvDecodeFailed))
}

// TestCommandEnvDecodeHook_IgnoresWrongTargetOrSourceKind verifies the hook's early-out
// guards: it only converts data when the target type is []CommandEnv, the source Kind
// is Map, and the map is a stringifiable map[string]any.
func TestCommandEnvDecodeHook_IgnoresWrongTargetOrSourceKind(t *testing.T) {
	hook := CommandEnvDecodeHook().(func(reflect.Type, reflect.Type, any) (any, error))

	// Wrong target type: passthrough regardless of source kind.
	out, err := hook(reflect.TypeOf(map[string]any{}), reflect.TypeOf(""), map[string]any{"KEY": "value"})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"KEY": "value"}, out)

	// Correct target type but source is not a Map kind.
	out, err = hook(reflect.TypeOf(""), reflect.TypeOf([]CommandEnv{}), "not-a-map")
	require.NoError(t, err)
	assert.Equal(t, "not-a-map", out)

	// Correct target type, source Kind is Map, but not a map[string]any (e.g. map[int]any).
	badMap := map[int]any{1: "x"}
	out, err = hook(reflect.TypeOf(badMap), reflect.TypeOf([]CommandEnv{}), badMap)
	require.NoError(t, err)
	assert.Equal(t, badMap, out)
}

// TestDecodeCommandEnvMapValue_UnexpectedKind verifies the default branch of
// decodeCommandEnvMapValue returns ErrTaskUnexpectedNodeKind for values that are
// neither a string nor a map[string]any.
func TestDecodeCommandEnvMapValue_UnexpectedKind(t *testing.T) {
	_, err := decodeCommandEnvMapValue("BAD_KEY", 42)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTaskUnexpectedNodeKind))
	assert.Contains(t, err.Error(), "BAD_KEY")
}

func TestCommandArgument_EffectiveProvides(t *testing.T) {
	tests := []struct {
		name string
		arg  CommandArgument
		want string
	}{
		{
			name: "Provides set wins",
			arg:  CommandArgument{Provides: "component", Type: "stack"},
			want: "component",
		},
		{
			name: "falls back to deprecated Type when Provides unset",
			arg:  CommandArgument{Type: "stack"},
			want: "stack",
		},
		{
			name: "empty when neither set",
			arg:  CommandArgument{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.arg.EffectiveProvides())
		})
	}
}

func TestCommandFlag_EffectiveProvides(t *testing.T) {
	tests := []struct {
		name string
		flag CommandFlag
		want string
	}{
		{
			name: "Provides set wins",
			flag: CommandFlag{Provides: "component", SemanticType: "stack"},
			want: "component",
		},
		{
			name: "falls back to deprecated SemanticType when Provides unset",
			flag: CommandFlag{SemanticType: "stack"},
			want: "stack",
		},
		{
			name: "empty when neither set",
			flag: CommandFlag{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.flag.EffectiveProvides())
		})
	}
}

func TestCommandCopyLoaderFields(t *testing.T) {
	src := Command{
		Steps: Tasks{
			{Name: "a", ScriptSource: "/p/a.star", LiteralFields: []string{"script"}},
			{Name: "group", Steps: []WorkflowStep{
				{Name: "child", ScriptSource: "/p/child.star", LiteralFields: []string{"command", "env.GREETING"}},
				{Name: "inner", Steps: []WorkflowStep{{Name: "deep", ScriptSource: "/p/deep.star", LiteralFields: []string{"script"}}}},
			}},
			{Name: "inline"},
		},
		Commands: []Command{{Steps: Tasks{{Name: "sub", ScriptSource: "/p/sub.star", LiteralFields: []string{"command"}}}}},
	}
	// A JSON round trip is what cloneCommand does; it drops both fields.
	encoded, err := json.Marshal(src)
	require.NoError(t, err)
	var clone Command
	require.NoError(t, json.Unmarshal(encoded, &clone))
	require.Empty(t, clone.Steps[0].ScriptSource)
	require.Empty(t, clone.Steps[0].LiteralFields)

	clone.CopyLoaderFields(&src)

	assert.Equal(t, "/p/a.star", clone.Steps[0].ScriptSource)
	assert.Equal(t, []string{"script"}, clone.Steps[0].LiteralFields)
	assert.Equal(t, "/p/child.star", clone.Steps[1].Steps[0].ScriptSource)
	assert.Equal(t, []string{"command", "env.GREETING"}, clone.Steps[1].Steps[0].LiteralFields)
	assert.Equal(t, "/p/deep.star", clone.Steps[1].Steps[1].Steps[0].ScriptSource)
	assert.Equal(t, []string{"script"}, clone.Steps[1].Steps[1].Steps[0].LiteralFields)
	assert.Empty(t, clone.Steps[2].ScriptSource)
	assert.Empty(t, clone.Steps[2].LiteralFields)
	assert.Equal(t, "/p/sub.star", clone.Commands[0].Steps[0].ScriptSource)
	assert.Equal(t, []string{"command"}, clone.Commands[0].Steps[0].LiteralFields)

	t.Run("source to clone isolation", func(t *testing.T) {
		clone.Steps[0].LiteralFields[0] = "mutated"
		assert.Equal(t, []string{"script"}, src.Steps[0].LiteralFields)
	})

	t.Run("mismatched shapes copy only what lines up", func(t *testing.T) {
		short := Command{Steps: Tasks{{Name: "a"}}}
		short.CopyLoaderFields(&src)
		assert.Equal(t, "/p/a.star", short.Steps[0].ScriptSource)
		assert.Equal(t, []string{"script"}, short.Steps[0].LiteralFields)
	})
}

func TestScriptSourceSurvivesStepConversion(t *testing.T) {
	task := Task{Name: "t", Script: "x", ScriptSource: "/p/x.star"}
	step := task.ToWorkflowStep()
	assert.Equal(t, "/p/x.star", step.ScriptSource)
	assert.Equal(t, "/p/x.star", TaskFromWorkflowStep(&step).ScriptSource)
}

func TestScriptSourceDecodesFromMergedCommandConfig(t *testing.T) {
	data := map[string]any{"steps": []any{
		map[string]any{"name": "a", "type": "script", "script": "x", "script_source": "/p/a.star"},
		map[string]any{"name": "group", "type": "parallel", "steps": []any{
			map[string]any{"name": "child", "type": "script", "script": "y", "script_source": "/p/child.star"},
		}},
	}}
	var command Command
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: &command, DecodeHook: TasksDecodeHook(), TagName: "mapstructure",
	})
	require.NoError(t, err)
	require.NoError(t, decoder.Decode(data))

	require.Len(t, command.Steps, 2)
	assert.Equal(t, "/p/a.star", command.Steps[0].ScriptSource)
	require.Len(t, command.Steps[1].Steps, 1)
	assert.Equal(t, "/p/child.star", command.Steps[1].Steps[0].ScriptSource)
}

func TestScriptSourceHasNoYAMLOrJSONKey(t *testing.T) {
	step := WorkflowStep{Name: "s", ScriptSource: "/p/a.star"}
	encoded, err := json.Marshal(step)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "a.star")

	var fromYAML WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte("name: s\nscript_source: /etc/passwd\n"), &fromYAML))
	assert.Empty(t, fromYAML.ScriptSource)
}

func TestLiteralFieldsSurviveStepConversion(t *testing.T) {
	task := Task{Name: "t", Script: "x", LiteralFields: []string{"script", "env.GREETING", "ambient_env.Foo"}}
	step := task.ToWorkflowStep()
	assert.Equal(t, []string{"script", "env.GREETING", "ambient_env.Foo"}, step.LiteralFields)
	assert.Equal(t, []string{"script", "env.GREETING", "ambient_env.Foo"}, TaskFromWorkflowStep(&step).LiteralFields)
}

func TestIsLiteral(t *testing.T) {
	step := WorkflowStep{LiteralFields: []string{"script", "env.Greeting", "ambient_env.Foo"}}
	task := Task{LiteralFields: []string{"command"}}

	tests := []struct {
		name  string
		got   bool
		wants bool
	}{
		{"step script", step.IsLiteral("script"), true},
		{"step command not marked", step.IsLiteral("command"), false},
		{"step env name is not a field", step.IsLiteral("env"), false},
		{"env exact case", step.IsLiteralEnv("Greeting"), true},
		{"env case-insensitive", step.IsLiteralEnv("GREETING"), true},
		{"env other name", step.IsLiteralEnv("OTHER"), false},
		{"ambient exact case", step.IsLiteralEnv("Foo"), true},
		{"ambient other case", step.IsLiteralEnv("FOO"), false},
		{"task command", task.IsLiteral("command"), true},
		{"task script not marked", task.IsLiteral("script"), false},
		{"zero step", (&WorkflowStep{}).IsLiteral("script"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wants, tt.got)
		})
	}
}

func TestRuntimeLiteralMarkersAreNotSerialized(t *testing.T) {
	step := WorkflowStep{Name: "s", LiteralFields: []string{"env.DECLARED", "ambient_env.Ambient"}}
	encodedJSON, err := json.Marshal(step)
	require.NoError(t, err)
	encodedYAML, err := yaml.Marshal(step)
	require.NoError(t, err)
	for _, encoded := range [][]byte{encodedJSON, encodedYAML} {
		assert.NotContains(t, string(encoded), "ambient_env")
		assert.NotContains(t, string(encoded), "literal_fields")
	}
}
