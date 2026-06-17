package helm

import (
	"context"
	"sort"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/dependencies"
	helmplugin "github.com/cloudposse/atmos/pkg/helm/plugin"
	"github.com/cloudposse/atmos/pkg/ui"
)

const flagComponent = "component"

func init() {
	pluginCmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage Helm CLI plugins",
		Long:  "Install and list Helm CLI plugins (e.g. helm-diff, helm-secrets) in the Atmos-managed HELM_PLUGINS directory used by helm and helmfile components.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Usage()
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List installed Helm plugins",
		Args:  cobra.NoArgs,
		RunE:  runPluginList,
	}

	installCmd := &cobra.Command{
		Use:   "install [plugin...]",
		Short: "Install Helm plugins",
		Long:  "Install one or more Helm plugins by spec (e.g. `diff@v3.9.4`, `databus23/helm-diff@v3.9.4`, or a full URL), or install the plugins declared by a component with --component and --stack.",
		RunE:  runPluginInstall,
	}
	installCmd.Flags().String(flagComponent, "", "Install the plugins declared by this component (requires --stack).")

	pluginCmd.AddCommand(listCmd)
	pluginCmd.AddCommand(installCmd)
	helmCmd.AddCommand(pluginCmd)
}

// runPluginList lists the plugins installed in the managed HELM_PLUGINS directory.
func runPluginList(cmd *cobra.Command, _ []string) error {
	helmBin, err := resolveHelmBinary(cmd)
	if err != nil {
		return err
	}

	installed, err := helmplugin.NewInstaller(helmBin).ListInstalled(context.Background())
	if err != nil {
		return err
	}

	if len(installed) == 0 {
		ui.Info("No Helm plugins installed in the Atmos-managed directory")
		return nil
	}

	names := make([]string, 0, len(installed))
	for name := range installed {
		names = append(names, name)
	}
	sort.Strings(names)

	if err := data.Writef("%-24s %s\n", "NAME", "VERSION"); err != nil {
		return err
	}
	for _, name := range names {
		if err := data.Writef("%-24s %s\n", name, installed[name]); err != nil {
			return err
		}
	}
	return nil
}

// runPluginInstall installs explicit plugin specs, or the plugins declared by a
// component when --component and --stack are provided.
func runPluginInstall(cmd *cobra.Command, args []string) error {
	specs, err := collectInstallSpecs(cmd, args)
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		return errUtils.Build(errUtils.ErrInvalidHelmPluginSpec).
			WithExplanation("No plugins specified to install").
			WithHint("Pass plugin specs (e.g. `atmos helm plugin install diff@v3.9.4`) or use --component and --stack").
			Err()
	}

	helmBin, err := resolveHelmBinary(cmd)
	if err != nil {
		return err
	}

	if _, err := helmplugin.EnsureForComponent(context.Background(), helmBin, specs); err != nil {
		return err
	}
	ui.Success("Helm plugins are installed")
	return nil
}

// collectInstallSpecs returns the plugin specs to install: explicit args take
// precedence; otherwise the declared plugins of the --component in --stack.
func collectInstallSpecs(cmd *cobra.Command, args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}

	componentName, _ := cmd.Flags().GetString(flagComponent)
	if componentName == "" {
		return nil, nil
	}

	info := buildConfigAndStacksInfo(cmd)
	if info.Stack == "" {
		return nil, errUtils.ErrStackNotFound
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return nil, err
	}

	section, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		AtmosConfig:          &atmosConfig,
		Component:            componentName,
		Stack:                info.Stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	if err != nil {
		return nil, err
	}

	return helmplugin.ExtractSpecs(section), nil
}

// resolveHelmBinary resolves the helm binary path, installing it via the
// toolchain when declared as a dependency, and falling back to PATH.
func resolveHelmBinary(cmd *cobra.Command) (string, error) {
	info := buildConfigAndStacksInfo(cmd)
	atmosConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return "", err
	}

	tenv, err := dependencies.ForComponent(&atmosConfig, cfg.HelmfileComponentType, nil, nil)
	if err != nil {
		return "", err
	}
	return tenv.Resolve("helm"), nil
}
