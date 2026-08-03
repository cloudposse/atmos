package store

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var getParser *flags.StandardParser

var getCmd = &cobra.Command{
	Use:   "get STORE KEY",
	Short: "Retrieve a value from a store.",
	Long: "Retrieve a value from a configured store backend, scoped to --stack/--component the " +
		"same way `atmos store set` writes it (an omitted --stack/--component reads back a " +
		"value set without them).",
	Args: cobra.ExactArgs(2),
	RunE: runStoreGet,
}

func init() {
	getParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "", "text", "Output format: text, json, env"),
		flags.WithBoolFlag("raw", "r", false, "Print the raw value with no trailing newline (text only; ideal for piping, e.g. | pbcopy)"),
	)
	getParser.RegisterFlags(getCmd)
}

func runStoreGet(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "store.runStoreGet")()

	scope, err := parseStoreScope(cmd)
	if err != nil {
		return err
	}
	storeName, key := args[0], args[1]

	format, _ := cmd.Flags().GetString("format")
	raw, _ := cmd.Flags().GetBool("raw")
	if err := validateGetFlags(raw, cmd.Flags().Changed("format"), format); err != nil {
		return err
	}

	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	value, err := svc.Get(storeName, scope.Stack, scope.Component, key)
	if err != nil {
		return err
	}

	return writeStoreValue(key, value, format, raw)
}

// validateGetFlags rejects mutually exclusive flag combinations. --raw is text-only, so an
// explicit non-text --format is a conflict (rather than silently ignored).
func validateGetFlags(raw, formatChanged bool, format string) error {
	if raw && formatChanged && format != "text" {
		return ErrRawFormatConflict
	}
	return nil
}

// renderStoreValue formats a value for output. When raw is set, it returns the bare value with
// newline=false (no trailing newline, text only) so piping it (e.g. `| pbcopy`) does not capture
// a newline; otherwise it renders per format with newline=true.
func renderStoreValue(key string, value any, format string, raw bool) (content string, newline bool, err error) {
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
		return fmt.Sprintf("%s=%v", key, value), true, nil
	default:
		return fmt.Sprintf("%v", value), true, nil
	}
}

// writeStoreValue renders a single value and writes it to the masked data channel; a value from
// a `secret: true` store is revealed only when masking is disabled (--mask=false).
func writeStoreValue(key string, value any, format string, raw bool) error {
	content, newline, err := renderStoreValue(key, value, format, raw)
	if err != nil {
		return err
	}
	if newline {
		return data.Writeln(content)
	}
	return data.Write(content)
}
