package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/schema"
)

const rootTUIConfig = `base_path: ./
stacks:
  base_path: stacks
  included_paths: ["deploy/**/*"]
  name_template: "{{ .vars.stage }}"
components:
  terraform:
    base_path: components/terraform
`

const rootTUIStack = `vars:
  stage: dev
components:
  terraform:
    example:
      vars: {}
`

func rootTUIProject(t *testing.T, files map[string]string) string {
	t.Helper()
	_ = NewTestKit(t)
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", "")
	t.Setenv("ATMOS_STACKS_BASE_PATH", "")
	t.Setenv("ATMOS_STACKS_INCLUDED_PATHS", "")
	t.Setenv("ATMOS_STACKS_EXCLUDED_PATHS", "")
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	previous := executeAtmosUI
	t.Cleanup(func() { executeAtmosUI = previous })
	previousDetector := rootTTYDetector
	t.Cleanup(func() { rootTTYDetector = previousDetector })
	detector := term.NewMockTTYDetector(gomock.NewController(t))
	detector.EXPECT().IsTTYForStdin().Return(true).AnyTimes()
	detector.EXPECT().IsTTYForStdout().Return(true).AnyTimes()
	rootTTYDetector = detector
	return dir
}

func rootTUICommand() *cobra.Command {
	command := &cobra.Command{Use: "atmos", RunE: RootCmd.RunE}
	command.Flags().String("base-path", "", "")
	command.Flags().StringSlice("config", nil, "")
	command.Flags().StringSlice("config-path", nil, "")
	return command
}

func TestRootWithoutStacksRequestsHelp(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
	}{
		{name: "empty directory"},
		{name: "workflows and custom commands only", files: map[string]string{
			"atmos.yaml": "workflows:\n  base_path: workflows\ncommands:\n  - name: hello\n    steps: [echo hello]\n",
		}},
		{name: "missing stack base path", files: map[string]string{
			"atmos.yaml": "stacks:\n  base_path: ''\n  included_paths: [deploy/**/*]\n",
		}},
		{name: "missing include paths", files: map[string]string{
			"atmos.yaml": "stacks:\n  base_path: stacks\n  included_paths: []\n",
		}},
		{name: "missing stacks directory", files: map[string]string{"atmos.yaml": rootTUIConfig}},
		{name: "no matching manifests", files: map[string]string{
			"atmos.yaml": rootTUIConfig, "stacks/catalog/example.yaml": rootTUIStack,
		}},
		{name: "all manifests excluded", files: map[string]string{
			"atmos.yaml":             strings.Replace(rootTUIConfig, "  included_paths:", "  excluded_paths: [\"deploy/**/*\"]\n  included_paths:", 1),
			"stacks/deploy/dev.yaml": rootTUIStack,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootTUIProject(t, tt.files)
			executeAtmosUI = func(*schema.AtmosConfiguration) error {
				t.Fatal("TUI must not launch without stack manifests")
				return nil
			}
			command := rootTUICommand()
			var helpArgs []string
			command.SetHelpFunc(func(cmd *cobra.Command, args []string) {
				assert.Same(t, command, cmd)
				helpArgs = args
			})
			require.NoError(t, RootCmd.RunE(command, nil))
			assert.Equal(t, []string{helpFlagLong}, helpArgs)
		})
	}
}

func TestRootWithStacksLaunchesTUI(t *testing.T) {
	dir := rootTUIProject(t, map[string]string{
		"atmos.yaml": rootTUIConfig, "stacks/deploy/dev.yaml": rootTUIStack,
	})
	command := rootTUICommand()
	command.SetHelpFunc(func(*cobra.Command, []string) { t.Fatal("valid stacks must launch the TUI") })
	called := false
	executeAtmosUI = func(config *schema.AtmosConfiguration) error {
		called = true
		require.Len(t, config.StackConfigFilesAbsolutePaths, 1)
		assert.Equal(t, filepath.Join(dir, "stacks", "deploy", "dev.yaml"), config.StackConfigFilesAbsolutePaths[0])
		return nil
	}
	require.NoError(t, RootCmd.RunE(command, nil))
	assert.True(t, called, "bare atmos must invoke the TUI")
}

