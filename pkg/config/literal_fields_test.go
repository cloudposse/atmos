package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goyaml "go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels: a rename of the fields these tests rely on must fail the build.
var (
	_ = schema.WorkflowStep{LiteralFields: nil}
	_ = schema.Task{LiteralFields: nil}
)

const literalBlock = "#!/bin/sh\necho \"{{ x }} ${VAR}\"\n"

// loadViperWithPreprocess reads content into Viper the way the loader does, then runs the
// atmos.yaml YAML function preprocessing over it.
func loadViperWithPreprocess(t *testing.T, content string) *viper.Viper {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(strings.NewReader(content)))
	require.NoError(t, preprocessAtmosYamlFunc([]byte(content), v))
	return v
}

func parseTestNode(t *testing.T, content string) *goyaml.Node {
	t.Helper()
	var node goyaml.Node
	require.NoError(t, goyaml.Unmarshal([]byte(content), &node))
	return &node
}

func TestLiteralTagInAtmosYaml(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		key  string
		want any
	}{
		{name: "quoted scalar at the top level", yaml: "key: !literal \"{{ external.email }}\"\n", key: "key", want: "{{ external.email }}"},
		{name: "plain scalar", yaml: "key: !literal ${HOME}/x\n", key: "key", want: "${HOME}/x"},
		{name: "block scalar keeps every byte", yaml: "key: !literal |\n  #!/bin/sh\n  echo \"{{ x }} ${VAR}\"\n", key: "key", want: literalBlock},
		{name: "folded block scalar", yaml: "key: !literal >\n  one {{ a }}\n  two ${B}\n", key: "key", want: "one {{ a }} two ${B}\n"},
		{name: "nested mapping", yaml: "a:\n  b:\n    c: !literal \"{{ deep }}\"\n", key: "a.b.c", want: "{{ deep }}"},
		{name: "inline sequence element", yaml: "list: [!literal \"{{ one }}\", plain]\n", key: "list[0]", want: "{{ one }}"},
		{name: "block sequence element", yaml: "list:\n  - !literal \"{{ one }}\"\n  - plain\n", key: "list[0]", want: "{{ one }}"},
		{name: "number-like value stays a string", yaml: "key: !literal 123\n", key: "key", want: "123"},
		{name: "value of a mapping inside a sequence", yaml: "servers:\n  - name: !literal \"{{ s }}\"\n", key: "servers", want: []any{map[string]any{"name": "{{ s }}"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := loadViperWithPreprocess(t, tt.yaml)
			assert.Equal(t, tt.want, v.Get(tt.key))
		})
	}

	t.Run("sequence value as a whole keeps the literal element", func(t *testing.T) {
		v := loadViperWithPreprocess(t, "list:\n  - !literal \"{{ one }}\"\n  - plain\n")
		assert.Equal(t, []any{"{{ one }}", "plain"}, v.Get("list"))
	})

	t.Run("a typo of the tag is still rejected", func(t *testing.T) {
		v := viper.New()
		err := preprocessAtmosYamlFunc([]byte("key: !literall x\n"), v)
		require.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
	})
}

func TestLiteralTagIsListedAsSupportedInAtmosYaml(t *testing.T) {
	err := unsupportedAtmosYamlTagError("!store", "settings.value")
	require.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
	assert.Contains(t, err.Error(), "!literal")

	v := viper.New()
	err = preprocessAtmosYamlFunc([]byte("key: !store x\n"), v)
	require.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
	assert.Contains(t, err.Error(), "!literal")
}

func TestLiteralTagValueThroughDecodeNode(t *testing.T) {
	value, err := decodeNodeWithYamlFunctionsForFile(parseTestNode(t, "k: !literal |\n  a {{ b }} ${C}\n"), "")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"k": "a {{ b }} ${C}\n"}, value)
}

