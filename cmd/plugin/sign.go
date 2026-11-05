package plugin

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
	pluginsign "github.com/cloudposse/atmos/pkg/plugin/sign"
)

const cosignVersionFlag = "cosign-version"

func signRelease(ctx context.Context, manager *manage.Manager, reference string, values *viper.Viper) (*pluginoci.Release, error) {
	signer := pluginsign.Cosign{Config: manager.Config, Version: values.GetString(cosignVersionFlag), Yes: values.GetBool("yes")}
	service := pluginsign.Service{Registry: manager.Installer.Registry, Signer: signer, Descriptors: manager.Installer.Descriptors}
	return service.Release(ctx, reference)
}

func signingFlags() []flags.Option {
	return []flags.Option{
		flags.WithStringFlag(cosignVersionFlag, "", pluginsign.DefaultVersion, "Exact managed Cosign 3 version"),
		flags.WithBoolFlag("yes", "y", false, "Accept Sigstore transparency log publication without prompting"),
	}
}

func signCommand() *cobra.Command {
	parser := newOutputParser(signingFlags()...)
	command := &cobra.Command{Use: "sign <repository@sha256:digest>", Short: "Sign an immutable plugin release with Sigstore", Args: cobra.ExactArgs(1)}
	registerOutputFlags(command, parser)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values, err := parseOutputFlags(cmd, parser)
		if err != nil {
			return err
		}
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		release, err := signRelease(cmd.Context(), manager, args[0], values)
		if err != nil {
			return err
		}
		return pluginoutput.Signed(release, args[0], values.GetString(outputFormatFlag))
	}
	return command
}
