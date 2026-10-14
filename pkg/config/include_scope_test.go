package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goyaml "go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestCommandIncludeBasePath(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()

	tests := []struct {
		name    string
		file    string
		content string
		want    string
	}{
		{name: "no base_path uses the config directory", file: "atmos.yaml", content: "commands: []\n", want: root},
		{name: "dot base_path is the config directory", file: "atmos.yaml", content: "base_path: ./\n", want: root},
		{name: "relative base_path resolves against the config directory", file: filepath.Join("conf", "atmos.yaml"), content: "base_path: ..\n", want: root},
		{name: "atmos.d falls back to the parent of the directory", file: filepath.Join("atmos.d", "cmds.yaml"), content: "commands: []\n", want: root},
		{name: "a nested atmos.d file falls back to the parent of atmos.d", file: filepath.Join(".atmos.d", "cli", "deep", "cmds.yaml"), content: "commands: []\n", want: root},
		{name: "a base_path computed by a function is ignored", file: filepath.Join("fn", "atmos.yaml"), content: "base_path: !cwd\n", want: filepath.Join(root, "fn")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			writeTestFile(t, path, tc.content)
			assert.Equal(t, tc.want, commandIncludeBasePath(path))
		})
	}

	t.Run("ATMOS_BASE_PATH wins", func(t *testing.T) {
		override := t.TempDir()
		t.Setenv("ATMOS_BASE_PATH", override)
		path := filepath.Join(root, "atmos.yaml")
		writeTestFile(t, path, "base_path: ./\n")
		assert.Equal(t, override, commandIncludeBasePath(path))
	})
}

// setTestOsArgs replaces os.Args for one test, restoring it on cleanup.
func setTestOsArgs(t *testing.T, args ...string) {
	t.Helper()
	orig := os.Args
	t.Cleanup(func() { os.Args = orig })
	os.Args = append([]string{"atmos"}, args...)
}

// TestCommandIncludeBasePathHonorsBaseDirFlag checks the --base-path flag has the same precedence
// here as for the rest of the config: flag > ATMOS_BASE_PATH > the file's base_path > config dir.
func TestCommandIncludeBasePathHonorsBaseDirFlag(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	path := filepath.Join(root, "atmos.yaml")
	writeTestFile(t, path, "base_path: ./declared\n")

	t.Run("equals form wins over the declared base_path", func(t *testing.T) {
		flagDir := t.TempDir()
		setTestOsArgs(t, "--base-path="+flagDir, "custom-cmd")
		assert.Equal(t, flagDir, commandIncludeBasePath(path))
	})

	t.Run("space-separated form is honored", func(t *testing.T) {
		flagDir := t.TempDir()
		setTestOsArgs(t, "--base-path", flagDir, "custom-cmd")
		assert.Equal(t, flagDir, commandIncludeBasePath(path))
	})

	t.Run("flag wins over ATMOS_BASE_PATH", func(t *testing.T) {
		flagDir, envDir := t.TempDir(), t.TempDir()
		t.Setenv("ATMOS_BASE_PATH", envDir)
		setTestOsArgs(t, "--base-path="+flagDir)
		assert.Equal(t, flagDir, commandIncludeBasePath(path))
	})

	t.Run("without the flag ATMOS_BASE_PATH still applies", func(t *testing.T) {
		envDir := t.TempDir()
		t.Setenv("ATMOS_BASE_PATH", envDir)
		setTestOsArgs(t, "custom-cmd")
		assert.Equal(t, envDir, commandIncludeBasePath(path))
	})

	t.Run("without flag or env the declared base_path applies", func(t *testing.T) {
		setTestOsArgs(t, "custom-cmd")
		assert.Equal(t, filepath.Join(root, "declared"), commandIncludeBasePath(path))
	})

	t.Run("a dot-relative flag value resolves against the working directory", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)
		setTestOsArgs(t, "--base-path=./project")
		got := commandIncludeBasePath(path)
		wantCwd, err := filepath.EvalSymlinks(cwd)
		require.NoError(t, err)
		gotDir, err := filepath.EvalSymlinks(filepath.Dir(got))
		require.NoError(t, err)
		assert.Equal(t, wantCwd, gotDir)
		assert.Equal(t, "project", filepath.Base(got))
		assert.NotContains(t, got, root, "never anchored to the config directory")
	})
}

// TestCommandIncludesResolveAgainstBaseDirFlag checks a bare !include path in a custom command
// reads from the --base-path directory.
func TestCommandIncludesResolveAgainstBaseDirFlag(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	flagDir := t.TempDir()
	writeTestFile(t, filepath.Join(root, "scripts", "run.star"), "print('config-dir')\n")
	writeTestFile(t, filepath.Join(flagDir, "scripts", "run.star"), "print('flag-dir')\n")

	file := filepath.Join(root, "atmos.yaml")
	content := "commands:\n  - name: demo\n    steps:\n      - type: script\n        script: !include scripts/run.star\n"
	writeTestFile(t, file, content)

	setTestOsArgs(t, "--base-path="+flagDir, "demo")
	step := firstStep(t, decodeTestCommands(t, file, content))
	assert.Contains(t, step["script"], "flag-dir")
	assert.NotContains(t, step["script"], "config-dir")

	setTestOsArgs(t, "demo")
	step = firstStep(t, decodeTestCommands(t, file, content))
	assert.Contains(t, step["script"], "config-dir")
}