func TestCommandStepLiteralFieldsAreRecorded(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "atmos.yaml")
	writeTestFile(t, file, "")

	tests := []struct {
		name string
		yaml string
		// path selects the step map from the decoded commands.
		path []int
		want []any
	}{
		{
			name: "script",
			yaml: "commands:\n  - name: c\n    steps:\n      - name: s\n        type: script\n        script: !literal |\n          print(\"{{ x }}\")\n",
			path: []int{0},
			want: []any{"script"},
		},
		{
			name: "command",
			yaml: "commands:\n  - name: c\n    steps:\n      - command: !literal echo \"{{ y }}\"\n",
			path: []int{0},
			want: []any{"command"},
		},
		{
			name: "several fields and env values",
			yaml: "commands:\n  - name: c\n    steps:\n      - type: script\n        interpreter: !literal \"{{ i }}\"\n        working_directory: !literal \"{{ w }}\"\n        env:\n          GREETING: !literal \"{{ g }}\"\n          PLAIN: \"{{ p }}\"\n        script: !literal x\n",
			path: []int{0},
			want: []any{"script", "interpreter", "working_directory", "env.GREETING"},
		},
		{
			name: "nested group step",
			yaml: "commands:\n  - name: c\n    steps:\n      - name: g\n        type: parallel\n        steps:\n          - name: child\n            type: script\n            script: !literal \"{{ z }}\"\n",
			path: []int{0, 0},
			want: []any{"script"},
		},
		{
			name: "step without literal fields gets no key",
			yaml: "commands:\n  - name: c\n    steps:\n      - name: s\n        command: echo \"{{ y }}\"\n",
			path: []int{0},
			want: nil,
		},
		{
			name: "user-written literal_fields is stripped",
			yaml: "commands:\n  - name: c\n    steps:\n      - name: s\n        literal_fields: [command]\n        command: echo \"{{ y }}\"\n",
			path: []int{0},
			want: nil,
		},
		{
			name: "container run step marks its working_directory",
			yaml: "commands:\n  - name: c\n    steps:\n      - name: s\n        type: container\n        action: run\n        working_directory: !literal \"{{ w }}\"\n        with:\n          image: alpine\n          command: echo\n",
			path: []int{0},
			want: []any{"working_directory"},
		},
		{
			name: "container step without an action is a run step",
			yaml: "commands:\n  - name: c\n    steps:\n      - name: s\n        type: container\n        working_directory: !literal \"{{ w }}\"\n        with:\n          image: alpine\n",
			path: []int{0},
			want: []any{"working_directory"},
		},
		{
			name: "container run step without literal fields drops a user-written key",
			yaml: "commands:\n  - name: c\n    steps:\n      - name: s\n        type: container\n        action: run\n        literal_fields: [working_directory]\n        working_directory: \"{{ w }}\"\n",
			path: []int{0},
			want: nil,
		},
		{
			name: "container build step is not a literal-aware step",
			yaml: "commands:\n  - name: c\n    steps:\n      - name: s\n        type: container\n        action: build\n        working_directory: !literal \"{{ w }}\"\n",
			path: []int{0},
			want: nil,
		},
		{
			name: "an arbitrary type is not a literal-aware step",
			yaml: "commands:\n  - name: c\n    steps:\n      - name: s\n        type: sleep\n        working_directory: !literal \"{{ w }}\"\n",
			path: []int{0},
			want: nil,
		},
		{
			name: "user-written literal_fields on a mapping that is not a step is stripped",
			yaml: "commands:\n  - name: c\n    steps:\n      - name: s\n        type: sleep\n        literal_fields: [timeout]\n        timeout: 1s\n",
			path: []int{0},
			want: nil,
		},
		{
			name: "user-written literal_fields is replaced by the loader",
			yaml: "commands:\n  - name: c\n    steps:\n      - name: s\n        literal_fields: [command]\n        script: !literal x\n",
			path: []int{0},
			want: []any{"script"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := decodeTestCommands(t, file, tt.yaml)
			step := stepAt(t, commands, tt.path)
			if tt.want == nil {
				assert.NotContains(t, step, schema.LiteralFieldsKey)
				return
			}
			assert.Equal(t, tt.want, step[schema.LiteralFieldsKey])
		})
	}
}

// stepAt returns the step reached by path: path[0] indexes the first command's steps, and every
// further index descends into the nested `steps` of the step reached so far.
func stepAt(t *testing.T, commands []any, path []int) map[string]any {
	t.Helper()
	command, ok := commands[0].(map[string]any)
	require.True(t, ok)
	steps, ok := command["steps"].([]any)
	require.True(t, ok)
	step, ok := steps[path[0]].(map[string]any)
	require.True(t, ok)
	for _, index := range path[1:] {
		nested, ok := step["steps"].([]any)
		require.True(t, ok)
		step, ok = nested[index].(map[string]any)
		require.True(t, ok)
	}
	return step
}

