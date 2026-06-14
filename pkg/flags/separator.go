package flags

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// SplitArgsAtDash splits args into positional args (before "--") and
// pass-through args (after "--") using Cobra's ArgsLenAtDash().
//
// Cobra consumes the literal "--" during flag parsing, so the args a command
// receives never contain it; ArgsLenAtDash() records how many positional args
// appeared before it (-1 when no "--" was present). When there is no
// separator, or cmd is nil (e.g. validators invoked directly in tests), all
// args are positional and separated is nil.
//
// The dashIndex > len(args) guard protects callers that pass a slice shorter
// than what Cobra parsed (ArgsLenAtDash reflects Cobra's parse state, not the
// slice argument).
func SplitArgsAtDash(cmd *cobra.Command, args []string) (positional, separated []string) {
	defer perf.Track(nil, "flags.SplitArgsAtDash")()

	if cmd == nil {
		return args, nil
	}

	dashIndex := cmd.ArgsLenAtDash()
	if dashIndex < 0 || dashIndex > len(args) {
		return args, nil
	}
	return args[:dashIndex], args[dashIndex:]
}

// SeparatorAwareValidator wraps a cobra.PositionalArgs validator so it
// validates only the args before the "--" separator.
//
// Plain Cobra validators (cobra.ExactArgs, cobra.RangeArgs, ...) count the
// pass-through args after "--" as positional args, rejecting valid
// invocations like `command <component> -- --native-flag`. Wrapping them with
// this function makes any positional-args contract separator-aware without
// per-command splitting logic.
//
// A nil validator is returned unchanged.
func SeparatorAwareValidator(validator cobra.PositionalArgs) cobra.PositionalArgs {
	defer perf.Track(nil, "flags.SeparatorAwareValidator")()

	if validator == nil {
		return nil
	}

	return func(cmd *cobra.Command, args []string) error {
		positional, _ := SplitArgsAtDash(cmd, args)
		return validator(cmd, positional)
	}
}