func decodeTestCommands(t *testing.T, file, content string) []any {
	t.Helper()
	got, err := extractCommandsWithYamlFunctionsForFile([]byte(content), file)
	require.NoError(t, err)
	commands, ok := got.([]any)
	require.True(t, ok)
	return commands
}

func firstStep(t *testing.T, commands []any) map[string]any {
	t.Helper()
	command, ok := commands[0].(map[string]any)
	require.True(t, ok)
	steps, ok := command["steps"].([]any)
	require.True(t, ok)
	step, ok := steps[0].(map[string]any)
	require.True(t, ok)
	return step
}

func TestCommandIncludesResolveIndependentlyOfWorkingDirectory(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	configFile := filepath.Join(root, "conf", "atmos.yaml")
	writeTestFile(t, configFile, "base_path: ..\n")
	writeTestFile(t, filepath.Join(root, "scripts", "main.star"), "print(\"main\")\n")
	writeTestFile(t, filepath.Join(root, "conf", "local.star"), "print(\"local\")\n")

	// Decoys: files of the same names in the working directory must never be read.
	elsewhere := t.TempDir()
	writeTestFile(t, filepath.Join(elsewhere, "scripts", "main.star"), "decoy")
	writeTestFile(t, filepath.Join(elsewhere, "local.star"), "decoy")

	content := func(script string) string {
		return "commands:\n  - name: c\n    steps:\n      - name: s\n        type: script\n        script: " + script + "\n"
	}
	tests := []struct {
		name, script, wantScript, wantSource string
	}{
		{name: "bare resolves against the project base path", script: "!include scripts/main.star", wantScript: "print(\"main\")\n", wantSource: filepath.Join(root, "scripts", "main.star")},
		{name: "dot resolves against the config file", script: "!include ./local.star", wantScript: "print(\"local\")\n", wantSource: filepath.Join(root, "conf", "local.star")},
		{name: "include.raw records the source", script: "!include.raw scripts/main.star", wantScript: "print(\"main\")\n", wantSource: filepath.Join(root, "scripts", "main.star")},
		{name: "absolute is kept", script: "!include " + filepath.ToSlash(filepath.Join(root, "scripts", "main.star")), wantScript: "print(\"main\")\n", wantSource: filepath.Join(root, "scripts", "main.star")},
		{name: "inline script has no source", script: "'print(1)'", wantScript: "print(1)"},
	}
	for _, dir := range []string{root, elsewhere} {
		for _, tc := range tests {
			t.Run(filepath.Base(dir)+"/"+tc.name, func(t *testing.T) {
				t.Chdir(dir)
				step := firstStep(t, decodeTestCommands(t, configFile, content(tc.script)))
				assert.Equal(t, tc.wantScript, step["script"])
				if tc.wantSource == "" {
					assert.NotContains(t, step, scriptSourceKey)
					return
				}
				assert.Equal(t, tc.wantSource, step[scriptSourceKey])
			})
		}
	}
}

func TestCommandScriptSourceIsReservedAndQueriesHaveNone(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	configFile := filepath.Join(root, "atmos.yaml")
	writeTestFile(t, configFile, "")
	writeTestFile(t, filepath.Join(root, "data.yaml"), "script: print(1)\n")

	t.Run("a script_source written by the user is dropped", func(t *testing.T) {
		step := firstStep(t, decodeTestCommands(t, configFile, `
commands:
  - name: c
    steps:
      - name: s
        script_source: /etc/passwd
        script: print(1)
`))
		assert.NotContains(t, step, scriptSourceKey)
	})

	t.Run("a YQ query changes the content so there is no source", func(t *testing.T) {
		step := firstStep(t, decodeTestCommands(t, configFile, `
commands:
  - name: c
    steps:
      - name: s
        script: !include ./data.yaml .script
`))
		assert.Equal(t, "print(1)", step["script"])
		assert.NotContains(t, step, scriptSourceKey)
	})

	t.Run("no source file means no provenance", func(t *testing.T) {
		var node goyaml.Node
		require.NoError(t, goyaml.Unmarshal([]byte("script: !include x.star\n"), &node))
		decoded := map[string]any{"script_source": "kept-only-when-from-a-file"}
		recordScriptSource(node.Content[0], decoded, "")
		assert.Contains(t, decoded, scriptSourceKey)
	})
}

