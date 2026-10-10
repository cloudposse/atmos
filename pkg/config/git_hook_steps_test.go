package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/casemap"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.GitHookEntry{Steps: nil}

func TestLoadedGitHookStepsKeepMetadata(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "included.star"), "print(ctx.args)\n")
	writeTestFile(t, filepath.Join(root, "atmos.yaml"), `base_path: "./"
git:
  hooks:
    pre-commit:
      steps:
        - name: literal
          type: script
          interpreter: starlark
          script: !literal 'print("{{ untouched }}")'
          env:
            MESSAGE: !literal '{{ also untouched }}'
        - name: included
          type: script
          interpreter: starlark
          script: !include.raw ./included.star
        - name: forged
          type: script
          interpreter: starlark
          script: 'print("{{ evaluated }}")'
          literal_fields: [script]
          script_source: forged.star
    commit-msg:
      command: echo message
`)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Chdir(t.TempDir())

	config, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	steps := config.Git.Hooks["pre-commit"].Steps
	require.Len(t, steps, 3)
	assert.Equal(t, `print("{{ untouched }}")`, steps[0].Script)
	assert.Equal(t, []string{"script", "env.MESSAGE"}, steps[0].LiteralFields)
	assert.Equal(t, map[string]string{"MESSAGE": "{{ also untouched }}"}, steps[0].Env)
	step := steps[0].ToWorkflowStep()
	assert.True(t, step.IsLiteralEnv("MESSAGE"))
	assert.Equal(t, "print(ctx.args)\n", steps[1].Script)
	assert.Equal(t, filepath.Join(root, "included.star"), steps[1].ScriptSource)
	assert.Empty(t, steps[2].LiteralFields)
	assert.Empty(t, steps[2].ScriptSource)
	assert.Equal(t, "echo message", config.Git.Hooks["commit-msg"].Command)
}

func TestImportedGitHookStepsKeepSource(t *testing.T) {
	setupTestAdapters()
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "fragment.yaml"), `git:
  hooks:
    pre-commit:
      steps:
        - type: script
          interpreter: starlark
          script: !include.raw ./included.star
`)
	writeTestFile(t, filepath.Join(root, "included.star"), "print(ctx.args)\n")
	writeTestFile(t, filepath.Join(root, "atmos.yaml"), `base_path: "./"
import: ["fragment.yaml"]
git:
  hooks:
    commit-msg:
      steps:
        - type: script
          script: !literal '{{ untouched }}'
`)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Chdir(t.TempDir())
	config, err := InitCliConfig(schema.ConfigAndStacksInfo{AtmosBasePath: root}, false)
	require.NoError(t, err)
	steps := config.Git.Hooks["pre-commit"].Steps
	require.Len(t, steps, 1)
	assert.Equal(t, filepath.Join(root, "included.star"), steps[0].ScriptSource)
	assert.Equal(t, "print(ctx.args)\n", steps[0].Script)
	steps = config.Git.Hooks["commit-msg"].Steps
	require.Len(t, steps, 1)
	assert.Equal(t, []string{"script"}, steps[0].LiteralFields)
}

func TestPreprocessGitHookStepsOtherForms(t *testing.T) {
	for _, content := range []string{"", "settings: {}", "git: null", "git: {}", "git: {hooks: null}", "git: {hooks: {pre-commit: {command: echo}}}"} {
		t.Run(content, func(t *testing.T) {
			v := viper.New()
			v.SetConfigType("yaml")
			require.NoError(t, v.ReadConfig(strings.NewReader(content)))
			err := preprocessAtmosYamlFuncExceptCommands([]byte(content), v, "")
			require.NoError(t, err)
		})
	}
	err := preprocessAtmosYamlFuncExceptCommands([]byte("[invalid"), viper.New(), filepath.Join(t.TempDir(), "atmos.yaml"))
	require.ErrorContains(t, err, "did not find expected")
	err = preprocessAtmosYamlFuncExceptCommands([]byte("git: {hooks: {pre-commit: {steps: [{script: !include.raw ./missing.star}]}}}"), viper.New(), filepath.Join(t.TempDir(), "atmos.yaml"))
	require.ErrorIs(t, err, ErrExecuteYamlFunctions)
	require.ErrorContains(t, err, "references a file that does not exist")
}

