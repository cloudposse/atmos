package flags

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ValidateConstrainedFields enforces schema.CommandFlag/CommandArgument's `values:` constraint
// for a custom command: static validation for any value already provided (reusing ValidateValue,
// the same check built-in commands' `valid_values:` already uses so both surfaces report
// identical error text), and -- when a values-constrained field is Required, still missing, and
// the terminal is interactive -- an interactive picker (reusing PromptForValue, gated by
// IsInteractive exactly like PromptForMissingRequired/PromptForPositionalArg already gate
// built-in commands' prompts). Runs independently of commandConfig.Component (unlike
// promptForSemanticValues), since `values:` is not tied to component/stack semantic typing.
func ValidateConstrainedFields(cmd *cobra.Command, commandConfig *schema.Command, argumentsData map[string]string, flagsData map[string]any) error {
	defer perf.Track(nil, "flags.ValidateConstrainedFields")()

	if err := validateConstrainedArguments(commandConfig.Arguments, argumentsData); err != nil {
		return err
	}
	return validateConstrainedFlags(cmd, commandConfig.Flags, flagsData)
}

func validateConstrainedArguments(arguments []schema.CommandArgument, argumentsData map[string]string) error {
	for _, arg := range arguments {
		if len(arg.Values) == 0 {
			continue
		}
		current := argumentsData[arg.Name]
		if current == "" {
			if !arg.Required || !IsInteractive() {
				continue
			}
			selected, err := PromptForValue(arg.Name, fmt.Sprintf("Choose %s", arg.Name), arg.Values)
			if err != nil {
				return err
			}
			argumentsData[arg.Name] = selected
			continue
		}
		if err := ValidateValue(arg.Name, current, arg.Values); err != nil {
			return err
		}
	}
	return nil
}

func validateConstrainedFlags(cmd *cobra.Command, flags []schema.CommandFlag, flagsData map[string]any) error {
	for i := range flags {
		fl := &flags[i]
		if len(fl.Values) == 0 {
			continue
		}
		current, _ := flagsData[fl.Name].(string)
		if current == "" {
			if err := promptForMissingConstrainedFlag(cmd, fl, flagsData); err != nil {
				return err
			}
			continue
		}
		if err := ValidateValue(fl.Name, current, fl.Values); err != nil {
			return err
		}
	}
	return nil
}

// promptForMissingConstrainedFlag prompts for a required-but-missing values-constrained flag
// (skipping silently when not required or not interactive), persisting the selection onto both
// flagsData and the actual flag so later steps in a multi-step command read the resolved value
// back from cmd.Flag(fl.Name) instead of re-prompting.
func promptForMissingConstrainedFlag(cmd *cobra.Command, fl *schema.CommandFlag, flagsData map[string]any) error {
	if !fl.Required || !IsInteractive() {
		return nil
	}
	selected, err := PromptForValue(fl.Name, fmt.Sprintf("Choose %s", fl.Name), fl.Values)
	if err != nil {
		return err
	}
	flagsData[fl.Name] = selected
	if setErr := cmd.PersistentFlags().Set(fl.Name, selected); setErr != nil {
		return fmt.Errorf("failed to set flag %q after prompting: %w", fl.Name, setErr)
	}
	return nil
}
