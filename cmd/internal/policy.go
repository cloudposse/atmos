package internal

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
)

// CheckEarlyCommandAdmission covers built-ins before project initialization or
// configuration loading. Custom commands are checked after registration instead.
func CheckEarlyCommandAdmission(root *cobra.Command, args []string) error {
	defer perf.Track(nil, "internal.CheckEarlyCommandAdmission")()
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--help" || arg == "-h" || arg == "--help=true" {
			// Cobra renders help without running argument validators or hooks.
			return nil
		}
	}
	command, _, err := root.Find(args)
	if err != nil || !command.Runnable() {
		return nil
	}
	return checkCommandAdmission(root, command)
}

// installCommandAdmission gates argument validation, before any pre-run hooks.
// Each runnable command is wrapped, so overriding a persistent hook cannot bypass it.
// Restore prevents repeated embedded invocations accumulating wrappers.
func installCommandAdmission(root *cobra.Command) func() {
	var restores []func()
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if command.Runnable() {
			if command.Args == nil && !command.HasParent() && command.HasSubCommands() {
				// Find uses a nil root Args validator to reject unknown commands
				// before execution and usage rendering. Keep that sentinel intact.
				restores = append(restores, installRootAdmission(command))
			} else {
				original := command.Args
				command.Args = admittedCommandArgs(root, original)
				restores = append(restores, func() { command.Args = original })
			}
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
	return func() {
		for _, restore := range restores {
			restore()
		}
	}
}

// installRootAdmission preserves Cobra's implicit root argument validation while
// gating root execution before its persistent pre-run. Descendants retain their
// own Args gates, including when they override persistent hooks.
func installRootAdmission(root *cobra.Command) func() {
	originalE, original := root.PersistentPreRunE, root.PersistentPreRun
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd == root {
			if err := checkCommandAdmission(root, cmd); err != nil {
				return err
			}
		}
		if originalE != nil {
			return originalE(cmd, args)
		}
		if original != nil {
			original(cmd, args)
		}
		return nil
	}
	return func() { root.PersistentPreRunE = originalE }
}

func admittedCommandArgs(root *cobra.Command, original cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := checkCommandAdmission(root, cmd); err != nil {
			return err
		}
		if original != nil {
			return original(cmd, args)
		}
		return nil
	}
}

func checkCommandAdmission(root, cmd *cobra.Command) error {
	path := strings.TrimPrefix(cmd.CommandPath(), root.Name()+" ")
	if canonical := cmd.Annotations["policy-command"]; canonical != "" {
		path = canonical
	}
	data, err := json.Marshal(policy.CommandInput{Path: strings.Fields(path), Command: path})
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return admission.Check(ctx, &policy.Request{Kind: policy.Command, Input: data})
}
