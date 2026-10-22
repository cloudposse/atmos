package plugin

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/registryauth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	pluginconfig "github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	registryUsernameFlag = "username"
	identityFlag         = "identity"
)

func registryCommand() *cobra.Command {
	command := &cobra.Command{Use: "registry", Short: "Manage plugin discovery sources and registry authentication"}
	command.AddCommand(registryAddCommand(), registryRemoveCommand(), registryListCommand(), registryLoginCommand(), registryLogoutCommand())
	return command
}

func registryAddCommand() *cobra.Command {
	// The global --identity/-i flag (and ATMOS_IDENTITY) selects the registry's identity; a local
	// flag of the same name would shadow it and lose the shorthand and environment variable.
	parser := flags.NewStandardParser(
		flags.WithIdentityFlag(),
		flags.WithStringFlag("host", "", "", "Registry host for authentication, independent of discovery"),
		flags.WithStringFlag("catalog", "", "", "Optional OCI discovery catalog reference"),
		flags.WithStringSliceFlag("repository", "", nil, "OCI repository to index (repeatable)"),
	)
	command := &cobra.Command{Use: "add <name> [catalog]", Short: "Configure registry authentication and optional discovery sources", Args: cobra.RangeArgs(1, 2)}
	parser.RegisterFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		return addRegistry(cmd, args, values)
	}
	return command
}

func addRegistry(cmd *cobra.Command, args []string, values *viper.Viper) error {
	registry, err := registryFromFlags(args, values)
	if err != nil {
		return err
	}
	manager, err := newManager(cmd)
	if err != nil {
		return err
	}
	if registry.Identity == "" {
		registry.Auth = "docker"
	} else if _, exists := atmosConfig.Auth.Identities[registry.Identity]; !exists {
		return errUtils.Build(errUtils.ErrInvalidIdentityConfig).
			WithExplanationf("Registry identity %q is not configured.", registry.Identity).
			WithHint("Define the identity in `auth.identities` before adding the registry.").Err()
	}
	replaced, existed := atmosConfig.Plugins.Registries[args[0]]
	if err := manager.EditRegistry(cmd.Context(), args[0], &registry); err != nil {
		return err
	}
	if existed {
		ui.Warningf("Replaced existing plugin registry **%s** (%s) with the new configuration.", args[0], describeRegistry(&replaced))
	}
	host, err := pluginconfig.RegistryHost(&registry)
	if err != nil {
		return err
	}
	ui.Successf("Added plugin registry **%s** for `%s`.", args[0], host)
	if registry.Catalog != "" {
		ui.Infof("Catalog: `%s`", registry.Catalog)
	}
	for _, repository := range registry.Repositories {
		ui.Infof("Repository to index: `%s`", repository)
	}
	return nil
}

func registryFromFlags(args []string, values *viper.Viper) (schema.PluginRegistry, error) {
	identity, err := registryIdentityFlag(values.GetString(identityFlag))
	if err != nil {
		return schema.PluginRegistry{}, err
	}
	registry := schema.PluginRegistry{
		Host: values.GetString("host"), Catalog: values.GetString("catalog"),
		Repositories: values.GetStringSlice("repository"), Identity: identity,
	}
	if len(args) == 2 {
		if registry.Catalog != "" {
			return registry, errUtils.Build(errUtils.ErrPluginInvalidConfig).
				WithExplanation("Supply the catalog as a positional argument or with `--catalog`, not both.").
				WithHint("Drop the positional catalog argument, or drop `--catalog`.").Err()
		}
		registry.Catalog = args[1]
	}
	return registry, nil
}

// registryIdentityFlag interprets the shared --identity value. A registry binds to one named
// identity, so interactive selection is not meaningful, and a disabled sentinel (for example
// ATMOS_IDENTITY=false) means no identity.
func registryIdentityFlag(value string) (string, error) {
	switch cfg.NormalizeIdentityValue(value) {
	case cfg.IdentityFlagDisabledValue:
		return "", nil
	case cfg.IdentityFlagSelectValue:
		return "", errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanation("`--identity` needs the name of the identity that authenticates to this registry.").
			WithHint("Pass `--identity=<name>` (or set `ATMOS_IDENTITY`) with an identity defined in `auth.identities`.").Err()
	default:
		return value, nil
	}
}