func TestCommandNonStepMappingsGainNoLiteralFields(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "atmos.yaml")
	writeTestFile(t, file, "")

	commands := decodeTestCommands(t, file, `
commands:
  - name: c
    description: !literal "{{ not a step }}"
    env:
      - key: GREETING
        value: !literal "{{ g }}"
    flags:
      - name: flag
        default: !literal "{{ d }}"
    component_config:
      component: !literal "{{ comp }}"
    steps:
      - name: s
        command: !literal echo "{{ y }}"
        with:
          command: "{{ nested }}"
`)
	command, ok := commands[0].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, command, schema.LiteralFieldsKey)
	assert.Equal(t, "{{ not a step }}", command["description"])

	envList, ok := command["env"].([]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"key": "GREETING", "value": "{{ g }}", schema.LiteralFieldsKey: []any{"value"}}, envList[0],
		"a command-level env value written with !literal is marked")

	flags, ok := command["flags"].([]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"name": "flag", "default": "{{ d }}"}, flags[0])

	assert.Equal(t, map[string]any{"component": "{{ comp }}"}, command["component_config"])

	step := stepAt(t, commands, []int{0})
	assert.Equal(t, []any{"command"}, step[schema.LiteralFieldsKey])
	assert.Equal(t, map[string]any{"command": "{{ nested }}"}, step["with"], "a with block is not a step")
}

func TestLoadedCommandsKeepLiteralFields(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "atmos.yaml"), `base_path: "./"
settings:
  note: !literal "{{ keep }}"
commands:
  - name: c
    steps:
      - name: s
        type: script
        interpreter: starlark
        env:
          GREETING: !literal "{{ g }}"
        script: !literal |
          print("{{ x }}")
      - name: g
        type: parallel
        steps:
          - name: child
            type: script
            interpreter: starlark
            script: !literal |
              print("{{ z }}")
      - name: plain
        command: echo "{{ y }}"
`)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Chdir(t.TempDir())

	atmosConfig, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	var found *schema.Command
	for i := range atmosConfig.Commands {
		if atmosConfig.Commands[i].Name == "c" {
			found = &atmosConfig.Commands[i]
		}
	}
	require.NotNil(t, found)
	require.Len(t, found.Steps, 3)

	assert.Equal(t, "print(\"{{ x }}\")\n", found.Steps[0].Script)
	assert.Equal(t, []string{"script", "env.GREETING"}, found.Steps[0].LiteralFields)
	assert.True(t, found.Steps[0].IsLiteral("script"))
	step := found.Steps[0].ToWorkflowStep()
	assert.True(t, step.IsLiteralEnv("GREETING"))

	require.Len(t, found.Steps[1].Steps, 1)
	assert.Equal(t, "print(\"{{ z }}\")\n", found.Steps[1].Steps[0].Script)
	assert.Equal(t, []string{"script"}, found.Steps[1].Steps[0].LiteralFields)

	assert.Empty(t, found.Steps[2].LiteralFields)
}

func TestCommandLiteralPlainStringStep(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "atmos.yaml")
	writeTestFile(t, file, "")

	commands := decodeTestCommands(t, file, "commands:\n  - name: c\n    steps:\n      - !literal \"echo {{ y }}\"\n      - echo \"{{ z }}\"\n")
	command, ok := commands[0].(map[string]any)
	require.True(t, ok)
	steps, ok := command["steps"].([]any)
	require.True(t, ok)
	require.Len(t, steps, 2)
	assert.Equal(t, map[string]any{"command": "echo {{ y }}", schema.LiteralFieldsKey: []any{"command"}}, steps[0],
		"a literal plain-string step becomes a command step that is not rendered")
	assert.Equal(t, `echo "{{ z }}"`, steps[1], "a plain string without the tag stays a plain string")
}

func TestCommandIncludeRawStepFieldsAreLiteral(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "atmos.yaml")
	writeTestFile(t, file, "")
	writeTestFile(t, filepath.Join(root, "braces.star"), "print(\"{{ not a template }}\")\n")

	tests := []struct {
		name string
		step string
		want []any
	}{
		{name: "include.raw script is used as written", step: "script: !include.raw ./braces.star", want: []any{"script"}},
		{name: "include.raw command is used as written", step: "command: !include.raw ./braces.star", want: []any{"command"}},
		{name: "plain include is still rendered", step: "script: !include ./braces.star", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := decodeTestCommands(t, file, "commands:\n  - name: c\n    steps:\n      - name: s\n        type: script\n        interpreter: starlark\n        "+tt.step+"\n")
			step := firstStep(t, commands)
			if tt.want == nil {
				assert.NotContains(t, step, schema.LiteralFieldsKey)
				return
			}
			assert.Equal(t, tt.want, step[schema.LiteralFieldsKey])
		})
	}
}

