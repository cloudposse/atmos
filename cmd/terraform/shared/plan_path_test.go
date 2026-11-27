package shared

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSelectRegisteredPlanPath(t *testing.T) {
	originalInit, originalDescribe := initCliConfig, executeDescribeStacks
	originalInteractive, originalSelect := isInteractiveFn, selectFromOptions
	t.Cleanup(func() {
		initCliConfig, executeDescribeStacks = originalInit, originalDescribe
		isInteractiveFn, selectFromOptions = originalInteractive, originalSelect
	})
	t.Setenv("CI", "false")
	t.Setenv("ATMOS_CI", "false")
	root := t.TempDir()
	dir := filepath.Join(root, "roots", "network")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	config := schema.AtmosConfiguration{BasePathAbsolute: root, TerraformDirAbsolutePath: filepath.Join(root, "roots")}
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) { return config, nil }
	var loadErr error
	stacks := map[string]any{}
	executeDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return stacks, loadErr
	}
	stack := func(entries map[string]any) map[string]any {
		return map[string]any{"components": map[string]any{"terraform": entries}}
	}
	stacks["dev"] = stack(map[string]any{
		"only-alias": map[string]any{"component": "network"},
		"abstract":   map[string]any{"component": "network", "metadata": map[string]any{"type": "abstract"}},
		"unrelated":  map[string]any{"component": "does-not-exist"},
	})
	cmd := &cobra.Command{Use: "plan"}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: dir}
	name, selected, err := SelectPlanPath(cmd, info)
	require.NoError(t, err)
	require.Equal(t, "only-alias", name)
	require.Equal(t, "dev", selected)

	stacks["prod"] = stack(map[string]any{"network": map[string]any{}})
	isInteractiveFn = func() bool { return false }
	_, _, err = SelectPlanPath(cmd, info)
	require.ErrorIs(t, err, errUtils.ErrAmbiguousComponentPath)
	isInteractiveFn = func() bool { return true }
	selectFromOptions = func(_ string, _ string, choices []string) (string, error) {
		require.Equal(t, []string{"atmos terraform plan network -s prod", "atmos terraform plan only-alias -s dev"}, choices)
		return choices[0], nil
	}
	name, selected, err = SelectPlanPath(cmd, info)
	require.NoError(t, err)
	require.Equal(t, "network", name)
	require.Equal(t, "prod", selected)
	selectFromOptions = func(string, string, []string) (string, error) { return "", errUtils.ErrUserAborted }
	_, _, err = SelectPlanPath(cmd, info)
	require.ErrorIs(t, err, errUtils.ErrUserAborted)

	delete(stacks, "prod")
	delete(stacks, "dev")
	_, _, err = SelectPlanPath(cmd, info)
	require.ErrorIs(t, err, errUtils.ErrComponentNotInStack)
	stacks["stage"] = stack(map[string]any{"unit": map[string]any{"settings": map[string]any{"configuration": map[string]any{
		"provider": "terragrunt", schema.ConfigurationOwnedKey: true, "source": "roots/network/terragrunt.hcl",
	}}}})
	name, selected, err = SelectPlanPath(cmd, info)
	require.NoError(t, err)
	require.Equal(t, "unit", name)
	require.Equal(t, "stage", selected)

	unitFile := filepath.Join(dir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(unitFile, nil, 0o600))
	info.ComponentFromArg = unitFile
	name, _, err = SelectPlanPath(cmd, info)
	require.NoError(t, err)
	require.Equal(t, "unit", name)

	require.Empty(t, planSourcePath(&config, "unit", map[string]any{"settings": map[string]any{"configuration": map[string]any{
		"provider": "terragrunt", schema.ConfigurationOwnedKey: true,
	}}}, true))

	loadErr = errors.New("cannot read stack")
	_, _, err = SelectPlanPath(cmd, info)
	require.ErrorIs(t, err, loadErr)
	info.ComponentFromArg = filepath.Join(root, "missing")
	_, _, err = SelectPlanPath(cmd, info)
	require.ErrorIs(t, err, os.ErrNotExist)
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) { return config, loadErr }
	_, _, err = SelectPlanPath(cmd, info)
	require.ErrorIs(t, err, loadErr)
}