func TestLoadedCommandsKeepScriptSourceFromAnyWorkingDirectory(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "atmos.yaml"), `base_path: "./"
commands:
  - name: c
    steps:
      - name: s
        type: script
        interpreter: starlark
        script: !include scripts/main.star
      - name: g
        type: parallel
        steps:
          - name: child
            type: script
            interpreter: starlark
            script: !include scripts/main.star
`)
	writeTestFile(t, filepath.Join(root, "scripts", "main.star"), "print(\"main\")\n")
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
	require.Len(t, found.Steps, 2)
	main := filepath.Join(root, "scripts", "main.star")
	resolvedMain, err := filepath.EvalSymlinks(main)
	require.NoError(t, err)
	assert.Equal(t, "print(\"main\")\n", found.Steps[0].Script)
	assert.Contains(t, []string{main, resolvedMain}, found.Steps[0].ScriptSource)
	require.Len(t, found.Steps[1].Steps, 1)
	assert.Contains(t, []string{main, resolvedMain}, found.Steps[1].Steps[0].ScriptSource)
	assert.Equal(t, found.Steps[0].ScriptSource, found.Steps[0].ToWorkflowStep().ScriptSource)
}

// Commands are merged by name and a step list is replaced wholesale, so a step never combines one
// definition's script with another definition's script_source: an override that supplies an
// inline script leaves no inherited provenance behind. (Stack hooks deep-merge maps instead, and
// validate script_source against a content fingerprint for that reason.)
func TestOverriddenCommandStepDoesNotInheritScriptSource(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "atmos.yaml"), `base_path: "./"
commands:
  - name: c
    description: inline override
    steps:
      - name: s
        type: script
        interpreter: starlark
        script: print("inline")
  - name: kept
    steps:
      - name: s
        type: script
        interpreter: starlark
        script: !include scripts/main.star
`)
	writeTestFile(t, filepath.Join(root, "atmos.d", "base.yaml"), `commands:
  - name: c
    description: included base
    steps:
      - name: s
        type: script
        interpreter: starlark
        script: !include scripts/main.star
`)
	writeTestFile(t, filepath.Join(root, "scripts", "main.star"), "print(\"main\")\n")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Chdir(t.TempDir())

	atmosConfig, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	byName := map[string]schema.Command{}
	for _, command := range atmosConfig.Commands {
		byName[command.Name] = command
	}
	require.Contains(t, byName, "c")
	require.Len(t, byName["c"].Steps, 1)
	assert.Equal(t, "print(\"inline\")", byName["c"].Steps[0].Script)
	assert.Empty(t, byName["c"].Steps[0].ScriptSource, "an inline override must not inherit the base command's script_source")

	// Positive control: a command that keeps its include keeps its provenance.
	require.Contains(t, byName, "kept")
	require.Len(t, byName["kept"].Steps, 1)
	assert.NotEmpty(t, byName["kept"].Steps[0].ScriptSource)
}

func TestPreprocessExceptCommandsPreservesAnchors(t *testing.T) {
	content := []byte(`commands:
  - name: &command_name shared-name
    description: &metadata
      label: anchored-label
    steps:
      - command: !env ATMOS_TEST_ANCHOR_COMMAND
settings:
  name: *command_name
  metadata: *metadata
  enabled: !env ATMOS_TEST_ANCHOR_SETTING
`)
	t.Setenv("ATMOS_TEST_ANCHOR_SETTING", "resolved")
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, v.ReadConfig(bytes.NewReader(content)))
	require.NoError(t, preprocessAtmosYamlFuncExceptCommands(content, v, "atmos.yaml"))
	assert.Equal(t, "shared-name", v.GetString("settings.name"))
	assert.Equal(t, "anchored-label", v.GetString("settings.metadata.label"))
	assert.Equal(t, "resolved", v.GetString("settings.enabled"))
	commands := v.Get("commands").([]any)
	command := commands[0].(map[string]any)
	steps := command["steps"].([]any)
	assert.Equal(t, "ATMOS_TEST_ANCHOR_COMMAND", steps[0].(map[string]any)["command"], "excluded commands are not evaluated")
}

func TestMergeConfigFilePreservesCommandAnchors(t *testing.T) {
	t.Setenv("ATMOS_TEST_ANCHORED_COMMAND", "command-value")
	t.Setenv("ATMOS_TEST_ANCHORED_SETTING", "setting-value")
	file := filepath.Join(t.TempDir(), "atmos.yaml")
	writeTestFile(t, file, `commands:
  - name: &shared_name anchored-command
    steps:
      - command: !env ATMOS_TEST_ANCHORED_COMMAND
settings:
  name: *shared_name
  value: !env ATMOS_TEST_ANCHORED_SETTING
`)
	v := viper.New()
	v.SetConfigType("yaml")
	require.NoError(t, mergeConfigFile(file, v))
	assert.Equal(t, "anchored-command", v.GetString("settings.name"))
	assert.Equal(t, "setting-value", v.GetString("settings.value"))
	commands := v.Get("commands").([]any)
	steps := commands[0].(map[string]any)["steps"].([]any)
	assert.Equal(t, "command-value", steps[0].(map[string]any)["command"])
}
