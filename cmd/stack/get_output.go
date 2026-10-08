package stack

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/ui"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// getFormatParsers holds the --format parser of each read command, so `stack get` and
// `stack config get` each resolve flag > environment variable > default for their own variable.
var getFormatParsers = map[*cobra.Command]getFormatBinding{}

// getFormatBinding is a read command's parser and the Viper key it stores --format under. Each
// command has its own key so one command's environment variable never changes another's output.
type getFormatBinding struct {
	parser *flags.StandardParser
	key    string
}

// Adds --format/-f (default raw, or json) to a read command with registerGetFormat. The envVar
// argument is the environment variable that sets it when the flag is absent.
func registerGetFormat(c *cobra.Command, viperPrefix, envVar string) {
	parser := flags.NewStandardParser(
		flags.WithViperPrefix(viperPrefix),
		flags.WithStringFlag("format", "f", "raw", "Output format: raw or json"),
		flags.WithEnvVars("format", envVar),
	)
	parser.RegisterFlags(c)
	if err := parser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	getFormatParsers[c] = getFormatBinding{parser: parser, key: viperPrefix + ".format"}
}

// getFormat resolves the output format of a read command.
func getFormat(cmd *cobra.Command) (string, error) {
	binding, ok := getFormatParsers[cmd]
	if !ok {
		return cmd.Flags().GetString("format")
	}
	v := viper.GetViper()
	if err := binding.parser.BindFlagsToViper(cmd, v); err != nil {
		return "", err
	}
	return v.GetString(binding.key), nil
}

func runStackGetCommand(cmd *cobra.Command, args []string) error {
	format, err := getFormat(cmd)
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
