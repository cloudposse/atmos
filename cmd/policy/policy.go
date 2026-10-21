// Package policy provides offline policy authoring and validation commands.
package policy

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/hooks"
	policypkg "github.com/cloudposse/atmos/pkg/policy"
)

var command = newCommand()

func newCommand() *cobra.Command {
	root := &cobra.Command{Use: "policy", Short: "Validate, inspect, and test policies offline", PersistentPreRunE: func(*cobra.Command, []string) error { return nil }}
	check := &cobra.Command{Use: "check", Short: "Validate policies and check selected YAML/JSON files", Args: cobra.NoArgs, RunE: runCheck}
	check.Flags().Bool("syntax-only", false, "Validate definitions without checking files")
	check.Flags().String("root", ".", "Root directory for file policy globs")
	root.AddCommand(
		check,
		&cobra.Command{Use: "list", Short: "List loaded policy definitions", Args: cobra.NoArgs, RunE: runList},
		&cobra.Command{Use: "schema <input-kind>", Short: "Print the JSON Schema of a supported policy input", Args: cobra.ExactArgs(1), RunE: runSchema},
		&cobra.Command{Use: "test <cases.json>", Short: "Run offline fixtures against the policy bundle", Args: cobra.ExactArgs(1), RunE: runTest},
	)
	return root
}

func runCheck(cmd *cobra.Command, _ []string) error {
	bundle, err := loadBundle()
	if err != nil {
		return err
	}
	if err := hooks.ValidatePolicyBundle(bundle); err != nil {
		return err
	}
	report := map[string]any{"valid": true, "rules": len(bundle.Rules), "hooks": len(bundle.Hooks)}
	if len(bundle.RulesFor(policypkg.File)) == 0 {
		return data.WriteJSON(report)
	}
	ctx := context.Background()
	root := "."
	if cmd != nil {
		ctx = cmd.Context()
		syntaxOnly, _ := cmd.Flags().GetBool("syntax-only")
		if syntaxOnly {
			return data.WriteJSON(report)
		}
		if value, err := cmd.Flags().GetString("root"); err == nil {
			root = value
		}
	}
	snapshot, err := ci.CollectPolicySnapshot(ctx)
	if err != nil {
		return err
	}
	results, checkErr := policypkg.CheckFiles(ctx, bundle, root, policypkg.WithFileEvidence(policypkg.NewEngine(), snapshot))
	report["valid"] = checkErr == nil
	report["files"] = results
	if err := data.WriteJSON(report); err != nil {
		return err
	}
	return checkErr
}

func runList(_ *cobra.Command, _ []string) error {
	bundle, err := loadBundle()
	if err != nil {
		return err
	}
	return data.WriteJSON(bundle.Rules)
}

func runSchema(_ *cobra.Command, args []string) error {
	schema, err := policypkg.InputSchema(policypkg.InputKind(args[0]))
	if err != nil {
		return err
	}
	return data.WriteJSON(schema)
}

func runTest(cmd *cobra.Command, args []string) error {
	bundle, err := loadBundle()
	if err != nil {
		return err
	}
	if err := hooks.ValidatePolicyBundle(bundle); err != nil {
		return err
	}
	results, testErr := policypkg.RunTests(cmd.Context(), bundle, args[0])
	if results != nil {
		if err := data.WriteJSON(results); err != nil {
			return err
		}
	}
	return testErr
}

func loadBundle() (*policypkg.Bundle, error) {
	directory, err := policypkg.DirectoryFromEnvironment()
	if err != nil {
		return nil, err
	}
	if directory == "" {
		return nil, fmt.Errorf("%w: no policy directory; create policy.d or set ATMOS_POLICY_DIR", errUtils.ErrPolicyInvalid)
	}
	return policypkg.LoadDirectory(directory)
}

// IsInspection identifies the built-in offline command tree before config loading.
func IsInspection(root *cobra.Command, args []string) bool {
	cmd, _, err := root.Find(args)
	if err != nil {
		return false
	}
	for c := cmd; c != nil; c = c.Parent() {
		if c == command {
			return true
		}
	}
	return false
}

func init() { internal.Register(&CommandProvider{}) }

// CommandProvider registers the offline policy command group.
type CommandProvider struct{}

// GetCommand implements internal.CommandProvider.
func (*CommandProvider) GetCommand() *cobra.Command { return command }

// GetName implements internal.CommandProvider.
func (*CommandProvider) GetName() string { return "policy" }

// GetGroup implements internal.CommandProvider.
func (*CommandProvider) GetGroup() string { return "Other Commands" }

// GetFlagsBuilder implements internal.CommandProvider.
func (*CommandProvider) GetFlagsBuilder() flags.Builder { return nil }

// GetPositionalArgsBuilder implements internal.CommandProvider.
func (*CommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder { return nil }

// GetCompatibilityFlags implements internal.CommandProvider.
func (*CommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag { return nil }

// GetAliases implements internal.CommandProvider.
func (*CommandProvider) GetAliases() []internal.CommandAlias { return nil }

// IsExperimental implements internal.CommandProvider.
func (*CommandProvider) IsExperimental() bool { return false }