func TestCommandLiteralTimeoutAndGroupEnv(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "atmos.yaml")
	writeTestFile(t, file, "")

	commands := decodeTestCommands(t, file, `
commands:
  - name: c
    steps:
      - name: slow
        type: script
        interpreter: starlark
        timeout: !literal "{{ 1 }}s"
        script: print("x")
      - name: fan
        type: parallel
        env:
          KEPT: !literal "{{ k }}"
          RENDERED: "{{ r }}"
        steps:
          - name: child
            type: script
            interpreter: starlark
            script: print("y")
`)
	assert.Equal(t, []any{"timeout"}, stepAt(t, commands, []int{0})[schema.LiteralFieldsKey])
	assert.Equal(t, []any{"env.KEPT"}, stepAt(t, commands, []int{1})[schema.LiteralFieldsKey],
		"a parallel parent marks its own env values")
}

func TestCommandLiteralOnUnsupportedFieldFailsAtLoad(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "atmos.yaml")
	writeTestFile(t, file, "")

	for _, tt := range []struct{ name, field, yaml string }{
		{"when", "when", "        when: !literal \"{{ a }}\"\n"},
		{"output", "output", "        output: !literal raw\n"},
		{"with", "with", "        with:\n          image: !literal \"{{ i }}\"\n"},
		{"other scalar", "level", "        level: !literal info\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractCommandsWithYamlFunctionsForFile(
				[]byte("commands:\n  - name: c\n    steps:\n      - name: s\n        command: echo hi\n"+tt.yaml), file,
			)
			require.ErrorIs(t, err, errUtils.ErrLiteralFieldUnsupported)
			assert.ErrorContains(t, err, tt.field)
			assert.ErrorContains(t, err, `step "s"`)
		})
	}
}

func TestCommandLevelEnvLiterals(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "atmos.yaml")
	writeTestFile(t, file, "")

	commands := decodeTestCommands(t, file, `
commands:
  - name: listed
    env:
      - key: KEPT
        value: !literal "{{ k }}"
      - key: RENDERED
        value: "{{ r }}"
    steps:
      - command: echo hi
  - name: mapped
    env:
      KEPT: !literal "{{ k }}"
      RENDERED: "{{ r }}"
    steps:
      - command: echo hi
`)
	listed, ok := commands[0].(map[string]any)
	require.True(t, ok)
	items, ok := listed["env"].([]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"key": "KEPT", "value": "{{ k }}", schema.LiteralFieldsKey: []any{"value"}}, items[0])
	assert.Equal(t, map[string]any{"key": "RENDERED", "value": "{{ r }}"}, items[1])

	mapped, ok := commands[1].(map[string]any)
	require.True(t, ok)
	envMap, ok := mapped["env"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"value": "{{ k }}", schema.LiteralFieldsKey: []any{"value"}}, envMap["KEPT"])
	assert.Equal(t, "{{ r }}", envMap["RENDERED"])
}

// The global env section of atmos.yaml is not a command's env: its values stay plain strings.
func TestGlobalEnvIsNotRewritten(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "atmos.yaml")
	writeTestFile(t, file, "")
	got, err := decodeNodeWithYamlFunctionsForFile(parseTestNode(t, "env:\n  KEPT: !literal \"{{ k }}\"\n"), file)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"env": map[string]any{"KEPT": "{{ k }}"}}, got)
}

// An argument with neither `required:` nor `default:` is required: leaving both out used to
// resolve to an empty string silently, hiding a missing value until a step misbehaved.
func TestCommandArgumentsWithoutRequiredOrDefaultAreRequired(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "atmos.yaml")
	writeTestFile(t, file, "")

	commands := decodeTestCommands(t, file, `
commands:
  - name: c
    arguments:
      - name: bare
      - name: explicit-optional
        required: false
      - name: explicit-required
        required: true
      - name: defaulted
        default: x
      - name: empty-default
        default: ""
    steps:
      - command: echo hi
`)
	command, ok := commands[0].(map[string]any)
	require.True(t, ok)
	arguments, ok := command["arguments"].([]any)
	require.True(t, ok)
	require.Len(t, arguments, 5)
	assert.Equal(t, map[string]any{"name": "bare", "required": true}, arguments[0])
	assert.Equal(t, map[string]any{"name": "explicit-optional", "required": false}, arguments[1])
	assert.Equal(t, map[string]any{"name": "explicit-required", "required": true}, arguments[2])
	assert.Equal(t, map[string]any{"name": "defaulted", "default": "x"}, arguments[3])
	assert.Equal(t, map[string]any{"name": "empty-default", "default": "", "required": true}, arguments[4],
		"an empty default is no default")
}

// Only custom commands declare arguments this way; other sections keep their own shape.
func TestArgumentsOutsideCommandsAreUntouched(t *testing.T) {
	got, err := decodeNodeWithYamlFunctionsForFile(parseTestNode(t, "toolbox:\n  arguments:\n    - name: bare\n"), "")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"toolbox": map[string]any{"arguments": []any{map[string]any{"name": "bare"}}}}, got)
}