// Viper lowercases map keys while merging configuration, so env step vars and step output names
// must be restored to their authored case or later steps cannot see SHARED_VAR or myOutput.
func TestLoadedGitHookStepsKeepVarsAndOutputsCase(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "atmos.yaml"), `base_path: "./"
git:
  hooks:
    pre-commit:
      steps:
        - name: setenv
          type: env
          vars:
            SHARED_VAR: shared
        - name: produce
          type: shell
          command: echo hi
          outputs:
            myOutput: '{{ .value }}'
        - name: group
          type: shell
          command: echo group
          steps:
            - name: nested
              type: env
              vars:
                Nested_Var: nested
`)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Chdir(t.TempDir())

	config, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	steps := config.Git.Hooks["pre-commit"].Steps
	require.Len(t, steps, 3)
	assert.Equal(t, map[string]string{"SHARED_VAR": "shared"}, steps[0].Vars)
	assert.Equal(t, map[string]string{"myOutput": "{{ .value }}"}, steps[1].Outputs)
	require.Len(t, steps[2].Steps, 1)
	assert.Equal(t, map[string]string{"Nested_Var": "nested"}, steps[2].Steps[0].Vars)
}

func TestMergeRecursiveStepCaseKeys(t *testing.T) {
	t.Run("invalid YAML and unrelated documents record nothing", func(t *testing.T) {
		caseMaps := casemap.New()
		mergeRecursiveStepCaseKeys([]byte("[invalid"), caseMaps)
		mergeRecursiveStepCaseKeys([]byte("git: {hooks: {h: {command: echo}}}"), caseMaps)
		assert.Nil(t, caseMaps.Get(stepVarsCaseKey))
		assert.Nil(t, caseMaps.Get(stepOutputsCaseKey))
	})
	t.Run("vars and outputs are kept apart and accumulate across files", func(t *testing.T) {
		caseMaps := casemap.New()
		mergeRecursiveStepCaseKeys([]byte("git: {hooks: {h: {steps: [{vars: {AbC: x}, outputs: {OutX: y}}]}}}"), caseMaps)
		mergeRecursiveStepCaseKeys([]byte("commands: [{name: c, steps: [{vars: {Other: x}}]}]"), caseMaps)
		assert.Equal(t, casemap.CaseMap{"abc": "AbC", "other": "Other"}, caseMaps.Get(stepVarsCaseKey))
		assert.Equal(t, casemap.CaseMap{"outx": "OutX"}, caseMaps.Get(stepOutputsCaseKey))
	})
}

// atmos.yaml is merged after its atmos.d fragments, so it can redefine a hook a fragment defined
// with `steps:`. Setting the decoded steps on Viper's override layer used to shadow atmos.yaml, so
// a hook could not be switched to `command:` (or to different steps) by a later source.
func TestLaterSourceSwitchesGitHookFromStepsToCommand(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "atmos.d", "hooks.yaml"), `git:
  hooks:
    pre-commit:
      steps:
        - name: first
          type: shell
          command: echo from-steps
    commit-msg:
      steps:
        - name: kept
          type: shell
          command: echo kept
`)
	writeTestFile(t, filepath.Join(root, "atmos.yaml"), `base_path: "./"
git:
  hooks:
    pre-commit:
      command: echo from-command
`)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Chdir(t.TempDir())

	config, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	hook := config.Git.Hooks["pre-commit"]
	assert.Equal(t, "echo from-command", hook.Command)
	assert.Empty(t, hook.Steps, "the later definition replaces the steps instead of running both")
	require.Len(t, config.Git.Hooks["commit-msg"].Steps, 1, "hooks the later source does not mention are untouched")
}

// The reverse switch: a later source defines steps for a hook an earlier source gave a command.
func TestLaterSourceSwitchesGitHookFromCommandToSteps(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "atmos.d", "hooks.yaml"), `git:
  hooks:
    pre-commit:
      command: echo from-command
`)
	writeTestFile(t, filepath.Join(root, "atmos.yaml"), `base_path: "./"
git:
  hooks:
    pre-commit:
      steps:
        - name: only
          type: shell
          command: echo from-steps
`)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Chdir(t.TempDir())

	config, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	hook := config.Git.Hooks["pre-commit"]
	assert.Empty(t, hook.Command)
	require.Len(t, hook.Steps, 1)
	assert.Equal(t, "only", hook.Steps[0].Name)
}

// A later source that defines steps for the same hook replaces the earlier steps.
func TestLaterSourceReplacesGitHookSteps(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "atmos.d", "hooks.yaml"), `git:
  hooks:
    pre-commit:
      steps:
        - name: first
          type: shell
          command: echo one
        - name: second
          type: shell
          command: echo two
`)
	writeTestFile(t, filepath.Join(root, "atmos.yaml"), `base_path: "./"
git:
  hooks:
    pre-commit:
      steps:
        - name: only
          type: shell
          command: echo replaced
`)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Chdir(t.TempDir())

	config, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	steps := config.Git.Hooks["pre-commit"].Steps
	require.Len(t, steps, 1)
	assert.Equal(t, "only", steps[0].Name)
}

