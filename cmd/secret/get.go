package secret

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets"
)

var getParser *flags.StandardParser

var getCmd = &cobra.Command{
	Use:   "get NAME",
	Short: "Retrieve a declared secret's value.",
	Long:  "Retrieve a declared secret's value from its backend. Use --path to extract a nested value from a structured secret.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSecretGet,
}

func init() {
	getParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "", "text", "Output format: text, json, env"),
		flags.WithStringFlag("path", "", "", "Extract a nested value from a structured secret (YQ path, e.g. .host)"),
	)
	getParser.RegisterFlags(getCmd)
}

func runSecretGet(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretGet")()

	scope, err := parseScope(cmd)
	if err != nil {
		return err
	}
	name := args[0]
	format, _ := cmd.Flags().GetString("format")
	path, _ := cmd.Flags().GetString("path")

	svc, err := loadService(scope)
	if err != nil {
		return err
	}

	value, err := svc.Get(name, secrets.ResolveOptions{Path: path})
	if err != nil {
		return err
	}

	return writeSecretValue(name, value, format)
}

// writeSecretValue renders a single secret value in the requested format. Output goes through
// the masked data channel; the value is revealed only when masking is disabled (--mask=false).
func writeSecretValue(name string, value any, format string) error {
	switch format {
	case "json":
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return data.Writeln(string(b))
	case "env":
		return data.Writeln(fmt.Sprintf("%s=%v", name, value))
	default:
		return data.Writeln(fmt.Sprintf("%v", value))
	}
}
