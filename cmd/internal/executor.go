package internal

import (
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Execute runs the Cobra command and converts Cobra errors to sentinel errors.
// This is the boundary between Cobra's string-based errors and our sentinel errors.
// The command registry owns both registration and execution of commands.
func Execute(cmd *cobra.Command) (*cobra.Command, error) {
	executedCmd, err := cmd.ExecuteC()
	if err != nil {
		err = convertCobraError(err)
	}
	return executedCmd, err
}

// convertCobraError converts Cobra's dynamic string errors to our sentinel errors.
// Cobra v1.10.2 does not export sentinel errors, so string matching is required.
// See: github.com/spf13/cobra/args.go lines 36, 44.
func convertCobraError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// Cobra's "unknown command" error format: `unknown command "xyz" for "atmos"`.
	if strings.Contains(errMsg, "unknown command") {
		command := extractCommandName(errMsg)
		return errUtils.Build(errUtils.ErrCommandNotFound).
			WithCause(err).
			WithContext("command", command).
			WithHint("Run 'atmos --help' to see available commands").
			Err()
	}

	// Add more Cobra error conversions here as needed.

	return err
}

// extractCommandName extracts the command name from Cobra's error message.
func extractCommandName(errMsg string) string {
	re := regexp.MustCompile(`unknown command "([^"]+)"`)
	match := re.FindStringSubmatch(errMsg)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}