func TestStarlarkTagIsRejectedInAtmosYaml(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "atmos.yaml"), `base_path: "./"
settings:
  note: !starlark |
    return "computed"
`)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Chdir(t.TempDir())

	_, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.ErrorIs(t, err, errUtils.ErrStarlarkUnsupportedInConfig)
	assert.Contains(t, err.Error(), "atmos.yaml")
	assert.Contains(t, err.Error(), "settings.note")
}

func TestStarlarkTagIsRejectedWhereverItAppears(t *testing.T) {
	for name, content := range map[string]string{
		"nested mapping":    "a:\n  b:\n    c: !starlark return 1\n",
		"list item":         "list:\n  - one\n  - !starlark return 2\n",
		"git hook step env": "git:\n  hooks:\n    pre-commit:\n      steps:\n        - type: shell\n          command: x\n          env:\n            A: !starlark return 3\n",
	} {
		t.Run(name, func(t *testing.T) {
			v := viper.New()
			v.SetConfigType("yaml")
			err := preprocessAtmosYamlFuncExceptCommands([]byte(content), v, "profile.yaml")
			require.ErrorIs(t, err, errUtils.ErrStarlarkUnsupportedInConfig)
			assert.Contains(t, err.Error(), "profile.yaml")
		})
	}
	t.Run("a quoted string that starts with the tag is data", func(t *testing.T) {
		v := viper.New()
		v.SetConfigType("yaml")
		err := preprocessAtmosYamlFuncExceptCommands([]byte(`note: "!starlark return 1"`), v, "atmos.yaml")
		require.NoError(t, err)
	})
}

func TestConfigAnchorsAcrossCommandsGitHooksAndSettings(t *testing.T) {
	tests := []struct{ name, content string }{
		{"command anchors used by hooks", `commands:
  - name: &command_name shared-command
    steps:
      - command: echo command
git:
  hooks:
    pre-commit:
      steps:
        - name: &hook_name shared-hook
          type: shell
          command: *command_name
        - name: included
          type: script
          script: !include.raw ./hook.star
          env:
            MESSAGE: !literal '{{ untouched }}'
settings:
  command_name: *command_name
  hook_name: *hook_name
  enabled: !env ATMOS_TEST_ANCHOR_SETTING
`},
		{"hook anchors used by commands", `git:
  hooks:
    pre-commit:
      steps:
        - name: &hook_name shared-hook
          type: shell
          command: &command_name shared-command
        - name: included
          type: script
          script: !include.raw ./hook.star
          env:
            MESSAGE: !literal '{{ untouched }}'
commands:
  - name: *command_name
    steps:
      - command: echo command
settings:
  command_name: *command_name
  hook_name: *hook_name
  enabled: !env ATMOS_TEST_ANCHOR_SETTING
`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_BASE_PATH", "")
			t.Setenv("ATMOS_TEST_ANCHOR_SETTING", "resolved-setting")
			dir := t.TempDir()
			file := filepath.Join(dir, "atmos.yaml")
			writeTestFile(t, file, tt.content)
			writeTestFile(t, filepath.Join(dir, "hook.star"), "print(ctx.args)\n")
			v := viper.New()
			v.SetConfigType("yaml")
			require.NoError(t, mergeConfigFile(file, v))
			assert.Equal(t, "shared-command", v.GetString("settings.command_name"))
			assert.Equal(t, "shared-hook", v.GetString("settings.hook_name"))
			assert.Equal(t, "resolved-setting", v.GetString("settings.enabled"))
			commands := v.Get("commands").([]any)
			require.Len(t, commands, 1)
			assert.Equal(t, "shared-command", commands[0].(map[string]any)["name"])
			hook := v.Get("git.hooks.pre-commit").(map[string]any)
			steps := hook["steps"].([]any)
			require.Len(t, steps, 2)
			assert.Equal(t, "shared-hook", steps[0].(map[string]any)["name"])
			assert.Equal(t, "shared-command", steps[0].(map[string]any)["command"])
			script := steps[1].(map[string]any)
			assert.Equal(t, "print(ctx.args)\n", script["script"])
			assert.Equal(t, filepath.Join(dir, "hook.star"), script["script_source"])
			assert.Contains(t, script["literal_fields"], "env.MESSAGE")
		})
	}
}
