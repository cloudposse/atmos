package selection

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

func TestSelectPair(t *testing.T) {
	stack := func(components map[string]any) map[string]any {
		return map[string]any{"components": map[string]any{"helm": components, "packer": map[string]any{"image": map[string]any{}}}}
	}
	manifest := map[string]any{
		"qa":    stack(map[string]any{"api": map[string]any{}, "worker": map[string]any{}, "disabled": map[string]any{"metadata": map[string]any{"enabled": false}}, "abstract": map[string]any{"metadata": map[string]any{"type": "abstract"}}, "bad": 42}),
		"dev":   stack(map[string]any{"api": map[string]any{}}),
		"empty": "invalid stack section",
	}
	var choices [][]string
	prompt := func(_, _ string, v []string) (string, error) { choices = append(choices, v); return v[0], nil }
	for _, tc := range []struct {
		initial, stack, selected, wantStack string
		prompts                             [][]string
	}{
		{"", "", "api", "dev", [][]string{{"api", "worker"}, {"dev", "qa"}}},
		{"worker", "", "worker", "qa", nil},
		{"", "qa", "api", "qa", [][]string{{"api", "worker"}}},
		{"", "dev", "api", "dev", nil},
	} {
		info := schema.ConfigAndStacksInfo{ComponentType: "helm", ComponentFromArg: tc.initial, Stack: tc.stack}
		choices = nil
		require.NoError(t, selectPair(&info, manifest, prompt))
		require.Equal(t, tc.selected, info.ComponentFromArg)
		require.Equal(t, tc.wantStack, info.Stack)
		require.Equal(t, tc.prompts, choices)
	}
	for _, failField := range []string{"component", "stack"} {
		info := schema.ConfigAndStacksInfo{ComponentType: "helm"}
		err := selectPair(&info, manifest, func(field, _ string, options []string) (string, error) {
			if field == failField {
				return "", errUtils.ErrUserAborted
			}
			return options[0], nil
		})
		require.ErrorIs(t, err, errUtils.ErrUserAborted)
		require.Empty(t, info.ComponentFromArg)
		require.Empty(t, info.Stack)
	}
	info := schema.ConfigAndStacksInfo{ComponentType: "helm", ComponentFromArg: "image"}
	require.ErrorIs(t, selectPair(&info, manifest, prompt), errUtils.ErrNoOptionsAvailable)
	info.ComponentFromArg = ""
	info.Stack = "empty"
	require.ErrorContains(t, selectPair(&info, manifest, prompt), `stack "empty" contains no deployable helm components`)
	info.Stack = ""
	require.ErrorContains(t, selectPair(&info, nil, prompt), "no deployable helm components were found")
}

func TestResolveFromProject(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("ATMOS_FORCE_TTY", "true")
	t.Setenv("ATMOS_CI_WORKER_MODE", "false")
	env := telemetry.PreserveCIEnvVars()
	t.Cleanup(func() { telemetry.RestoreCIEnvVars(env) })
	prev := viper.Get("interactive")
	viper.Set("interactive", true)
	t.Cleanup(func() { viper.Set("interactive", prev) })
	cmd := &cobra.Command{}
	cmd.Flags().Bool("ci", false, "")
	require.NoError(t, cmd.Flags().Set("ci", "true"))
	info := schema.ConfigAndStacksInfo{ComponentType: "packer"}
	require.NoError(t, Resolve(cmd, &info))
	require.Empty(t, info.Stack)
	require.NoError(t, cmd.Flags().Set("ci", "false"))
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("base_path: .\nstacks:\n  base_path: stacks\n  included_paths: ['*.yaml']\n"), 0o600))
	require.Error(t, Resolve(cmd, &info))
	require.NoError(t, os.Mkdir("stacks", 0o700))
	f := filepath.Join("stacks", "dev.yaml")
	require.NoError(t, os.WriteFile(f, []byte("components: {packer: {image: {metadata: {component: image}}}}"), 0o600))
	require.NoError(t, Resolve(cmd, &info))
	require.Equal(t, "image", info.ComponentFromArg)
	require.Equal(t, "dev", info.Stack)
	require.NoError(t, os.Remove(f))
	require.NoError(t, Resolve(cmd, &info))
}
