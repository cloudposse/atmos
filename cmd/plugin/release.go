package plugin

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginbuild "github.com/cloudposse/atmos/pkg/plugin/build"
	"github.com/cloudposse/atmos/pkg/plugin/conformance"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
	pluginrelease "github.com/cloudposse/atmos/pkg/plugin/release"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/ui"
)

func releaseCommand() *cobra.Command {
	parser := newOutputParser(publicationFlags()...)
	command := &cobra.Command{
		Use:   "release [directory] --to <repository>",
		Short: "Build all targets, test the host target, and publish a plugin release",
		Args:  cobra.MaximumNArgs(1),
	}
	registerOutputFlags(command, parser)
	// Required validation lives in RunE so release plan does not inherit --to.
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values, err := parseOutputFlags(cmd, parser)
		if err != nil {
			return err
		}
		if err := rejectOrphanSigningFlags(cmd, values); err != nil {
			return err
		}
		if _, err := pluginoci.ParseDestination(values.GetString(publishDestinationFlag)); err != nil {
			return err
		}
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		directory := "."
		if len(args) != 0 {
			directory = args[0]
		}
		backend := &releaseBackend{manager: manager, values: values}
		result, err := pluginrelease.Run(cmd.Context(), directory, backend)
		if err != nil {
			return err
		}
		return presentPublishedRelease(result, values)
	}
	command.AddCommand(releasePlanCommand())
	return command
}

type releaseBackend struct {
	manager *manage.Manager
	values  *viper.Viper
}

func (backend *releaseBackend) Validate(_ context.Context, directory string) error {
	defer perf.Track(nil, "plugin.releaseBackend.Validate")()

	definition, err := manifest.Load(directory)
	if err != nil {
		return err
	}
	if err := pluginoci.ValidatePackage(directory, definition); err != nil {
		return err
	}
	_, err = backend.manager.Installer.Descriptors.Validate(directory, definition.Spec.Capabilities)
	return err
}

func (backend *releaseBackend) Build(ctx context.Context, directory string) error {
	defer perf.Track(nil, "plugin.releaseBackend.Build")()

	streams := atmosio.GetContext()
	_, err := pluginbuild.CompileAll(ctx, directory, pluginbuild.WithAllPlatforms(true),
		pluginbuild.WithStreams(process.Streams{Stdout: streams.UI(), Stderr: streams.UI()}))
	return err
}

func (backend *releaseBackend) Test(ctx context.Context, directory string) error {
	defer perf.Track(nil, "plugin.releaseBackend.Test")()

	ui.Info("Running conformance and authored tests on the current host; other platforms are statically verified during packaging.")
	streams := atmosio.GetContext()
	report, err := conformance.Run(ctx, directory, conformance.Options{
		Project: projectDirectory(), Stdout: streams.UI(), Stderr: streams.UI(), OnResult: conformance.Show,
	})
	return conformance.Present(report, err, conformance.FormatText)
}

func (backend *releaseBackend) Pack(_ context.Context, directory, layout string) error {
	return pluginoci.Pack(directory, layout, backend.manager.Installer.Descriptors, pluginoci.WithExecutableFormatCheck())
}

func (backend *releaseBackend) Publish(ctx context.Context, layout string) (*pluginoci.Release, error) {
	return publishLayout(ctx, backend.manager, layout, backend.values)
}

// publicationFlags is shared by publication of a prepared layout and source releases.
func publicationFlags() []flags.Option {
	return append(signingFlags(),
		flags.WithBoolFlag(signFlag, "", false, "Sign the published release with Sigstore"),
		flags.WithBoolFlag(verifyPublicFlag, "", false, "Require public GHCR organization package visibility and verify every published payload anonymously"),
		flags.WithStringFlag(publishDestinationFlag, "", "", "Destination OCI repository"))
}

func presentPublishedRelease(release *pluginoci.Release, values *viper.Viper) error {
	files, err := release.Files()
	if err != nil {
		return err
	}
	return pluginoutput.Published(pluginoutput.PublishedRelease{
		Release: release, Repository: values.GetString(publishDestinationFlag), Signed: values.GetBool(signFlag), Files: files,
	}, values.GetString(outputFormatFlag))
}
