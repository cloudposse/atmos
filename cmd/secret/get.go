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
		flags.WithBoolFlag("raw", "r", false, "Print the raw value with no trailing newline (text only; ideal for piping, e.g. | pbcopy)"),
	)
	getParser.RegisterFlags(getCmd)
}

func runSecretGet(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretGet")()

	scope, err := parseScope(cmd, args)
	if err != nil {
		return err
	}
	name := args[0]
	format, _ := cmd.Flags().GetString("format")
	path, _ := cmd.Flags().GetString("path")
	raw, _ := cmd.Flags().GetBool("raw")

	if err := validateGetFlags(raw, cmd.Flags().Changed("format"), format); err != nil {
		return err
	}

	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	value, err := svc.Get(name, secrets.ResolveOptions{Path: path})
	if err != nil {
		return err
	}

	return writeSecretValue(name, value, format, raw)
}

// validateGetFlags rejects mutually exclusive flag combinations. --raw is text-only, so an
// explicit non-text --format is a conflict (rather than silently ignored).
func validateGetFlags(raw, formatChanged bool, format string) error {
	if raw && formatChanged && format != "text" {
		return ErrRawFormatConflict
	}
	return nil
}

// renderSecretValue formats a secret value for output. When raw is set, it returns the bare value
// with newline=false (no trailing newline, text only) so piping it (e.g. `| pbcopy`) does not
// capture a newline; otherwise it renders per format with newline=true.
func renderSecretValue(name string, value any, format string, raw bool) (content string, newline bool, err error) {
	if raw {
		return fmt.Sprintf("%v", value), false, nil
	}
	switch format {
	case "json":
		b, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return "", false, marshalErr
		}
		return string(b), true, nil
	case "env":
		return fmt.Sprintf("%s=%v", name, value), true, nil
	default:
		return fmt.Sprintf("%v", value), true, nil
	}
}

// writeSecretValue renders a single secret value and writes it to the masked data channel; the
// value is revealed only when masking is disabled (--mask=false).
func writeSecretValue(name string, value any, format string, raw bool) error {
	content, newline, err := renderSecretValue(name, value, format, raw)
	if err != nil {
		return err
	}
	if newline {
		return data.Writeln(content)
	}
	return data.Write(content)
}