// describeRegistry summarizes what a registry entry pointed at, for replacement warnings.
func describeRegistry(registry *schema.PluginRegistry) string {
	switch {
	case registry.Catalog != "":
		return "catalog `" + registry.Catalog + "`"
	case registry.Host != "":
		return "host `" + registry.Host + "`"
	case len(registry.Repositories) > 0:
		return "repository `" + registry.Repositories[0] + "`"
	default:
		return "no discovery source"
	}
}

func registryRemoveCommand() *cobra.Command {
	return &cobra.Command{Use: "remove <name>", Short: "Remove a registry configuration without removing installed plugins or credentials", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		if err := manager.EditRegistry(cmd.Context(), args[0], nil); err != nil {
			return err
		}
		ui.Successf("Removed plugin registry **%s**.", args[0])
		return nil
	}}
}

func registryListCommand() *cobra.Command {
	parser := newOutputParser()
	command := &cobra.Command{Use: "list", Short: "List registry hosts, discovery sources, and identities", Args: cobra.NoArgs}
	registerOutputFlags(command, parser)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values, err := parseOutputFlags(cmd, parser)
		if err != nil {
			return err
		}
		if atmosConfig == nil {
			return errMissingConfiguration()
		}
		return pluginoutput.Registries(atmosConfig.Plugins.Registries, values.GetString("format"))
	}
	return command
}

func registryLoginCommand() *cobra.Command {
	parser := flags.NewStandardParser(flags.WithStringFlag(registryUsernameFlag, "u", "", "Registry username"), flags.WithBoolFlag("password-stdin", "", false, "Read the registry password or token from standard input"))
	command := &cobra.Command{Use: "login <registry-or-alias>", Short: "Authenticate a registry's Atmos identity, or store explicit registry credentials", Args: cobra.ExactArgs(1)}
	parser.RegisterFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		host, err := manage.ResolveRegistryHost(atmosConfig, args[0])
		if err != nil {
			return err
		}
		if err := pluginpolicy.Login(cmd.Context(), host); err != nil {
			return err
		}
		identity, err := registryIdentity(host)
		if err != nil {
			return err
		}
		if identity != "" {
			return loginRegistryIdentity(cmd.Context(), host, identity, values)
		}
		if !values.GetBool("password-stdin") || values.GetString(registryUsernameFlag) == "" {
			return errUtils.Build(errUtils.ErrPluginInvalidConfig).
				WithExplanation("This registry has no Atmos auth identity configured.").
				WithHint("Configure `plugins.registries.<name>.identity`, or supply `--username` and `--password-stdin`.").Err()
		}
		credentials := manage.RegistryCredentials{Auth: &atmosConfig.Auth}
		if err := credentials.Store(host, values.GetString(registryUsernameFlag)); err != nil {
			return err
		}
		ui.Successf("Stored registry credentials for `%s`.", host)
		return nil
	}
	return command
}

func loginRegistryIdentity(ctx context.Context, host, identity string, values *viper.Viper) error {
	if values.GetBool("password-stdin") || values.GetString(registryUsernameFlag) != "" {
		return errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanationf("Registry %q uses Atmos identity %q.", host, identity).
			WithHintf("Run `atmos auth login --identity %s`, or omit `--username` and `--password-stdin`.", identity).Err()
	}
	if _, err := registryauth.Authenticate(ctx, atmosConfig, identity, host, registryauth.WithPreparation(prepareRegistryAuth)); err != nil {
		return err
	}
	ui.Successf("Authenticated identity **%s** for registry `%s`.", identity, host)
	return nil
}

func registryLogoutCommand() *cobra.Command {
	return &cobra.Command{Use: "logout <registry-or-alias>", Short: "Log out a registry's Atmos identity, or remove explicit registry credentials", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		host, err := manage.ResolveRegistryHost(atmosConfig, args[0])
		if err != nil {
			return err
		}
		identity, err := registryIdentity(host)
		if err != nil {
			return err
		}
		if identity != "" {
			if err := registryauth.Logout(cmd.Context(), atmosConfig, identity, registryauth.WithPreparation(prepareRegistryAuth)); err != nil {
				return err
			}
			ui.Successf("Logged out identity **%s** used by registry `%s`.", identity, host)
			return nil
		}
		credentials := manage.RegistryCredentials{Auth: &atmosConfig.Auth}
		if err := credentials.Remove(host); err != nil {
			return err
		}
		ui.Successf("Removed registry credentials for `%s`.", host)
		return nil
	}}
}

func registryIdentity(host string) (string, error) {
	return registryauth.Identity(atmosConfig, host)
}
