package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for schema fields referenced by these tests.
var (
	_ = schema.Task{Env: map[string]string{}, Defaults: &schema.CastDefaults{}}
	_ = schema.Command{Env: []schema.CommandEnv{}}
	_ = schema.CastDefaults{Simulate: &schema.CastSimulateDefaults{Rate: "", Jitter: 0}}
)

// writeCommandIncludeFixture writes a minimal atmos.yaml, an atmos.d command file,
// and the !include target into a temp directory and returns the directory path.
func writeCommandIncludeFixture(t *testing.T, includeYAML, commandYAML string) string {
	t.Helper()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "atmos.d"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte("base_path: \".\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "cast-defaults.yaml"), []byte(includeYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.d", "commands.yaml"), []byte(commandYAML), 0o644))

	return tempDir
}

// loadConfigViaMergeConfig loads the fixture directory through the real config
// pipeline (mergeConfig with import processing, which is what LoadConfig uses for
// each config source) and applies the same case restoration LoadConfig performs.
func loadConfigViaMergeConfig(t *testing.T, tempDir string) *schema.AtmosConfiguration {
	t.Helper()

	// The !include function resolves relative paths against the working directory.
	t.Chdir(tempDir)

	v := viper.New()
	v.SetConfigType(yamlType)

	// mergeConfigFile tracks merged files in a per-v tracker for case-map
	// extraction; register one for v like LoadConfig does at the start of each load.
	resetMergedConfigFiles(v)
	require.NoError(t, mergeConfig(v, tempDir, CliConfigFileName, true))

	var cfg schema.AtmosConfiguration
	require.NoError(t, v.Unmarshal(&cfg, atmosDecodeHook()))
	preserveCaseSensitiveMaps(v, &cfg)
	restoreCaseSensitiveEnvMaps(&cfg)
	return &cfg
}

const envIncludeFixtureYAML = `
env:
  recording:
    ATMOS_FORCE_COLOR: "true"
    ATMOS_EXPERIMENTAL: "true"
    COLUMNS: "120"
`

// TestAtmosDCommandStepEnvInclude reproduces the bug where a step-level
// `env: !include <file> <selector>` in an atmos.d command definition silently
// resolved to the raw include string instead of the included map: the
// un-preprocessed commands leaked through the nil commands override in
// processConfigImportsAndReapply and overrode the resolved default-import
// commands. It also verifies original key case is recoverable from the env
// case map, the same way custom-command execution restores step env case.
func TestAtmosDCommandStepEnvInclude(t *testing.T) {
	commandYAML := `
commands:
  - name: casts envtest
    description: Env include probe
    steps:
      - type: cast
        name: envtest
        title: env probe
        mode: steps
        env: !include cast-defaults.yaml .env.recording
        steps:
          - type: shell
            name: show-env
            command: env | sort
      - type: shell
        name: literal
        command: env | sort
        env:
          LITERAL_UPPER: "yes"
`

	cfg := loadConfigViaMergeConfig(t, writeCommandIncludeFixture(t, envIncludeFixtureYAML, commandYAML))

	casts := findCommand(t, cfg.Commands, "casts")
	envtest := findCommand(t, casts.Commands, "envtest")
	require.Len(t, envtest.Steps, 2)

	castStep := envtest.Steps[0]
	assert.Equal(t, "cast", castStep.Type)
	require.NotEmpty(t, castStep.Env, "step env from !include must not be empty")

	// Custom-command execution restores step env case with CaseMaps.ApplyCase
	// (see cmd/cmd_utils.go); assert the restored map carries original-case keys.
	env := cfg.CaseMaps.ApplyCase(envKey, castStep.Env)
	assert.Equal(t, "true", env["ATMOS_FORCE_COLOR"])
	assert.Equal(t, "true", env["ATMOS_EXPERIMENTAL"])
	assert.Equal(t, "120", env["COLUMNS"])

	// Regression guard: a literal env map on a sibling step keeps working.
	literalStep := envtest.Steps[1]
	literalEnv := cfg.CaseMaps.ApplyCase(envKey, literalStep.Env)
	assert.Equal(t, "yes", literalEnv["LITERAL_UPPER"])
}

// TestAtmosDCommandLevelEnvInclude verifies that a command-level
// `env: !include ...` resolves to the included map with original-case keys.
func TestAtmosDCommandLevelEnvInclude(t *testing.T) {
	commandYAML := `
commands:
  - name: envtest
    description: Env include probe
    env: !include cast-defaults.yaml .env.recording
    steps:
      - type: shell
        name: show-env
        command: env | sort
`

	cfg := loadConfigViaMergeConfig(t, writeCommandIncludeFixture(t, envIncludeFixtureYAML, commandYAML))

	envtest := findCommand(t, cfg.Commands, "envtest")
	require.NotEmpty(t, envtest.Env, "command env from !include must not be empty")

	// Command-level env case is restored by restoreCaseSensitiveEnvMaps during
	// LoadConfig, which loadConfigViaMergeConfig mirrors.
	envByKey := map[string]string{}
	for _, e := range envtest.Env {
		envByKey[e.Key] = e.Value
	}
	assert.Equal(t, "true", envByKey["ATMOS_FORCE_COLOR"])
	assert.Equal(t, "true", envByKey["ATMOS_EXPERIMENTAL"])
	assert.Equal(t, "120", envByKey["COLUMNS"])
}

