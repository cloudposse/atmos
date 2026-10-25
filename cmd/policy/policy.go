// Package policy provides offline policy authoring and validation commands.
package policy

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/hooks"
	policypkg "github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/ui"
)

var checkParser = flags.NewStandardParser(
	flags.WithBoolFlag("syntax-only", "", false, "Validate definitions without checking files"),
	flags.WithStringFlag("root", "", ".", "Root directory for file policy globs"),
	flags.WithStringFlag("format", "f", "text", "Output format: text or json"),
	flags.WithEnvVars("format", "ATMOS_POLICY_CHECK_FORMAT"),
	flags.WithValidValues("format", "text", "json"),
)

var command = newCommand()

func newCommand() *cobra.Command {
	root := &cobra.Command{Use: "policy", Short: "Validate, inspect, and test policies offline", PersistentPreRunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(
		newCheckCommand(),
		&cobra.Command{Use: "list", Short: "List loaded policy definitions", Args: cobra.NoArgs, RunE: runList},
		&cobra.Command{Use: "schema <input-kind>", Short: "Print the JSON Schema of a supported policy input", Args: cobra.ExactArgs(1), RunE: runSchema},
		&cobra.Command{Use: "test <cases.json>", Short: "Run offline fixtures against the policy bundle", Args: cobra.ExactArgs(1), RunE: runTest},
	)
	return root
}

func newCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "check",
		Short:   "Validate policies and check selected YAML/JSON files",
		Long:    "Validate the policy bundle and check selected YAML/JSON files.\nUse --syntax-only to validate definitions without checking files.\nUse --format json for machine-readable output on stdout.",
		Example: "atmos policy check\natmos policy check --format json",
		Args:    cobra.NoArgs,
		RunE:    runCheck,
	}
	checkParser.RegisterFlags(cmd)
	return cmd
}

func runCheck(cmd *cobra.Command, _ []string) error {
	// Keep offline inspection independent of project configuration and global Viper state.
	v := viper.New()
	if err := checkParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	if err := checkParser.ValidateFlagValues(cmd); err != nil {
		return err
	}
	bundle, err := loadBundle()
	if err != nil {
		return err
	}
	if err := hooks.ValidatePolicyBundle(bundle); err != nil {
		return err
	}
	report := checkReport{Valid: true, Rules: len(bundle.Rules), Hooks: len(bundle.Hooks)}
	var checkErr error
	if len(bundle.RulesFor(policypkg.File)) > 0 && !v.GetBool("syntax-only") {
		snapshot, err := ci.CollectPolicySnapshot(cmd.Context())
		if err != nil {
			return err
		}
		report.Files, checkErr = policypkg.CheckFiles(cmd.Context(), bundle, v.GetString("root"), policypkg.WithFileEvidence(policypkg.NewEngine(), snapshot))
		report.Valid = checkErr == nil
	}
	if err := writeCheckReport(report, v.GetString("format")); err != nil {
		return err
	}
	return checkErr
}

type checkReport struct {
	Valid bool                   `json:"valid"`
	Rules int                    `json:"rules"`
	Hooks int                    `json:"hooks"`
	Files []policypkg.FileResult `json:"files,omitempty"`
}

func writeCheckReport(report checkReport, format string) error {
	if format == "json" {
		return data.WriteJSON(report)
	}
	if report.Valid {
		ui.Success("Policy bundle is valid")
	} else {
		ui.Error("Policy file checks failed")
	}
	ui.Writef("Rules: %d\nHooks: %d\n", report.Rules, report.Hooks)
	for i := range report.Files {
		writeFileResult(&report.Files[i])
	}
	return nil
}

func writeFileResult(result *policypkg.FileResult) {
	ui.Writef("%s: %s\n", result.File, result.Decision.Status)
	if result.Error != "" {
		ui.Writef("  %s\n", result.Error)
	}
	for i := range result.Decision.Findings {
		finding := &result.Decision.Findings[i]
		for _, diagnostic := range finding.Diagnostics {
			ui.Writef("  %s %s: %s\n", finding.RuleID, diagnostic.Path, diagnostic.Message)
		}
	}
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

func init() {
	internal.Register(&CommandProvider{}, internal.WithDocumentationURL("https://atmos.tools/cli/commands/policy"), internal.WithExample("examples/policies", "Policies"))
}

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
