package plugin

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
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
	parser := newOutputParser(append(publicationFlags(), inventoryReleaseFlags()...)...)
	command := &cobra.Command{
		Use:   "release [directory] [--to <repository> | --inventory <file>]",
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
		if err := validateReleaseMode(values, args); err != nil {
			return err
		}
		if values.GetString(releaseInventoryFlag) != "" {
			return runInventoryRelease(cmd, values)
		}
		return runDirectoryRelease(cmd, args, values)
	}
	command.AddCommand(releasePlanCommand())
	return command
}

func runDirectoryRelease(cmd *cobra.Command, args []string, values *viper.Viper) error {
	if !values.GetBool(releaseCheckFlag) {
		if _, err := pluginoci.ParseDestination(values.GetString(publishDestinationFlag)); err != nil {
			return err
		}
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
	result, err := pluginrelease.Run(cmd.Context(), directory, backend, pluginrelease.WithCheck(values.GetBool(releaseCheckFlag)))
	if err != nil {
		return err
	}
	if values.GetBool(releaseCheckFlag) {
		ui.Success("Plugin release checks passed.")
		return nil
	}
	return presentPublishedRelease(result, values)
}

type releaseBackend struct {
	manager *manage.Manager
	values  *viper.Viper
	project string
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
	project := backend.project
	if project == "" {
		project = projectDirectory()
	}
	report, err := conformance.Run(ctx, directory, conformance.Options{
		Project: project, Stdout: streams.UI(), Stderr: streams.UI(), OnResult: conformance.Show,
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

func validateReleaseMode(values *viper.Viper, args []string) error {
	inventory := values.GetString(releaseInventoryFlag) != ""
	if inventory && (len(args) > 0 || values.GetString(publishDestinationFlag) != "") {
		return fmt.Errorf("%w: --inventory supplies package paths and repositories; omit directory and --to", errUtils.ErrPluginFlagConflict)
	}
	if incompatibleCheckMode(values) {
		return fmt.Errorf("%w: --check cannot sign, verify public access, or reconcile a catalog", errUtils.ErrPluginFlagConflict)
	}
	if !inventory && (values.GetString("catalog") != "" || values.GetString("base") != "" || values.GetString("head") != "HEAD") {
		return fmt.Errorf("%w: --catalog, --base, and --head require --inventory", errUtils.ErrPluginFlagConflict)
	}
	return nil
}

func incompatibleCheckMode(values *viper.Viper) bool {
	return values.GetBool(releaseCheckFlag) && (values.GetBool(signFlag) || values.GetBool(verifyPublicFlag) || values.GetString("catalog") != "")
}
