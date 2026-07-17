package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	validateFormatText = "text"
	validateFormatRich = "rich"
)

// aggregateValidationFormat carries the root command's presentation choice to
// focused commands during one in-process aggregate run. Cobra keeps persistent
// flag state on separate command instances, so relying on flag mutation here
// leaks state into later invocations and is not deterministic in tests.
var aggregateValidationFormat string

// validationFormat resolves the shared rich/text contract. Callers with
// legacy specialized formats (EditorConfig and ci validate) keep their own
// resolver and use this only for the common validators.
func validationFormat(cmd *cobra.Command) (string, error) {
	value := ""
	if aggregateValidationFormat != "" {
		value = aggregateValidationFormat
	} else if cmd.Flags().Changed("format") {
		value, _ = cmd.Flags().GetString("format")
	} else if env := os.Getenv("ATMOS_VALIDATE_FORMAT"); env != "" {
		value = env
	} else {
		value = atmosConfig.Validate.Format
	}
	if value == "" {
		return validateFormatText, nil
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value != validateFormatText && value != validateFormatRich {
		return "", fmt.Errorf("unsupported validation format %q: expected text or rich", value)
	}
	return value, nil
}

func addValidationFormatFlag(cmd *cobra.Command) {
	// Register on the local flag set: every caller is a leaf command, and both
	// validationFormat and the aggregate runner read through cmd.Flags(), which
	// only includes persistent flags after cobra merges them during execution.
	cmd.Flags().String("format", "", "Output format: text, rich")
}
