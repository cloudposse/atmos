package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			_, err := preprocessGitHookSteps([]byte(content), v, "")
			require.NoError(t, err)
		})
	}
	for _, content := range []string{"[invalid", "git: {hooks: {pre-commit: {steps: [{script: !include.raw ./missing.star}]}}}"} {
		_, err := preprocessGitHookSteps([]byte(content), viper.New(), filepath.Join(t.TempDir(), "atmos.yaml"))
		require.Error(t, err)
	}
}
