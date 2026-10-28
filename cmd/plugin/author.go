package plugin

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
)

const packOutputFlag = "output"

func validateCommand() *cobra.Command {
	parser := newOutputParser()
	command := &cobra.Command{Use: "validate [directory]", Short: "Validate package metadata and extension schemas without executing code", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		values, err := parseOutputFlags(cmd, parser)
		if err != nil {
			return err
		}
		directory := "."
		if len(args) != 0 {
			directory = args[0]
		}
		definition, err := manifest.Load(directory)
		if err != nil {
			return err
		}
		// Static guards run first and never execute plugin code or require a built binary.
		if err := pluginoci.ValidatePackage(directory, definition); err != nil {
			return err
		}
		registry, err := bootstrap.Descriptors()
		if err != nil {
			return err
		}
		bindings, err := registry.Validate(directory, definition.Spec.Capabilities)
		if err != nil {
			return err
		}
		return pluginoutput.Validation(definition, bindings, values.GetString(outputFormatFlag))
	}}
	registerOutputFlags(command, parser)
	return command
}

func packCommand() *cobra.Command {
	parser := newOutputParser(flags.WithStringFlag(packOutputFlag, "o", "", "Destination OCI layout directory"))
	command := &cobra.Command{Use: "pack [directory] --output <layout>", Short: "Pack precompiled plugins into a multi-platform OCI layout", Args: cobra.MaximumNArgs(1)}
	registerOutputFlags(command, parser)
	_ = command.MarkFlagRequired(packOutputFlag)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values, err := parseOutputFlags(cmd, parser)
		if err != nil {
			return err
		}
		directory := "."
		if len(args) != 0 {
			directory = args[0]
		}
		registry, err := bootstrap.Descriptors()
		if err != nil {
			return err
		}
		definition, err := manifest.Load(directory)
		if err != nil {
			return err
		}
		output := values.GetString(packOutputFlag)
		if err := pluginoci.Pack(directory, output, registry, pluginoci.WithExecutableFormatCheck()); err != nil {
			return err
		}
		files, err := pluginoci.LayoutFiles(output)
		if err != nil {
			return err
		}
		return pluginoutput.Packed(definition, output, files, values.GetString(outputFormatFlag))
	}
	return command
}