// TestAtmosDCommandStepDefaultsInclude verifies that a cast step's
// `defaults: !include ...` resolves to the included structure.
func TestAtmosDCommandStepDefaultsInclude(t *testing.T) {
	includeYAML := `
cast:
  defaults:
    simulate:
      rate: 25ms
      jitter: 0.3
`
	commandYAML := `
commands:
  - name: envtest
    description: Defaults include probe
    steps:
      - type: cast
        name: envtest
        mode: steps
        defaults: !include cast-defaults.yaml .cast.defaults
        steps:
          - type: shell
            name: show-env
            command: env | sort
`

	cfg := loadConfigViaMergeConfig(t, writeCommandIncludeFixture(t, includeYAML, commandYAML))

	envtest := findCommand(t, cfg.Commands, "envtest")
	require.Len(t, envtest.Steps, 1)
	step := envtest.Steps[0]
	require.NotNil(t, step.Defaults, "cast step defaults from !include must not be nil")
	require.NotNil(t, step.Defaults.Simulate, "cast step simulate defaults from !include must not be nil")
	assert.Equal(t, "25ms", step.Defaults.Simulate.Rate)
	assert.InDelta(t, 0.3, step.Defaults.Simulate.Jitter, 0.0001)
}

func TestCommandsUnsetClearsInheritedCommands(t *testing.T) {
	setupTestAdapters()
	for _, mode := range []string{"fragment", "main"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			inherited := "commands:\n  - name: inherited\n    steps: [echo inherited]\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, "imported.yaml"), []byte(inherited), 0o600))
			v := viper.New()
			v.SetConfigType(yamlType)
			resetMergedConfigFiles(v)
			if mode == "fragment" {
				require.NoError(t, mergeConfigFile(filepath.Join(root, "imported.yaml"), v))
				require.NoError(t, os.WriteFile(filepath.Join(root, "unset.yaml"), []byte("commands: !unset\n"), 0o600))
				require.NoError(t, mergeConfigFile(filepath.Join(root, "unset.yaml"), v))
			} else {
				content := "base_path: .\nimport: [imported.yaml]\ncommands: !unset\n"
				require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(content), 0o600))
				t.Chdir(root)
				require.NoError(t, mergeConfig(v, root, CliConfigFileName, true))
			}
			var cfg schema.AtmosConfiguration
			require.NoError(t, v.Unmarshal(&cfg, atmosDecodeHook()))
			assert.Empty(t, cfg.Commands, "!unset must clear inherited commands")
		})
	}
}

func TestImportedCommandsUnsetClearsDefaults(t *testing.T) {
	setupTestAdapters()
	for _, tc := range []struct {
		name, main string
		want       []string
	}{
		{"direct import", "import: [unset.yaml]\n", nil},
		{"nested import", "import: [nested.yaml]\n", nil},
		{"main restores commands", "import: [nested.yaml]\ncommands:\n  - name: restored-main\n", []string{"restored-main"}},
		{"later import restores commands", "import: [unset.yaml, restore.yaml]\n", []string{"restored-import"}},
		{"nested unset clears importing file commands", "import: [nested-restore.yaml]\n", nil},
		{"omitted commands retain defaults", "import: [omitted.yaml]\n", []string{"inherited"}},
		{"empty command list retains defaults", "import: [empty.yaml]\n", []string{"inherited"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".atmos.d"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(root, ".atmos.d", "defaults.yaml"), []byte("commands:\n  - name: inherited\n    steps: [echo inherited]\n"), 0o600))
			for name, content := range map[string]string{
				"unset.yaml":          "commands: !unset\n",
				"nested.yaml":         "import: [unset.yaml]\n",
				"restore.yaml":        "commands:\n  - name: restored-import\n",
				"nested-restore.yaml": "import: [unset.yaml]\ncommands:\n  - name: restored-parent\n",
				"omitted.yaml":        "logs: {level: Info}\n",
				"empty.yaml":          "commands: []\n",
				"atmos.yaml":          "base_path: .\n" + tc.main,
			} {
				require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o600))
			}
			t.Chdir(root)
			v := viper.New()
			v.SetConfigType(yamlType)
			resetMergedConfigFiles(v)
			require.NoError(t, mergeConfig(v, root, CliConfigFileName, true))
			var cfg schema.AtmosConfiguration
			require.NoError(t, v.Unmarshal(&cfg, atmosDecodeHook()))
			var names []string
			for _, command := range cfg.Commands {
				names = append(names, command.Name)
			}
			assert.Equal(t, tc.want, names)
		})
	}
}
