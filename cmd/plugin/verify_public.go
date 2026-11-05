package plugin

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/plugin/publicaccess"
	"github.com/cloudposse/atmos/pkg/ui"
)

func newPublicVerifier() *publicaccess.Client {
	values := viper.New()
	values.AutomaticEnv()
	token := values.GetString("GH_TOKEN")
	if token == "" {
		token = values.GetString("GITHUB_TOKEN")
	}
	return publicaccess.New(publicaccess.WithGitHubToken(token))
}

func verifyPublicCommand() *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStringFlag("reference", "", "", "Tag or digest to download anonymously; omitted checks GHCR organization package visibility"),
		flags.WithStringFlag("digest", "", "", "Expected root digest for anonymous verification"),
	)
	command := &cobra.Command{Use: "verify-public <repository>", Short: "Check public visibility or anonymously verify every OCI platform", Args: cobra.ExactArgs(1)}
	parser.RegisterFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		client := newPublicVerifier()
		reference, expected := values.GetString("reference"), values.GetString("digest")
		if reference == "" {
			if expected != "" {
				return errUtils.ErrPluginFlagConflict
			}
			if err := client.RequirePublic(cmd.Context(), args[0]); err != nil {
				return err
			}
			ui.Successf("GHCR package `%s` is public.", args[0])
			return nil
		}
		digest, err := client.Verify(cmd.Context(), args[0], reference, expected)
		if err != nil {
			return err
		}
		ui.Successf("All OCI content for `%s@%s` is anonymously readable.", args[0], digest)
		return nil
	}
	return command
}
