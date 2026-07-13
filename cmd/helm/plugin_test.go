package helm

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// findSubcommand returns the named direct subcommand of parent, or nil.
func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func TestPluginCommandStructure(t *testing.T) {
	pluginCmd := findSubcommand(helmCmd, "plugin")
	require.NotNil(t, pluginCmd, "helm should have a plugin subcommand")

	var subs []string
	for _, c := range pluginCmd.Commands() {
		subs = append(subs, c.Name())
	}
	assert.ElementsMatch(t, []string{"list", "install"}, subs)

	installCmd := findSubcommand(pluginCmd, "install")
	require.NotNil(t, installCmd)
	assert.NotNil(t, installCmd.Flag(flagComponent), "install should expose --component")
}

func TestCollectInstallSpecs(t *testing.T) {
	t.Run("explicit args take precedence", func(t *testing.T) {
		cmd := &cobra.Command{Use: "install"}
		cmd.Flags().String(flagComponent, "", "")
		specs, err := collectInstallSpecs(cmd, []string{"diff@v3.9.4", "secrets"})
		require.NoError(t, err)
		assert.Equal(t, []string{"diff@v3.9.4", "secrets"}, specs)
	})

	t.Run("no args and no component returns nil", func(t *testing.T) {
		cmd := &cobra.Command{Use: "install"}
		cmd.Flags().String(flagComponent, "", "")
		specs, err := collectInstallSpecs(cmd, nil)
		require.NoError(t, err)
		assert.Nil(t, specs)
	})
}

type fakePluginLister struct {
	installed map[string]string
	err       error
}

func (f fakePluginLister) ListInstalled(context.Context) (map[string]string, error) {
	return f.installed, f.err
}

func withPluginCommandSeams(t *testing.T) {
	t.Helper()
	origResolve := resolveHelmBinaryForPlugin
	origLister := newHelmPluginLister
	origEnsure := ensureHelmPluginsForComponent
	origInit := initHelmCliConfig
	origDescribe := describeHelmComponent
	t.Cleanup(func() {
		resolveHelmBinaryForPlugin = origResolve
		newHelmPluginLister = origLister
		ensureHelmPluginsForComponent = origEnsure
		initHelmCliConfig = origInit
		describeHelmComponent = origDescribe
	})
}

func TestRunPluginList(t *testing.T) {
	withPluginCommandSeams(t)
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)

	resolveHelmBinaryForPlugin = func(*cobra.Command) (string, error) { return "/bin/helm", nil }
	newHelmPluginLister = func(helmBin string) helmPluginLister {
		assert.Equal(t, "/bin/helm", helmBin)
		return fakePluginLister{installed: map[string]string{
			"secrets": "4.6.0",
			"diff":    "3.9.4",
		}}
	}

	require.NoError(t, runPluginList(&cobra.Command{}, nil))
}

func TestRunPluginListErrors(t *testing.T) {
	withPluginCommandSeams(t)

	sentinel := errors.New("helm resolution failed")
	resolveHelmBinaryForPlugin = func(*cobra.Command) (string, error) { return "", sentinel }
	require.ErrorIs(t, runPluginList(&cobra.Command{}, nil), sentinel)

	resolveHelmBinaryForPlugin = func(*cobra.Command) (string, error) { return "/bin/helm", nil }
	newHelmPluginLister = func(string) helmPluginLister {
		return fakePluginLister{err: sentinel}
	}
	require.ErrorIs(t, runPluginList(&cobra.Command{}, nil), sentinel)
}

func TestRunPluginInstall(t *testing.T) {
	withPluginCommandSeams(t)

	resolveHelmBinaryForPlugin = func(*cobra.Command) (string, error) { return "/bin/helm", nil }
	var gotHelm string
	var gotSpecs []string
	ensureHelmPluginsForComponent = func(_ context.Context, helmBin string, specs []string) (string, error) {
		gotHelm = helmBin
		gotSpecs = specs
		return "/plugins", nil
	}

	cmd := &cobra.Command{Use: "install"}
	cmd.Flags().String(flagComponent, "", "")
	require.NoError(t, runPluginInstall(cmd, []string{"diff@v3.9.4", "secrets"}))
	assert.Equal(t, "/bin/helm", gotHelm)
	assert.Equal(t, []string{"diff@v3.9.4", "secrets"}, gotSpecs)
}

func TestRunPluginInstallValidationAndErrors(t *testing.T) {
	withPluginCommandSeams(t)

	cmd := &cobra.Command{Use: "install"}
	cmd.Flags().String(flagComponent, "", "")
	err := runPluginInstall(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidHelmPluginSpec)

	sentinel := errors.New("install failed")
	resolveHelmBinaryForPlugin = func(*cobra.Command) (string, error) { return "/bin/helm", nil }
	ensureHelmPluginsForComponent = func(context.Context, string, []string) (string, error) {
		return "", sentinel
	}
	require.ErrorIs(t, runPluginInstall(cmd, []string{"diff"}), sentinel)
}

func TestCollectInstallSpecsFromComponent(t *testing.T) {
	withPluginCommandSeams(t)

	cmd := &cobra.Command{Use: "install"}
	cmd.Flags().String(flagComponent, "app", "")
	cmd.Flags().String("stack", "dev", "")

	initHelmCliConfig = func(info schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
		assert.Equal(t, "dev", info.Stack)
		assert.True(t, processStacks)
		return schema.AtmosConfiguration{}, nil
	}
	describeHelmComponent = func(params *e.ExecuteDescribeComponentParams) (map[string]any, error) {
		assert.Equal(t, "app", params.Component)
		assert.Equal(t, "dev", params.Stack)
		assert.True(t, params.ProcessTemplates)
		assert.True(t, params.ProcessYamlFunctions)
		return map[string]any{"plugins": []any{"diff@v3.9.4", "secrets", nil, 123}}, nil
	}

	specs, err := collectInstallSpecs(cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"diff@v3.9.4", "secrets", "123"}, specs)
}

func TestCollectInstallSpecsFromComponentErrors(t *testing.T) {
	withPluginCommandSeams(t)

	cmd := &cobra.Command{Use: "install"}
	cmd.Flags().String(flagComponent, "app", "")
	_, err := collectInstallSpecs(cmd, nil)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)

	cmd.Flags().String("stack", "dev", "")
	sentinel := errors.New("config failed")
	initHelmCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, sentinel
	}
	_, err = collectInstallSpecs(cmd, nil)
	assert.ErrorIs(t, err, sentinel)

	initHelmCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	describeHelmComponent = func(*e.ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, sentinel
	}
	_, err = collectInstallSpecs(cmd, nil)
	assert.ErrorIs(t, err, sentinel)
}
