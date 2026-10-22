// Package plugin provides the Atmos plugin command family.
package plugin

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/mod/semver"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/plugin/scaffold"
	"github.com/cloudposse/atmos/pkg/version"
)

var pluginCmd = &cobra.Command{Use: "plugin", Short: "Create, install, and manage Atmos plugins"}

func init() {
	pluginCmd.AddCommand(buildCommand(), testCommand(), runCommand(), createCommand(), installCommand(), validateCommand(), packCommand(), updateCommand(), publishCommand(), signCommand(), deleteCommand(), removeCommand(), listCommand(), infoCommand(), registryCommand(), catalogCommand(), searchCommand())
	internal.Register(&CommandProvider{})
}

func createCommand() *cobra.Command {
	sdkVersion := "v" + strings.TrimPrefix(version.Version, "v")
	if !semver.IsValid(sdkVersion) {
		sdkVersion = ""
	}
	parser := flags.NewStandardParser(
		flags.WithStringFlag("kind", "", "example/oidc", "Provider kind (organization/name)"),
		flags.WithStringFlag("display-name", "", "", "Human-readable plugin name"),
		flags.WithStringFlag("description", "", "", "Plugin description"),
		flags.WithStringFlag("author", "", "", "Plugin author"),
		flags.WithStringFlag("repository", "", "", "Plugin source repository URL"),
		flags.WithStringFlag("module", "", "example.com/atmos-oidc", "Generated Go module path"),
		flags.WithStringFlag("runtime", "", "go", "Plugin authoring runtime"),
		flags.WithStringFlag("capability", "", "auth.provider", "Primary extension capability"),
		flags.WithStringFlag("sdk-version", "", sdkVersion, "Plugin SDK module version"),
		flags.WithStringFlag("sdk-path", "", "", "Local SDK checkout for development"),
		flags.WithBoolFlag("force", "", false, "Allow overwriting generated files"),
		flags.WithBoolFlag("defaults", "", false, "Use scaffold field defaults without prompting"),
	)
	command := &cobra.Command{Use: "create <directory>", Short: "Scaffold a Go OIDC auth plugin", Args: cobra.ExactArgs(1)}
	parser.RegisterFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		return scaffold.Create(cmd.Context(), args[0], scaffold.Options{
			Name: values.GetString("kind"), DisplayName: values.GetString("display-name"),
			Description: values.GetString("description"), Author: values.GetString("author"), Repository: values.GetString("repository"),
			Module: values.GetString("module"), Runtime: values.GetString("runtime"),
			Capability: values.GetString("capability"), SDKVersion: values.GetString("sdk-version"), SDKPath: values.GetString("sdk-path"),
			Force: values.GetBool("force"), Defaults: values.GetBool("defaults"),
		})
	}
	return command
}

// CommandProvider registers plugin commands with the standard Atmos command registry.
type CommandProvider struct{}

func (*CommandProvider) GetCommand() *cobra.Command { return pluginCmd }

func (*CommandProvider) GetName() string { return "plugin" }

func (*CommandProvider) GetGroup() string                                           { return "Other Commands" }
func (*CommandProvider) GetFlagsBuilder() flags.Builder                             { return nil }
func (*CommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder     { return nil }
func (*CommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag { return nil }
func (*CommandProvider) GetAliases() []internal.CommandAlias                        { return nil }
func (*CommandProvider) IsExperimental() bool                                       { return true }
