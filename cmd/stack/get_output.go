package stack

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/ui"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

func runStackGetCommand(cmd *cobra.Command, args []string) error {
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	if format == "raw" {
		return runStackGet(args)
	}
	return runStackGetFormat(args, format)
}

func runStackGetFormat(args []string, format string) error {
	tgt, err := resolveEditTarget(args[0], false)
	if err != nil {
		return err
	}
	if tgt.provFile != "" {
		ui.Infof("%s resolves from %s:%d", args[0], tgt.provFile, tgt.provLine)
	}
	if format == "raw" {
		return data.Writeln(tgt.value)
	}
	value, err := atmosyaml.GetFormatted(tgt.valueContent, tgt.valuePath, format)
	if err != nil {
		return err
	}
	return data.Writeln(value)
}