func TestRootWithStacksWithoutTerminalRequestsHelp(t *testing.T) {
	for _, stdinTTY := range []bool{false, true} {
		t.Run(strconv.FormatBool(stdinTTY), func(t *testing.T) {
			rootTUIProject(t, map[string]string{
				"atmos.yaml": rootTUIConfig, "stacks/deploy/dev.yaml": rootTUIStack,
			})
			detector := term.NewMockTTYDetector(gomock.NewController(t))
			detector.EXPECT().IsTTYForStdin().Return(stdinTTY)
			if stdinTTY {
				detector.EXPECT().IsTTYForStdout().Return(false)
			}
			rootTTYDetector = detector
			executeAtmosUI = func(*schema.AtmosConfiguration) error {
				t.Fatal("noninteractive input or output must not launch the TUI")
				return nil
			}
			command := rootTUICommand()
			var helpArgs []string
			command.SetHelpFunc(func(_ *cobra.Command, args []string) { helpArgs = args })
			require.NoError(t, RootCmd.RunE(command, nil))
			assert.Equal(t, []string{helpFlagLong}, helpArgs)
		})
	}
}

func TestRootTUIHonorsConfigOverrides(t *testing.T) {
	for _, mode := range []string{"config", "config-path", "base-path", "environment"} {
		t.Run(mode, func(t *testing.T) {
			dir := rootTUIProject(t, map[string]string{
				"atmos.yaml":                     "stacks:\n  base_path: ''\n  included_paths: []\n",
				"project/atmos.yaml":             rootTUIConfig,
				"project/stacks/deploy/dev.yaml": rootTUIStack,
			})
			project := filepath.Join(dir, "project")
			command := rootTUICommand()
			switch mode {
			case "config":
				require.NoError(t, command.Flags().Set("config", filepath.Join(project, "atmos.yaml")))
			case "config-path":
				require.NoError(t, command.Flags().Set("config-path", project))
			case "base-path":
				t.Setenv("ATMOS_CLI_CONFIG_PATH", project)
				t.Setenv("ATMOS_BASE_PATH", filepath.Join(dir, "wrong"))
				require.NoError(t, command.Flags().Set("base-path", project))
			case "environment":
				t.Setenv("ATMOS_BASE_PATH", project)
				t.Setenv("ATMOS_STACKS_BASE_PATH", "stacks")
				t.Setenv("ATMOS_STACKS_INCLUDED_PATHS", "deploy/**/*")
			}
			command.SetHelpFunc(func(*cobra.Command, []string) { t.Fatal("overridden stacks must launch the TUI") })
			called := false
			executeAtmosUI = func(config *schema.AtmosConfiguration) error {
				called = true
				assert.Equal(t, project, config.BasePathAbsolute)
				assert.Equal(t, []string{filepath.Join(project, "stacks", "deploy", "dev.yaml")}, config.StackConfigFilesAbsolutePaths)
				return nil
			}
			require.NoError(t, RootCmd.RunE(command, nil))
			assert.True(t, called)
		})
	}
}

func TestRootTUIDoesNotHideErrors(t *testing.T) {
	tests := []struct {
		name   string
		config string
		stack  string
	}{
		{name: "malformed CLI config", config: "stacks: ["},
		{name: "invalid glob", config: strings.Replace(rootTUIConfig, "deploy/**/*", "[", 1)},
		{name: "malformed stack", config: rootTUIConfig, stack: "components: ["},
		{name: "missing stack import", config: rootTUIConfig, stack: "import: [missing]\n" + rootTUIStack},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"atmos.yaml": tt.config}
			if tt.stack != "" {
				files["stacks/deploy/dev.yaml"] = tt.stack
			}
			rootTUIProject(t, files)
			executeAtmosUI = e.ExecuteAtmosCmdWithConfig
			command := rootTUICommand()
			command.SetHelpFunc(func(*cobra.Command, []string) { t.Fatal("configuration errors must not display help") })
			require.Error(t, RootCmd.RunE(command, nil))
		})
	}
}

func TestRootTUIReturnsLaunchError(t *testing.T) {
	rootTUIProject(t, map[string]string{
		"atmos.yaml": rootTUIConfig, "stacks/deploy/dev.yaml": rootTUIStack,
	})
	wantErr := errors.New("cannot open terminal")
	executeAtmosUI = func(*schema.AtmosConfiguration) error { return wantErr }
	command := rootTUICommand()
	command.SetHelpFunc(func(*cobra.Command, []string) { t.Fatal("TUI failures must not display help") })
	require.ErrorIs(t, RootCmd.RunE(command, nil), wantErr)
}

func TestRootTUIExplicitHelp(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{name: "valid stacks", config: rootTUIConfig},
		{name: "invalid config", config: "stacks: ["},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootTUIProject(t, map[string]string{
				"atmos.yaml": tt.config, "stacks/deploy/dev.yaml": rootTUIStack,
			})
			executeAtmosUI = func(*schema.AtmosConfiguration) error {
				t.Fatal("explicit help must never launch the TUI")
				return nil
			}
			called := false
			RootCmd.SetHelpFunc(func(*cobra.Command, []string) { called = true })
			RootCmd.SetArgs([]string{"--help"})
			require.NoError(t, RootCmd.Execute())
			assert.True(t, called)
		})
	}
}
