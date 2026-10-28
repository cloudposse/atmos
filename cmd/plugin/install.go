package plugin

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
)

var atmosConfig *schema.AtmosConfiguration

// SetAtmosConfig supplies the resolved project configuration to lifecycle commands.
func SetAtmosConfig(config *schema.AtmosConfiguration) { atmosConfig = config }

func installCommand() *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStringFlag("as", "", "", "Package alias"),
		flags.WithStringFlag("version", "", "", "Release version constraint"),
		flags.WithStringFlag("from-layout", "", "", "Restore from a local OCI layout"),
		flags.WithBoolFlag("frozen", "", false, "Require exact declaration and lock agreement"),
	)
	command := &cobra.Command{Use: "install [source]", Short: "Install an Atmos plugin", Args: cobra.MaximumNArgs(1)}
	parser.RegisterFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		if atmosConfig == nil {
			return errNoProject()
		}
		return installRelease(cmd, args, values)
	}
	return command
}

func errNoProject() error {
	return errUtils.Build(errUtils.ErrPluginInvalidConfig).
		WithHint("Run this command from a directory that has an `atmos.yaml`, or pass `--config`.").
		Err()
}

func installRelease(cmd *cobra.Command, args []string, values *viper.Viper) error {
	if len(args) == 0 {
		return restoreDeclared(cmd, values)
	}
	if values.GetBool("frozen") {
		return errUtils.Build(fmt.Errorf("%w: --frozen cannot be combined with a source", errUtils.ErrPluginFlagConflict)).
			WithHint("`--frozen` restores exactly what `.atmos.plugins.lock.yaml` records and never adds a package. Run `atmos plugin install --frozen` without a source, or drop `--frozen`.").
			Err()
	}
	source := args[0]
	if err := install.CheckReleaseSource(source); err != nil {
		return err
	}
	manager, err := newManager(cmd)
	if err != nil {
		return err
	}
	options := manage.AddOptions{Alias: values.GetString("as"), Source: source, Constraint: values.GetString("version"), Layout: values.GetString("from-layout")}
	if !strings.HasPrefix(options.Source, "oci://") {
		selection := options.Constraint
		if selection == "" {
			selection = "*"
		}
		entry, version, err := resolveDiscoveredPackage(cmd, manager, options.Source, selection)
		if err != nil {
			return err
		}
		options.Source, options.PinnedDigest, options.SelectedVersion = entry.Source, version.Digest, version.Version
	}
	return manager.Add(cmd.Context(), options)
}

// restoreDeclared installs every enabled declaration. A source-less install has nothing to apply --as or --version to.
func restoreDeclared(cmd *cobra.Command, values *viper.Viper) error {
	var ignored []string
	if values.GetString("as") != "" {
		ignored = append(ignored, "--as")
	}
	if values.GetString("version") != "" {
		ignored = append(ignored, "--version")
	}
	if len(ignored) != 0 {
		return errUtils.Build(fmt.Errorf("%w: %s needs a source to install", errUtils.ErrPluginFlagConflict, strings.Join(ignored, " and "))).
			WithHint("Pass a source such as `atmos plugin install oci://registry.example.com/acme/oidc --as oidc`, or drop the flags to restore every declared package.").
			Err()
	}
	manager, err := newManager(cmd)
	if err != nil {
		return err
	}
	return manager.Restore(cmd.Context(), manage.RestoreOptions{Frozen: values.GetBool("frozen"), Layout: values.GetString("from-layout")})
}
