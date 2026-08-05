package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
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

// validateFormatEnvParser wires each command's local "format" flag through the
// standard parser so ATMOS_VALIDATE_FORMAT follows the standard flag > env var
// > config > default precedence via pkg/flags, replacing a direct os.Getenv
// call. Each validate command already registers its own "format" pflag (their
// allowed values differ), so this parser is only used for BindFlagsToViper,
// which binds the caller's existing flag plus the environment variable.
var validateFormatEnvParser = flags.NewStandardParser(
	flags.WithStringFlag("format", "", "", "Output format"),
	flags.WithEnvVars("format", "ATMOS_VALIDATE_FORMAT"),
)

// validationFormat resolves the shared rich/text contract. Callers with
// legacy specialized formats (EditorConfig and ci validate) keep their own
// resolver and use this only for the common validators.
func validationFormat(cmd *cobra.Command) (string, error) {
	value := aggregateValidationFormat
	if value == "" {
		resolved, err := resolveValidationFormatValue(cmd)
		if err != nil {
			return "", err
		}
		value = resolved
	}
	if value == "" {
		return validateFormatText, nil
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value != validateFormatText && value != validateFormatRich {
		return "", fmt.Errorf("%w: %q", errUtils.ErrUnsupportedValidationFormat, value)
	}
	return value, nil
}

// resolveValidationFormatValue resolves cmd's own "format" flag/env/config
// value, used only when no aggregate-run override (aggregateValidationFormat)
// is active.
func resolveValidationFormatValue(cmd *cobra.Command) (string, error) {
	if err := validateFormatEnvParser.BindFlagsToViper(cmd, viper.GetViper()); err != nil {
		return "", err
	}
	if resolved := strings.TrimSpace(viper.GetString("format")); resolved != "" {
		return resolved, nil
	}
	return atmosConfig.Validate.Format, nil
}

func addValidationFormatFlag(cmd *cobra.Command) {
	// Register on the local flag set: every caller is a leaf command, and both
	// validationFormat and the aggregate runner read through cmd.Flags(), which
	// only includes persistent flags after cobra merges them during execution.
	cmd.Flags().String("format", "", "Output format: text, rich")
}
